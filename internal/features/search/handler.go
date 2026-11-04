package search

import (
	"context"
	"log"
	"math"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Handler struct {
	repo        *Repository
	authRepo    *auth.Repository
	followsRepo *follows.Repository
	config      *config.Config
}

func NewHandler(repo *Repository, authRepo *auth.Repository, followsRepo *follows.Repository, cfg *config.Config) *Handler {
	return &Handler{
		repo:        repo,
		authRepo:    authRepo,
		followsRepo: followsRepo,
		config:      cfg,
	}
}

const msgSearchFailed = "Search isn't working right now. Please try again in a moment"

// UnifiedSearch godoc
// @Summary Unified search
// @Description Search across anchors and users
// @Tags search
// @Produce json
// @Param q query string true "Search query (min 2 chars)"
// @Param type query string false "Type: all, anchors, users (default all)"
// @Param limit query int false "Results per type (default 10, max 20)"
// @Success 200 {object} response.APIResponse{data=UnifiedSearchResponse}
// @Failure 400 {object} response.APIResponse
// @Router /search [get]
func (h *Handler) UnifiedSearch(c *gin.Context) {
	query, err := ParseUnifiedQuery(c.Query("q"), c.Query("type"), c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}
	viewer, hidden, ok := h.viewerAndHidden(c)
	if !ok {
		return
	}

	resp := UnifiedSearchResponse{Query: query.Q}
	ctx := c.Request.Context()
	if query.Type == TypeAll || query.Type == TypeAnchors {
		if resp.Anchors, err = h.topAnchors(ctx, query.Q, query.Limit, hidden); err != nil {
			h.failed(c, "anchors", err)
			return
		}
	}
	if query.Type == TypeAll || query.Type == TypeUsers {
		if resp.Users, err = h.topUsers(ctx, query.Q, query.Limit, viewer, hidden); err != nil {
			h.failed(c, "users", err)
			return
		}
	}
	response.Success(c, resp)
}

// SearchAnchors godoc
// @Summary Search anchors
// @Description Search anchors with filters and pagination
// @Tags search
// @Produce json
// @Param q query string true "Search query (min 2 chars)"
// @Param tag query string false "Filter by tag"
// @Param sort query string false "Sort: relevant, recent, popular (default relevant)"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse{data=PaginatedAnchorSearchResponse}
// @Failure 400 {object} response.APIResponse
// @Router /search/anchors [get]
func (h *Handler) SearchAnchors(c *gin.Context) {
	query, err := ParseAnchorSearchQuery(c.Query("q"), c.Query("tag"), c.Query("sort"), c.Query("page"), c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}
	_, hidden, ok := h.viewerAndHidden(c)
	if !ok {
		return
	}

	anchors, total, matchedAll, err := h.repo.SearchAnchors(c.Request.Context(), query.Q, query.Tag, query.Sort, query.Page, query.Limit, hidden)
	if err != nil {
		h.failed(c, "anchors", err)
		return
	}
	anchorResults, err := h.enrichAnchorResults(c.Request.Context(), anchors)
	if err != nil {
		h.failed(c, "anchor authors", err)
		return
	}

	var tagFilter *string
	if query.Tag != "" {
		tagFilter = &query.Tag
	}
	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))
	resp := PaginatedAnchorSearchResponse{
		Anchors: anchorResults,
		Meta: AnchorSearchMeta{
			Query:           query.Q,
			Tag:             tagFilter,
			Sort:            query.Sort,
			MatchedAllWords: matchedAll,
		},
	}
	resp.Pagination.Page = query.Page
	resp.Pagination.Limit = query.Limit
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = totalPages
	resp.Pagination.HasMore = query.Page < totalPages

	response.Success(c, resp)
}

// SearchUsers godoc
// @Summary Search users
// @Description Search users with pagination
// @Tags search
// @Produce json
// @Param q query string true "Search query (min 2 chars)"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse{data=PaginatedUserSearchResponse}
// @Failure 400 {object} response.APIResponse
// @Router /search/users [get]
func (h *Handler) SearchUsers(c *gin.Context) {
	query, err := ParseUserSearchQuery(c.Query("q"), c.Query("page"), c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}
	viewer, hidden, ok := h.viewerAndHidden(c)
	if !ok {
		return
	}

	users, total, matchedAll, err := h.repo.SearchUsers(c.Request.Context(), query.Q, query.Page, query.Limit, hidden)
	if err != nil {
		h.failed(c, "users", err)
		return
	}
	userResults, err := h.enrichUserResults(c.Request.Context(), users, viewer)
	if err != nil {
		h.failed(c, "follow status", err)
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))
	resp := PaginatedUserSearchResponse{
		Users: userResults,
		Meta:  UserSearchMeta{Query: query.Q, MatchedAllWords: matchedAll},
	}
	resp.Pagination.Page = query.Page
	resp.Pagination.Limit = query.Limit
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = totalPages
	resp.Pagination.HasMore = query.Page < totalPages

	response.Success(c, resp)
}

// SearchTags godoc
// @Summary Tag autocomplete
// @Description Get tag suggestions based on prefix
// @Tags search
// @Produce json
// @Param q query string true "Tag prefix (min 1 char)"
// @Param limit query int false "Max suggestions (default 10, max 20)"
// @Success 200 {object} response.APIResponse{data=TagSearchResponse}
// @Failure 400 {object} response.APIResponse
// @Router /search/tags [get]
func (h *Handler) SearchTags(c *gin.Context) {
	query, err := ParseTagSearchQuery(c.Query("q"), c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}

	tags, err := h.repo.SearchTags(c.Request.Context(), query.Q, query.Limit)
	if err != nil {
		h.failed(c, "tags", err)
		return
	}
	response.Success(c, TagSearchResponse{Tags: tags, Query: query.Q})
}

// CombinedSearch godoc
// @Summary Combined search for type-ahead
// @Description Aggregate top results for anchors, users, and tags
// @Tags search
// @Produce json
// @Param q query string true "Search query (min 2 chars)"
// @Param limit query int false "Results per type (default 3, max 10)"
// @Success 200 {object} response.APIResponse{data=UnifiedSearchResponse}
// @Failure 400 {object} response.APIResponse
// @Router /search/combined [get]
func (h *Handler) CombinedSearch(c *gin.Context) {
	q, limit, err := ParseCombinedQuery(c.Query("q"), c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}
	viewer, hidden, ok := h.viewerAndHidden(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	resp := UnifiedSearchResponse{Query: q, Tags: []TagResult{}}
	if resp.Anchors, err = h.topAnchors(ctx, q, limit, hidden); err != nil {
		h.failed(c, "anchors", err)
		return
	}
	if resp.Users, err = h.topUsers(ctx, q, limit, viewer, hidden); err != nil {
		h.failed(c, "users", err)
		return
	}
	// Text like "!!" has no tag in it; skip tags rather than fail the whole type-ahead
	if prefix := tagPrefix(q); prefix != "" && utf8.RuneCountInString(prefix) <= maxTagPrefix {
		if resp.Tags, err = h.repo.SearchTags(ctx, prefix, limit); err != nil {
			h.failed(c, "tags", err)
			return
		}
	}
	response.Success(c, resp)
}

// Helper methods

// viewerAndHidden returns the signed-in user (nil if anonymous) and the users hidden from them:
// people they blocked and people who blocked them. On failure it writes the response.
func (h *Handler) viewerAndHidden(c *gin.Context) (*auth.User, []primitive.ObjectID, bool) {
	val, exists := c.Get("user")
	if !exists {
		return nil, nil, true
	}
	viewer, ok := val.(*auth.User)
	if !ok {
		return nil, nil, true
	}
	hidden, err := h.authRepo.HiddenUserIDs(c.Request.Context(), viewer)
	if err != nil {
		h.failed(c, "blocked users", err)
		return nil, nil, false
	}
	return viewer, hidden, true
}

func (h *Handler) failed(c *gin.Context, what string, err error) {
	log.Printf("search: loading %s failed: %v", what, err)
	response.InternalServerError(c, msgSearchFailed, "SEARCH_FAILED")
}

func (h *Handler) topAnchors(ctx context.Context, q string, limit int, hidden []primitive.ObjectID) (*UnifiedSearchAnchorsResult, error) {
	anchors, total, matchedAll, err := h.repo.SearchAnchors(ctx, q, "", SortRelevant, 1, limit, hidden)
	if err != nil {
		return nil, err
	}
	items, err := h.enrichAnchorResults(ctx, anchors)
	if err != nil {
		return nil, err
	}
	return &UnifiedSearchAnchorsResult{Items: items, Total: total, HasMore: total > int64(limit), MatchedAllWords: matchedAll}, nil
}

func (h *Handler) topUsers(ctx context.Context, q string, limit int, viewer *auth.User, hidden []primitive.ObjectID) (*UnifiedSearchUsersResult, error) {
	users, total, matchedAll, err := h.repo.SearchUsers(ctx, q, 1, limit, hidden)
	if err != nil {
		return nil, err
	}
	items, err := h.enrichUserResults(ctx, users, viewer)
	if err != nil {
		return nil, err
	}
	return &UnifiedSearchUsersResult{Items: items, Total: total, HasMore: total > int64(limit), MatchedAllWords: matchedAll}, nil
}

// enrichAnchorResults adds author details. Anchors whose author no longer exists (an account
// in the middle of being deleted) are left out rather than shown as "Deleted User".
func (h *Handler) enrichAnchorResults(ctx context.Context, anchors []AnchorSearchDoc) ([]SearchAnchorResult, error) {
	results := make([]SearchAnchorResult, 0, len(anchors))
	if len(anchors) == 0 {
		return results, nil
	}

	authorIDs := make([]primitive.ObjectID, len(anchors))
	for i, a := range anchors {
		authorIDs[i] = a.UserID
	}
	authors, err := h.authRepo.GetUsersByIDs(ctx, authorIDs)
	if err != nil {
		return nil, err
	}
	authorMap := make(map[primitive.ObjectID]*auth.User, len(authors))
	for i := range authors {
		authorMap[authors[i].ID] = &authors[i]
	}

	for _, anchor := range anchors {
		author, ok := authorMap[anchor.UserID]
		if !ok {
			continue
		}
		tags := anchor.Tags
		if tags == nil {
			tags = []string{}
		}
		results = append(results, SearchAnchorResult{
			ID:              anchor.ID,
			Title:           anchor.Title,
			Description:     anchor.Description,
			Visibility:      anchor.Visibility,
			ItemCount:       anchor.ItemCount,
			LikeCount:       anchor.LikeCount,
			CommentCount:    anchor.CommentCount,
			CloneCount:      anchor.CloneCount,
			EngagementScore: anchor.EngagementScore,
			Tags:            tags,
			CreatedAt:       anchor.CreatedAt,
			Author: SearchAnchorAuthor{
				ID:             author.ID,
				Username:       author.Username,
				DisplayName:    author.DisplayName,
				ProfilePicture: optionalString(author.ProfilePictureURL),
				IsVerified:     author.IsVerified,
			},
		})
	}
	return results, nil
}

func (h *Handler) enrichUserResults(ctx context.Context, users []UserSearchDoc, viewer *auth.User) ([]SearchUserResult, error) {
	results := make([]SearchUserResult, len(users))
	if len(users) == 0 {
		return results, nil
	}

	followingMap := map[primitive.ObjectID]bool{}
	if viewer != nil {
		userIDs := make([]primitive.ObjectID, len(users))
		for i, u := range users {
			userIDs[i] = u.ID
		}
		var err error
		if followingMap, err = h.followsRepo.GetFollowingStatus(ctx, viewer.ID, userIDs); err != nil {
			return nil, err
		}
	}

	for i, user := range users {
		results[i] = SearchUserResult{
			ID:             user.ID,
			Username:       user.Username,
			DisplayName:    user.DisplayName,
			Bio:            user.Bio,
			ProfilePicture: optionalString(user.ProfilePictureURL),
			FollowerCount:  user.FollowerCount,
			AnchorCount:    user.AnchorCount,
			IsVerified:     user.IsVerified,
			IsFollowing:    viewer != nil && user.ID != viewer.ID && followingMap[user.ID],
		}
	}
	return results, nil
}

// optionalString returns nil for "", so a missing picture is null rather than ""
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
