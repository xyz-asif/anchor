package feed

import (
	"context"
	"log"

	"github.com/xyz-asif/gotodo/internal/features/anchor_follows"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/likes"
	"github.com/xyz-asif/gotodo/internal/pkg/tags"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Service struct {
	feedRepo          *Repository
	authRepo          *auth.Repository
	followsRepo       *follows.Repository
	likesRepo         *likes.Repository
	anchorsRepo       *anchors.Repository
	anchorFollowsRepo *anchor_follows.Repository
}

func NewService(
	feedRepo *Repository,
	authRepo *auth.Repository,
	followsRepo *follows.Repository,
	likesRepo *likes.Repository,
	anchorsRepo *anchors.Repository,
	anchorFollowsRepo *anchor_follows.Repository,
) *Service {
	return &Service{
		feedRepo:          feedRepo,
		authRepo:          authRepo,
		followsRepo:       followsRepo,
		likesRepo:         likesRepo,
		anchorsRepo:       anchorsRepo,
		anchorFollowsRepo: anchorFollowsRepo,
	}
}

// hiddenAuthors returns the users whose anchors the viewer must not see in feeds: users the
// viewer blocked or muted, and users who blocked the viewer. A lookup failure is an error, so
// a feed never shows blocked people because a query failed.
func (s *Service) hiddenAuthors(ctx context.Context, viewer *auth.User) (blocked, hidden []primitive.ObjectID, err error) {
	blocked, err = s.authRepo.HiddenUserIDs(ctx, viewer)
	if err != nil {
		return nil, nil, err
	}
	hidden = append(append([]primitive.ObjectID{}, blocked...), viewer.MutedUsers...)
	return blocked, hidden, nil
}

func (s *Service) GetHomeFeed(
	ctx context.Context,
	viewer *auth.User,
	query *FeedQuery,
) (*HomeFeedResponse, error) {
	userID := viewer.ID
	cursor := query.Cursor
	includeOwn := query.IncludeOwn

	followingIDs, err := s.followsRepo.GetAllFollowingIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	blockedIDs, hiddenIDs, err := s.hiddenAuthors(ctx, viewer)
	if err != nil {
		return nil, err
	}

	feedUserIDs := append([]primitive.ObjectID{}, followingIDs...)
	if includeOwn {
		feedUserIDs = append(feedUserIDs, userID)
	}

	// The extra sections only come with the first page; they are best effort, so a failure
	// there still returns the feed itself
	followingAnchors := FollowingAnchorsSection{Items: []FollowingAnchorFeedItem{}}
	suggestedCategories := []SuggestedCategory{}
	if cursor == nil {
		if section, err := s.getFollowingAnchorsForFeed(ctx, userID, blockedIDs); err != nil {
			log.Printf("feed: followed anchors for %s failed: %v", userID.Hex(), err)
		} else {
			followingAnchors = section
		}
		if categories, err := s.getSuggestedCategories(ctx); err != nil {
			log.Printf("feed: suggested categories failed: %v", err)
		} else {
			suggestedCategories = categories
		}
	}

	emptyFeed := func(reason string) *HomeFeedResponse {
		return &HomeFeedResponse{
			FollowingAnchors:    followingAnchors,
			SuggestedCategories: suggestedCategories,
			Feed: FeedResponse{
				Items:      []FeedItem{},
				Pagination: FeedPagination{Limit: query.Limit},
				Meta: FeedMeta{
					FeedType:           "following",
					IncludesOwnAnchors: includeOwn,
					TotalFollowing:     len(followingIDs),
					EmptyReason:        &reason,
				},
			},
		}
	}

	if len(feedUserIDs) == 0 {
		return emptyFeed("NO_FOLLOWING"), nil
	}

	// Blocked and muted authors are dropped in the query, so pages stay full
	anchorsList, err := s.feedRepo.GetFeedAnchors(ctx, feedUserIDs, hiddenIDs, cursor, query.Limit+1)
	if err != nil {
		return nil, err
	}

	hasMore := false
	if len(anchorsList) > query.Limit {
		hasMore = true
		anchorsList = anchorsList[:query.Limit]
	}

	if len(anchorsList) == 0 {
		switch {
		case cursor != nil:
			return emptyFeed("END_OF_FEED"), nil
		case len(followingIDs) == 0:
			return emptyFeed("NO_FOLLOWING"), nil
		default:
			return emptyFeed("NO_CONTENT"), nil
		}
	}

	// The cursor comes from the last anchor of the query, before anything is dropped below
	var nextCursor *string
	if hasMore {
		lastAnchor := anchorsList[len(anchorsList)-1]
		c := EncodeCursor(lastAnchor.LastItemAddedAt, lastAnchor.ID)
		nextCursor = &c
	}

	users, err := s.loadUsers(ctx, authorIDsOf(anchorsList))
	if err != nil {
		return nil, err
	}
	engagementMap, err := s.enrichWithEngagement(ctx, anchorsList, viewer, followingIDs, hiddenIDs)
	if err != nil {
		return nil, err
	}
	previewsMap, err := s.getPreviewsForAnchors(ctx, anchorIDsOf(anchorsList))
	if err != nil {
		return nil, err
	}

	feedItems := make([]FeedItem, 0, len(anchorsList))
	for _, anchor := range anchorsList {
		author, ok := users[anchor.UserID]
		if !ok {
			continue // the author's account is being deleted
		}
		preview := previewsMap[anchor.ID]
		if preview == nil {
			preview = &FeedPreview{Items: []FeedPreviewItem{}}
		}
		feedItems = append(feedItems, FeedItem{
			ID:              anchor.ID,
			Title:           anchor.Title,
			Description:     anchor.Description,
			CoverMediaType:  anchor.CoverMediaType,
			CoverMediaValue: anchor.CoverMediaValue,
			Visibility:      anchor.Visibility,
			IsPinned:        anchor.IsPinned,
			Tags:            nonNilTags(anchor.Tags),
			ItemCount:       anchor.ItemCount,
			LikeCount:       anchor.LikeCount,
			CloneCount:      anchor.CloneCount,
			CommentCount:    anchor.CommentCount,
			LastItemAddedAt: anchor.LastItemAddedAt,
			CreatedAt:       anchor.CreatedAt,
			Author: FeedItemAuthor{
				ID:             author.ID,
				Username:       author.Username,
				DisplayName:    author.DisplayName,
				ProfilePicture: optionalString(author.ProfilePictureURL),
				IsVerified:     author.IsVerified,
			},
			Engagement: *engagementMap[anchor.ID],
			Preview:    *preview,
		})
	}

	return &HomeFeedResponse{
		FollowingAnchors:    followingAnchors,
		SuggestedCategories: suggestedCategories,
		Feed: FeedResponse{
			Items: feedItems,
			Pagination: FeedPagination{
				Limit:      query.Limit,
				HasMore:    hasMore,
				NextCursor: nextCursor,
				ItemCount:  len(feedItems),
			},
			Meta: FeedMeta{
				FeedType:           "following",
				IncludesOwnAnchors: includeOwn,
				TotalFollowing:     len(followingIDs),
				EmptyReason:        nil,
			},
		},
	}, nil
}

// getFollowingAnchorsForFeed returns the 10 followed anchors updated most recently. Deleted
// anchors, anchors that went private and anchors by blocked owners are excluded in the query,
// so the section is full and the total is exact.
func (s *Service) getFollowingAnchorsForFeed(ctx context.Context, userID primitive.ObjectID, blockedIDs []primitive.ObjectID) (FollowingAnchorsSection, error) {
	rows, total, _, err := s.anchorFollowsRepo.ListFollowing(ctx, userID, anchor_follows.FollowingListOptions{
		Sort:        anchor_follows.SortUpdated,
		HiddenUsers: blockedIDs,
		Page:        1,
		Limit:       10,
	})
	if err != nil {
		return FollowingAnchorsSection{}, err
	}

	items := make([]FollowingAnchorFeedItem, 0, len(rows))
	for _, row := range rows {
		a := row.Anchor
		items = append(items, FollowingAnchorFeedItem{
			ID:              a.ID,
			Title:           a.Title,
			CoverMediaType:  a.CoverMediaType,
			CoverMediaValue: a.CoverMediaValue,
			HasUpdate:       a.Version > row.Follow.LastSeenVersion,
			LastSeenVersion: row.Follow.LastSeenVersion,
			CurrentVersion:  a.Version,
		})
	}
	return FollowingAnchorsSection{Items: items, TotalCount: int(total)}, nil
}

func (s *Service) getSuggestedCategories(ctx context.Context) ([]SuggestedCategory, error) {
	popular, err := s.anchorsRepo.GetPopularTags(ctx, 10)
	if err != nil {
		return nil, err
	}
	results := make([]SuggestedCategory, len(popular))
	for i, t := range popular {
		results[i] = SuggestedCategory{Name: t.Name, AnchorCount: t.Count}
	}
	return results, nil
}

// loadUsers fetches the given users (deduplicated), keyed by ID
func (s *Service) loadUsers(ctx context.Context, userIDs []primitive.ObjectID) (map[primitive.ObjectID]*auth.User, error) {
	seen := make(map[primitive.ObjectID]bool, len(userIDs))
	ids := make([]primitive.ObjectID, 0, len(userIDs))
	for _, id := range userIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	users, err := s.authRepo.GetUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[primitive.ObjectID]*auth.User, len(users))
	for i := range users {
		byID[users[i].ID] = &users[i]
	}
	return byID, nil
}

// enrichWithEngagement adds the viewer's like/clone state and the like summary to every anchor
// on the page, with a fixed number of queries (no per-anchor queries). viewer may be nil.
func (s *Service) enrichWithEngagement(
	ctx context.Context,
	anchorsList []anchors.Anchor,
	viewer *auth.User,
	followingIDs []primitive.ObjectID,
	hiddenIDs []primitive.ObjectID,
) (map[primitive.ObjectID]*FeedEngagement, error) {
	anchorIDs := anchorIDsOf(anchorsList)

	likedMap := map[primitive.ObjectID]bool{}
	clonedMap := map[primitive.ObjectID]bool{}
	if viewer != nil {
		var err error
		if likedMap, err = s.likesRepo.GetUserLikedAnchors(ctx, viewer.ID, anchorIDs); err != nil {
			return nil, err
		}
		if clonedMap, err = s.feedRepo.GetUserClonedAnchors(ctx, viewer.ID, anchorIDs); err != nil {
			return nil, err
		}
	}

	summaries, err := s.getLikeSummaries(ctx, anchorsList, followingIDs, hiddenIDs)
	if err != nil {
		return nil, err
	}

	engagementMap := make(map[primitive.ObjectID]*FeedEngagement, len(anchorsList))
	for _, a := range anchorsList {
		engagementMap[a.ID] = &FeedEngagement{
			HasLiked:    likedMap[a.ID],
			HasCloned:   clonedMap[a.ID],
			LikeSummary: summaries[a.ID],
		}
	}
	return engagementMap, nil
}

func (s *Service) getPreviewsForAnchors(ctx context.Context, anchorIDs []primitive.ObjectID) (map[primitive.ObjectID]*FeedPreview, error) {
	previewMap := make(map[primitive.ObjectID]*FeedPreview)

	// One query for all anchors on the page (was one query per anchor)
	itemsByAnchor, err := s.feedRepo.GetPreviewItemsForAnchors(ctx, anchorIDs, 3)
	if err != nil {
		return nil, err
	}

	for _, id := range anchorIDs {
		items := itemsByAnchor[id]

		previewItems := make([]FeedPreviewItem, len(items))
		for i, item := range items {
			preview := FeedPreviewItem{
				Type: item.Type,
			}
			if item.Type == "url" && item.URLData != nil {
				thumb := item.URLData.Favicon
				if thumb == "" {
					thumb = item.URLData.Thumbnail
				}
				if thumb != "" {
					preview.Thumbnail = &thumb
				}
				if item.URLData.Title != "" {
					t := truncateRunes(item.URLData.Title, 50, "")
					preview.Title = &t
				}
			} else if item.Type == "image" && item.ImageData != nil {
				preview.Thumbnail = &item.ImageData.CloudinaryURL
			} else if item.Type == "text" && item.TextData != nil {
				t := truncateRunes(item.TextData.Content, 100, "...")
				preview.Snippet = &t
			}
			previewItems[i] = preview
		}
		previewMap[id] = &FeedPreview{Items: previewItems}
	}

	return previewMap, nil
}

// getLikeSummaries builds "liked by <people you follow> and N others" for every anchor on the
// page: up to 3 followed users per anchor (newest like first), found among *all* the anchor's
// likes in a single query. Blocked and muted users are never named.
func (s *Service) getLikeSummaries(
	ctx context.Context,
	anchorsList []anchors.Anchor,
	followingIDs []primitive.ObjectID,
	hiddenIDs []primitive.ObjectID,
) (map[primitive.ObjectID]FeedLikeSummary, error) {
	hidden := make(map[primitive.ObjectID]bool, len(hiddenIDs))
	for _, id := range hiddenIDs {
		hidden[id] = true
	}
	candidates := make([]primitive.ObjectID, 0, len(followingIDs))
	for _, id := range followingIDs {
		if !hidden[id] {
			candidates = append(candidates, id)
		}
	}

	likersByAnchor, err := s.likesRepo.GetLikersAmong(ctx, anchorIDsOf(anchorsList), candidates, 3)
	if err != nil {
		return nil, err
	}
	var likerIDs []primitive.ObjectID
	for _, ids := range likersByAnchor {
		likerIDs = append(likerIDs, ids...)
	}
	users := map[primitive.ObjectID]*auth.User{}
	if len(likerIDs) > 0 {
		if users, err = s.loadUsers(ctx, likerIDs); err != nil {
			return nil, err
		}
	}

	summaries := make(map[primitive.ObjectID]FeedLikeSummary, len(anchorsList))
	for _, a := range anchorsList {
		likedBy := []FeedLikeSummaryUser{}
		for _, id := range likersByAnchor[a.ID] {
			if u, ok := users[id]; ok {
				likedBy = append(likedBy, FeedLikeSummaryUser{
					ID:             u.ID,
					Username:       u.Username,
					DisplayName:    u.DisplayName,
					ProfilePicture: optionalString(u.ProfilePictureURL),
				})
			}
		}
		total := a.LikeCount
		if total < len(likedBy) {
			total = len(likedBy) // counter lagging behind the likes themselves
		}
		summaries[a.ID] = FeedLikeSummary{
			TotalCount:       total,
			LikedByFollowing: likedBy,
			OtherLikersCount: total - len(likedBy),
		}
	}
	return summaries, nil
}

func (s *Service) GetDiscoverFeed(
	ctx context.Context,
	viewer *auth.User, // nil if not signed in
	query *DiscoverQuery,
) (*DiscoverResponse, error) {
	cursor := query.Cursor
	isAuthenticated := viewer != nil

	var excludeUserIDs, followingIDs, hiddenIDs, hiddenAnchorIDs []primitive.ObjectID
	var interestTags []string
	if isAuthenticated {
		var err error
		if followingIDs, err = s.followsRepo.GetAllFollowingIDs(ctx, viewer.ID); err != nil {
			return nil, err
		}
		if _, hiddenIDs, err = s.hiddenAuthors(ctx, viewer); err != nil {
			return nil, err
		}
		excludeUserIDs = append(excludeUserIDs, hiddenIDs...)
		// Discover shows people you don't follow yet; the tag feed shows everyone
		if !query.IncludeFollowed {
			excludeUserIDs = append(append(excludeUserIDs, followingIDs...), viewer.ID)
		}
		hiddenAnchorIDs = viewer.HiddenAnchors

		if query.Category == CategoryForYou {
			interestTags = tags.NormalizeList(viewer.Interests)
			// If nothing the viewer could see matches their interests, fall back to popular
			// instead of an empty feed (deterministic, so every page of the same scroll agrees)
			if len(interestTags) > 0 {
				ok, err := s.feedRepo.AnyDiscoverableAnchorWithTags(ctx, interestTags, excludeUserIDs, hiddenAnchorIDs)
				if err != nil {
					return nil, err
				}
				if !ok {
					interestTags = nil
				}
			}
		}
	}
	// for_you without saved interests (or signed out) behaves like popular

	var tagPtr *string
	if query.Tag != "" {
		tagPtr = &query.Tag
	}

	anchorsList, err := s.feedRepo.GetDiscoverAnchors(
		ctx, excludeUserIDs, hiddenAnchorIDs, interestTags, query.Category, tagPtr, cursor, query.Limit+1,
	)
	if err != nil {
		return nil, err
	}

	hasMore := false
	if len(anchorsList) > query.Limit {
		hasMore = true
		anchorsList = anchorsList[:query.Limit]
	}

	if len(anchorsList) == 0 {
		var emptyReason string
		switch {
		case cursor != nil:
			emptyReason = "END_OF_FEED"
		case query.Tag != "":
			emptyReason = "NO_TAG_CONTENT"
		default:
			emptyReason = "NO_CONTENT"
		}
		return &DiscoverResponse{
			Items:      []DiscoverItem{},
			Pagination: FeedPagination{Limit: query.Limit},
			Meta: DiscoverMeta{
				FeedType:        "discover",
				Category:        query.Category,
				Tag:             tagPtr,
				IsAuthenticated: isAuthenticated,
				EmptyReason:     &emptyReason,
			},
		}, nil
	}

	// The cursor comes from the last anchor of the query, before anything is dropped below
	var nextCursor *string
	if hasMore {
		lastAnchor := anchorsList[len(anchorsList)-1]
		var c string
		if query.Category == CategoryRecent {
			c = EncodeDiscoverCursor(nil, lastAnchor.CreatedAt, lastAnchor.ID)
		} else {
			score := lastAnchor.EngagementScore
			c = EncodeDiscoverCursor(&score, lastAnchor.CreatedAt, lastAnchor.ID)
		}
		nextCursor = &c
	}

	users, err := s.loadUsers(ctx, authorIDsOf(anchorsList))
	if err != nil {
		return nil, err
	}
	engagementMap, err := s.enrichWithEngagement(ctx, anchorsList, viewer, followingIDs, hiddenIDs)
	if err != nil {
		return nil, err
	}
	previewsMap, err := s.getPreviewsForAnchors(ctx, anchorIDsOf(anchorsList))
	if err != nil {
		return nil, err
	}

	items := make([]DiscoverItem, 0, len(anchorsList))
	for _, anchor := range anchorsList {
		author, ok := users[anchor.UserID]
		if !ok {
			continue // the author's account is being deleted
		}
		preview := previewsMap[anchor.ID]
		if preview == nil {
			preview = &FeedPreview{Items: []FeedPreviewItem{}}
		}
		items = append(items, DiscoverItem{
			ID:              anchor.ID,
			Title:           anchor.Title,
			Description:     anchor.Description,
			CoverMediaType:  anchor.CoverMediaType,
			CoverMediaValue: anchor.CoverMediaValue,
			Visibility:      anchor.Visibility,
			IsPinned:        anchor.IsPinned,
			Tags:            nonNilTags(anchor.Tags),
			ItemCount:       anchor.ItemCount,
			LikeCount:       anchor.LikeCount,
			CloneCount:      anchor.CloneCount,
			CommentCount:    anchor.CommentCount,
			EngagementScore: anchor.EngagementScore,
			LastItemAddedAt: anchor.LastItemAddedAt,
			CreatedAt:       anchor.CreatedAt,
			Author: DiscoverItemAuthor{
				ID:             author.ID,
				Username:       author.Username,
				DisplayName:    author.DisplayName,
				ProfilePicture: optionalString(author.ProfilePictureURL),
				IsVerified:     author.IsVerified,
				FollowerCount:  author.FollowerCount,
			},
			Engagement: *engagementMap[anchor.ID],
			Preview:    *preview,
		})
	}

	return &DiscoverResponse{
		Items: items,
		Pagination: FeedPagination{
			Limit:      query.Limit,
			HasMore:    hasMore,
			NextCursor: nextCursor,
			ItemCount:  len(items),
		},
		Meta: DiscoverMeta{
			FeedType:        "discover",
			Category:        query.Category,
			Tag:             tagPtr,
			IsAuthenticated: isAuthenticated,
			EmptyReason:     nil,
		},
	}, nil
}

func anchorIDsOf(list []anchors.Anchor) []primitive.ObjectID {
	ids := make([]primitive.ObjectID, len(list))
	for i, a := range list {
		ids[i] = a.ID
	}
	return ids
}

func authorIDsOf(list []anchors.Anchor) []primitive.ObjectID {
	ids := make([]primitive.ObjectID, len(list))
	for i, a := range list {
		ids[i] = a.UserID
	}
	return ids
}

// optionalString returns nil for "", so a missing picture is null rather than ""
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nonNilTags(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}

// truncateRunes shortens s to at most max characters (not bytes, so multi-byte characters
// are never cut in half), appending suffix when it was shortened.
func truncateRunes(s string, max int, suffix string) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + suffix
}
