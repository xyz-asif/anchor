package feed

import (
	"context"

	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/likes"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Service struct {
	feedRepo    *Repository
	authRepo    *auth.Repository
	followsRepo *follows.Repository
	likesRepo   *likes.Repository
	anchorsRepo *anchors.Repository
}

func NewService(
	feedRepo *Repository,
	authRepo *auth.Repository,
	followsRepo *follows.Repository,
	likesRepo *likes.Repository,
	anchorsRepo *anchors.Repository,
) *Service {
	return &Service{
		feedRepo:    feedRepo,
		authRepo:    authRepo,
		followsRepo: followsRepo,
		likesRepo:   likesRepo,
		anchorsRepo: anchorsRepo,
	}
}

func (s *Service) GetHomeFeed(
	ctx context.Context,
	userID primitive.ObjectID,
	query *FeedQuery,
) (*FeedResponse, error) {
	// 1. Decode Cursor
	cursor, err := DecodeCursor(query.Cursor)
	if err != nil {
		return nil, err
	}

	// 2. Get Following List
	followingIDs, err := s.followsRepo.GetAllFollowingIDs(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 3. Handle Own Anchors
	includeOwn := true
	if query.IncludeOwn != nil {
		includeOwn = *query.IncludeOwn
	}

	feedUserIDs := followingIDs
	if includeOwn {
		feedUserIDs = append(feedUserIDs, userID)
	}

	// 4. Check for empty connection
	if len(feedUserIDs) == 0 {
		reason := "NO_FOLLOWING"
		return &FeedResponse{
			Items: []FeedItem{},
			Pagination: FeedPagination{
				Limit: query.Limit,
			},
			Meta: FeedMeta{
				FeedType:           "following",
				IncludesOwnAnchors: includeOwn,
				TotalFollowing:     0,
				EmptyReason:        &reason,
			},
		}, nil
	}

	// 5. Query Anchors
	anchorsList, err := s.feedRepo.GetFeedAnchors(ctx, feedUserIDs, cursor, query.Limit+1)
	if err != nil {
		return nil, err
	}

	// 6. Check HasMore
	hasMore := false
	if len(anchorsList) > query.Limit {
		hasMore = true
		anchorsList = anchorsList[:query.Limit]
	}

	// 7. Handle Empty Results
	if len(anchorsList) == 0 {
		reason := "END_OF_FEED"
		if cursor == nil {
			if len(followingIDs) == 0 && !includeOwn {
				reason = "NO_FOLLOWING"
			} else {
				reason = "NO_CONTENT"
			}
		}
		emptyReasonPtr := &reason
		if reason == "END_OF_FEED" && cursor != nil {
			// Actually if cursor is not null, end of feed is correct.
			// If cursor is null, it means first page is empty.
		} else if reason != "END_OF_FEED" {
			// It was first page empty
		}

		return &FeedResponse{
			Items: []FeedItem{},
			Pagination: FeedPagination{
				Limit:      query.Limit,
				HasMore:    false,
				NextCursor: nil,
				ItemCount:  0,
			},
			Meta: FeedMeta{
				FeedType:           "following",
				IncludesOwnAnchors: includeOwn,
				TotalFollowing:     len(followingIDs),
				EmptyReason:        emptyReasonPtr,
			},
		}, nil
	}

	// 8. Enrich Authors
	anchorAuthorIDs := make([]primitive.ObjectID, len(anchorsList))
	for i, a := range anchorsList {
		anchorAuthorIDs[i] = a.UserID
	}
	authorsMap, err := s.enrichWithAuthors(ctx, anchorAuthorIDs)
	if err != nil {
		return nil, err
	}

	// 9. Enrich Engagement
	engagementMap, err := s.enrichWithEngagement(ctx, anchorsList, userID, followingIDs)
	if err != nil {
		return nil, err
	}

	// 10. Enrich Previews
	anchorIDs := make([]primitive.ObjectID, len(anchorsList))
	for i, a := range anchorsList {
		anchorIDs[i] = a.ID
	}
	previewsMap, err := s.getPreviewsForAnchors(ctx, anchorIDs)
	if err != nil {
		return nil, err
	}

	// 11. Build Response Items
	feedItems := make([]FeedItem, len(anchorsList))
	for i, anchor := range anchorsList {
		author, ok := authorsMap[anchor.UserID]
		if !ok {
			// Fallback or skip? Ideally shouldn't happen
			author = &FeedItemAuthor{ID: anchor.UserID, Username: "Deleted User"}
		}

		engagement := engagementMap[anchor.ID]
		preview := previewsMap[anchor.ID]
		if preview == nil {
			preview = &FeedPreview{Items: []FeedPreviewItem{}}
		}

		feedItems[i] = FeedItem{
			ID:              anchor.ID,
			Title:           anchor.Title,
			Description:     anchor.Description,
			CoverMediaType:  anchor.CoverMediaType,
			CoverMediaValue: anchor.CoverMediaValue,
			Visibility:      anchor.Visibility,
			IsPinned:        anchor.IsPinned,
			Tags:            anchor.Tags,
			ItemCount:       anchor.ItemCount,
			LikeCount:       anchor.LikeCount,
			CloneCount:      anchor.CloneCount,
			CommentCount:    anchor.CommentCount,
			LastItemAddedAt: anchor.LastItemAddedAt,
			CreatedAt:       anchor.CreatedAt,
			Author:          *author,
			Engagement:      *engagement,
			Preview:         *preview,
		}
	}

	// 12. Build Next Cursor
	var nextCursor *string
	if hasMore {
		lastAnchor := anchorsList[len(anchorsList)-1]
		c := EncodeCursor(lastAnchor.LastItemAddedAt, lastAnchor.ID)
		nextCursor = &c
	}

	return &FeedResponse{
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
	}, nil

}

func (s *Service) enrichWithAuthors(ctx context.Context, userIDs []primitive.ObjectID) (map[primitive.ObjectID]*FeedItemAuthor, error) {
	// Deduplicate IDs
	uniqueIDs := make(map[primitive.ObjectID]bool)
	var ids []primitive.ObjectID
	for _, id := range userIDs {
		if !uniqueIDs[id] {
			uniqueIDs[id] = true
			ids = append(ids, id)
		}
	}

	users, err := s.authRepo.GetUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	authorMap := make(map[primitive.ObjectID]*FeedItemAuthor)
	for _, user := range users {
		authorMap[user.ID] = &FeedItemAuthor{
			ID:             user.ID,
			Username:       user.Username,
			DisplayName:    user.DisplayName,
			ProfilePicture: &user.ProfilePictureURL,
			IsVerified:     false, // Pending verification feature
		}
	}
	return authorMap, nil
}

func (s *Service) enrichWithEngagement(
	ctx context.Context,
	anchorsList []anchors.Anchor,
	userID primitive.ObjectID,
	followingIDs []primitive.ObjectID,
) (map[primitive.ObjectID]*FeedEngagement, error) {
	anchorIDs := make([]primitive.ObjectID, len(anchorsList))
	for i, a := range anchorsList {
		anchorIDs[i] = a.ID
	}

	// Batch get liked status
	likedMap, err := s.likesRepo.GetUserLikedAnchors(ctx, userID, anchorIDs)
	if err != nil {
		return nil, err
	}

	// Batch get cloned status
	clonedMap, err := s.feedRepo.GetUserClonedAnchors(ctx, userID, anchorIDs)
	if err != nil {
		return nil, err
	}

	engagementMap := make(map[primitive.ObjectID]*FeedEngagement)

	// We can't batch get like summaries easily as it's top likers per anchor
	// For now, we'll do it in a loop or optimize later.
	// Given typical page size is 20, 20 parallel/sequential queries is acceptable for now.
	// To optimize, likes repo would need a "GetRecentLikersForAnchors" aggregation.
	// We'll proceed with sequential for simplicity as specified in algorithm step 7g.

	for _, a := range anchorsList {
		likeSummary, err := s.getLikeSummary(ctx, a.ID, a.LikeCount, followingIDs)
		if err != nil {
			// Log error but continue? For now return error
			return nil, err
		}

		engagementMap[a.ID] = &FeedEngagement{
			HasLiked:    likedMap[a.ID],
			HasCloned:   clonedMap[a.ID],
			LikeSummary: *likeSummary,
		}
	}

	return engagementMap, nil
}

func (s *Service) getPreviewsForAnchors(ctx context.Context, anchorIDs []primitive.ObjectID) (map[primitive.ObjectID]*FeedPreview, error) {
	previewMap := make(map[primitive.ObjectID]*FeedPreview)

	// Similar to like summary, this is N+1 but for 20 items.
	// Optimization: Batch item query with $in anchors and take 3.
	// But simple implementation first.

	for _, id := range anchorIDs {
		items, err := s.feedRepo.GetPreviewItems(ctx, id, 3)
		if err != nil {
			return nil, err
		}

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
					t := item.URLData.Title
					if len(t) > 50 {
						t = t[:50]
					}
					preview.Title = &t
				}
			} else if item.Type == "image" && item.ImageData != nil {
				preview.Thumbnail = &item.ImageData.CloudinaryURL
			} else if item.Type == "text" && item.TextData != nil {
				t := item.TextData.Content
				if len(t) > 100 {
					t = t[:100] + "..."
				}
				preview.Snippet = &t
			}
			previewItems[i] = preview
		}
		previewMap[id] = &FeedPreview{Items: previewItems}
	}

	return previewMap, nil
}

func (s *Service) getLikeSummary(
	ctx context.Context,
	anchorID primitive.ObjectID,
	totalCount int,
	followingIDs []primitive.ObjectID,
) (*FeedLikeSummary, error) {
	// Get recent likers (limit 20 to find matches)
	likers, err := s.likesRepo.GetRecentLikers(ctx, anchorID, 20)
	if err != nil {
		return nil, err
	}

	// Prioritize followed users
	var likedByFollowing []FeedLikeSummaryUser
	followingSet := make(map[primitive.ObjectID]bool)
	for _, id := range followingIDs {
		followingSet[id] = true
	}

	// Filter likers who are followed
	// We need user details for these likers.
	// Gather IDs to fetch
	var likerIDs []primitive.ObjectID
	for _, l := range likers {
		likerIDs = append(likerIDs, l.UserID)
	}

	if len(likerIDs) > 0 {
		likerUsers, err := s.authRepo.GetUsersByIDs(ctx, likerIDs)
		if err != nil {
			return nil, err
		}

		userMap := make(map[primitive.ObjectID]auth.User)
		for _, u := range likerUsers {
			userMap[u.ID] = u
		}

		// Find matches
		count := 0
		for _, l := range likers {
			if count >= 3 {
				break
			}
			if followingSet[l.UserID] {
				if u, ok := userMap[l.UserID]; ok {
					likedByFollowing = append(likedByFollowing, FeedLikeSummaryUser{
						ID:             u.ID,
						Username:       u.Username,
						DisplayName:    u.DisplayName,
						ProfilePicture: &u.ProfilePictureURL,
					})
					count++
				}
			}
		}
	}

	if likedByFollowing == nil {
		likedByFollowing = []FeedLikeSummaryUser{} // Ensure array not null
	}

	otherCount := totalCount - len(likedByFollowing)
	if otherCount < 0 {
		otherCount = 0
	}

	return &FeedLikeSummary{
		TotalCount:       totalCount,
		LikedByFollowing: likedByFollowing,
		OtherLikersCount: otherCount,
	}, nil
}

func (s *Service) GetDiscoverFeed(
	ctx context.Context,
	userID *primitive.ObjectID, // nil if not authenticated
	query *DiscoverQuery,
) (*DiscoverResponse, error) {
	// 1. Decode cursor
	cursor, err := DecodeDiscoverCursor(query.Cursor)
	if err != nil {
		return nil, err
	}

	// 2. Build exclusion list (if authenticated)
	var excludeUserIDs []primitive.ObjectID
	var followingIDs []primitive.ObjectID
	isAuthenticated := userID != nil

	if isAuthenticated {
		followingIDs, err = s.followsRepo.GetAllFollowingIDs(ctx, *userID)
		if err != nil {
			return nil, err
		}
		excludeUserIDs = append(followingIDs, *userID)
	}

	// 3. Get tag pointer
	var tagPtr *string
	if query.Tag != "" {
		tagPtr = &query.Tag
	}

	// 4. Query anchors
	anchorsList, err := s.feedRepo.GetDiscoverAnchors(
		ctx, excludeUserIDs, query.Category, tagPtr, cursor, query.Limit+1,
	)
	if err != nil {
		return nil, err
	}

	// 5. Check hasMore
	hasMore := false
	if len(anchorsList) > query.Limit {
		hasMore = true
		anchorsList = anchorsList[:query.Limit]
	}

	// 6. Handle empty results
	if len(anchorsList) == 0 {
		var emptyReason string
		if query.Tag != "" {
			emptyReason = "NO_TAG_CONTENT"
		} else if cursor != nil {
			emptyReason = "END_OF_FEED"
		} else {
			emptyReason = "NO_CONTENT"
		}

		return &DiscoverResponse{
			Items: []DiscoverItem{},
			Pagination: FeedPagination{
				Limit:     query.Limit,
				HasMore:   false,
				ItemCount: 0,
			},
			Meta: DiscoverMeta{
				FeedType:        "discover",
				Category:        query.Category,
				Tag:             tagPtr,
				IsAuthenticated: isAuthenticated,
				EmptyReason:     &emptyReason,
			},
		}, nil
	}

	// 7. Enrich with authors (includes followerCount)
	authorIDs := make([]primitive.ObjectID, len(anchorsList))
	for i, a := range anchorsList {
		authorIDs[i] = a.UserID
	}
	authorsMap, err := s.enrichDiscoverAuthors(ctx, authorIDs)
	if err != nil {
		return nil, err
	}

	// 8. Enrich with engagement (if authenticated)
	var engagementMap map[primitive.ObjectID]*FeedEngagement
	if isAuthenticated {
		engagementMap, err = s.enrichWithEngagement(ctx, anchorsList, *userID, followingIDs)
		if err != nil {
			return nil, err
		}
	} else {
		// Build empty engagement for unauthenticated
		engagementMap = make(map[primitive.ObjectID]*FeedEngagement)
		for _, a := range anchorsList {
			likeSummary, _ := s.getLikeSummary(ctx, a.ID, a.LikeCount, nil)
			engagementMap[a.ID] = &FeedEngagement{
				HasLiked:    false,
				HasCloned:   false,
				LikeSummary: *likeSummary,
			}
		}
	}

	// 9. Enrich with previews
	anchorIDs := make([]primitive.ObjectID, len(anchorsList))
	for i, a := range anchorsList {
		anchorIDs[i] = a.ID
	}
	previewsMap, err := s.getPreviewsForAnchors(ctx, anchorIDs)
	if err != nil {
		return nil, err
	}

	// 10. Build response items
	items := make([]DiscoverItem, len(anchorsList))
	for i, anchor := range anchorsList {
		author := authorsMap[anchor.UserID]
		if author == nil {
			author = &DiscoverItemAuthor{ID: anchor.UserID, Username: "Unknown"}
		}

		engagement := engagementMap[anchor.ID]
		preview := previewsMap[anchor.ID]
		if preview == nil {
			preview = &FeedPreview{Items: []FeedPreviewItem{}}
		}

		items[i] = DiscoverItem{
			ID:              anchor.ID,
			Title:           anchor.Title,
			Description:     anchor.Description,
			CoverMediaType:  anchor.CoverMediaType,
			CoverMediaValue: anchor.CoverMediaValue,
			Visibility:      anchor.Visibility,
			IsPinned:        anchor.IsPinned,
			Tags:            anchor.Tags,
			ItemCount:       anchor.ItemCount,
			LikeCount:       anchor.LikeCount,
			CloneCount:      anchor.CloneCount,
			CommentCount:    anchor.CommentCount,
			EngagementScore: anchor.EngagementScore,
			LastItemAddedAt: anchor.LastItemAddedAt,
			CreatedAt:       anchor.CreatedAt,
			Author:          *author,
			Engagement:      *engagement,
			Preview:         *preview,
		}
	}

	// 11. Build next cursor
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

// enrichDiscoverAuthors fetches author info with follower count
func (s *Service) enrichDiscoverAuthors(ctx context.Context, userIDs []primitive.ObjectID) (map[primitive.ObjectID]*DiscoverItemAuthor, error) {
	// Deduplicate
	uniqueIDs := make(map[primitive.ObjectID]bool)
	var ids []primitive.ObjectID
	for _, id := range userIDs {
		if !uniqueIDs[id] {
			uniqueIDs[id] = true
			ids = append(ids, id)
		}
	}

	users, err := s.authRepo.GetUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	authorMap := make(map[primitive.ObjectID]*DiscoverItemAuthor)
	for _, user := range users {
		authorMap[user.ID] = &DiscoverItemAuthor{
			ID:             user.ID,
			Username:       user.Username,
			DisplayName:    user.DisplayName,
			ProfilePicture: &user.ProfilePictureURL,
			IsVerified:     user.IsVerified,
			FollowerCount:  user.FollowerCount,
		}
	}
	return authorMap, nil
}
