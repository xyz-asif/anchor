package search

import (
	"context"
	"math"

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
	var query UnifiedSearchQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", "Invalid query parameters")
		return
	}

	if err := ValidateUnifiedSearchQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", err.Error())
		return
	}

	// Get current user if authenticated
	var currentUserID *primitive.ObjectID
	if usr, exists := c.Get("user"); exists {
		if user, ok := usr.(*auth.User); ok {
			currentUserID = &user.ID
		}
	}

	resp := UnifiedSearchResponse{
		Query: query.Q,
	}

	// Search anchors
	if query.Type == TypeAll || query.Type == TypeAnchors {
		anchors, total, err := h.repo.SearchAnchors(c.Request.Context(), query.Q, nil, SortRelevant, 1, query.Limit)
		if err == nil {
			anchorResults := h.enrichAnchorResults(c.Request.Context(), anchors)
			resp.Anchors = &UnifiedSearchAnchorsResult{
				Items:   anchorResults,
				Total:   total,
				HasMore: total > int64(query.Limit),
			}
		} else {
			resp.Anchors = &UnifiedSearchAnchorsResult{
				Items:   []SearchAnchorResult{},
				Total:   0,
				HasMore: false,
			}
		}
	}

	// Search users
	if query.Type == TypeAll || query.Type == TypeUsers {
		users, total, err := h.repo.SearchUsers(c.Request.Context(), query.Q, 1, query.Limit)
		if err == nil {
			userResults := h.enrichUserResults(c.Request.Context(), users, currentUserID)
			resp.Users = &UnifiedSearchUsersResult{
				Items:   userResults,
				Total:   total,
				HasMore: total > int64(query.Limit),
			}
		} else {
			resp.Users = &UnifiedSearchUsersResult{
				Items:   []SearchUserResult{},
				Total:   0,
				HasMore: false,
			}
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
	var query AnchorSearchQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", "Invalid query parameters")
		return
	}

	if err := ValidateAnchorSearchQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", err.Error())
		return
	}

	// Prepare tag filter
	var tagFilter *string
	if query.Tag != "" {
		tagFilter = &query.Tag
	}

	// Search
	anchors, total, err := h.repo.SearchAnchors(c.Request.Context(), query.Q, tagFilter, query.Sort, query.Page, query.Limit)
	if err != nil {
		response.InternalServerError(c, "SEARCH_FAILED", "Failed to search anchors")
		return
	}

	// Enrich results
	anchorResults := h.enrichAnchorResults(c.Request.Context(), anchors)

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))

	resp := PaginatedAnchorSearchResponse{
		Anchors: anchorResults,
		Meta: AnchorSearchMeta{
			Query: query.Q,
			Tag:   tagFilter,
			Sort:  query.Sort,
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
	var query UserSearchQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", "Invalid query parameters")
		return
	}

	if err := ValidateUserSearchQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", err.Error())
		return
	}

	// Get current user if authenticated
	var currentUserID *primitive.ObjectID
	if usr, exists := c.Get("user"); exists {
		if user, ok := usr.(*auth.User); ok {
			currentUserID = &user.ID
		}
	}

	// Search
	users, total, err := h.repo.SearchUsers(c.Request.Context(), query.Q, query.Page, query.Limit)
	if err != nil {
		response.InternalServerError(c, "SEARCH_FAILED", "Failed to search users")
		return
	}

	// Enrich results
	userResults := h.enrichUserResults(c.Request.Context(), users, currentUserID)

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))

	resp := PaginatedUserSearchResponse{
		Users: userResults,
		Meta: UserSearchMeta{
			Query: query.Q,
		},
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
	var query TagSearchQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", "Invalid query parameters")
		return
	}

	if err := ValidateTagSearchQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", err.Error())
		return
	}

	// Search tags
	tags, err := h.repo.SearchTags(c.Request.Context(), query.Q, query.Limit)
	if err != nil {
		response.InternalServerError(c, "SEARCH_FAILED", "Failed to search tags")
		return
	}

	response.Success(c, TagSearchResponse{
		Tags:  tags,
		Query: query.Q,
	})
}

// Helper methods

func (h *Handler) enrichAnchorResults(ctx context.Context, anchors []AnchorSearchDoc) []SearchAnchorResult {
	if len(anchors) == 0 {
		return []SearchAnchorResult{}
	}

	// Collect author IDs
	authorIDs := make([]primitive.ObjectID, len(anchors))
	for i, a := range anchors {
		authorIDs[i] = a.UserID
	}

	// Batch fetch authors
	authors, _ := h.authRepo.GetUsersByIDs(ctx, authorIDs)
	authorMap := make(map[primitive.ObjectID]*auth.User)
	for i := range authors {
		authorMap[authors[i].ID] = &authors[i]
	}

	// Build results
	results := make([]SearchAnchorResult, len(anchors))
	for i, anchor := range anchors {
		var authorResult SearchAnchorAuthor
		if author, ok := authorMap[anchor.UserID]; ok {
			var profilePic *string
			if author.ProfilePictureURL != "" {
				profilePic = &author.ProfilePictureURL
			}
			authorResult = SearchAnchorAuthor{
				ID:             author.ID,
				Username:       author.Username,
				DisplayName:    author.DisplayName,
				ProfilePicture: profilePic,
				IsVerified:     author.IsVerified,
			}
		} else {
			authorResult = SearchAnchorAuthor{
				ID:          anchor.UserID,
				Username:    "deleted",
				DisplayName: "Deleted User",
			}
		}

		tags := anchor.Tags
		if tags == nil {
			tags = []string{}
		}

		results[i] = SearchAnchorResult{
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
			Author:          authorResult,
		}
	}

	return results
}

func (h *Handler) enrichUserResults(ctx context.Context, users []UserSearchDoc, currentUserID *primitive.ObjectID) []SearchUserResult {
	if len(users) == 0 {
		return []SearchUserResult{}
	}

	// Batch fetch follow status if authenticated
	var followingMap map[primitive.ObjectID]bool
	if currentUserID != nil {
		userIDs := make([]primitive.ObjectID, len(users))
		for i, u := range users {
			userIDs[i] = u.ID
		}
		followingMap, _ = h.followsRepo.GetFollowingStatus(ctx, *currentUserID, userIDs)
	}

	// Build results
	results := make([]SearchUserResult, len(users))
	for i, user := range users {
		var profilePic *string
		if user.ProfilePictureURL != "" {
			profilePic = &user.ProfilePictureURL
		}

		isFollowing := false
		if followingMap != nil && currentUserID != nil && user.ID != *currentUserID {
			isFollowing = followingMap[user.ID]
		}

		results[i] = SearchUserResult{
			ID:             user.ID,
			Username:       user.Username,
			DisplayName:    user.DisplayName,
			Bio:            user.Bio,
			ProfilePicture: profilePic,
			FollowerCount:  user.FollowerCount,
			AnchorCount:    user.AnchorCount,
			IsVerified:     user.IsVerified,
			IsFollowing:    isFollowing,
		}
	}

	return results
}
