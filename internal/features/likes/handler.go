package likes

import (
	"context"
	"math"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"

	// "github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Handler handles like-related HTTP requests
type Handler struct {
	repo                *Repository
	anchorsRepo         *anchors.Repository
	authRepo            *auth.Repository
	notificationService *notifications.Service
	followsRepo         *follows.Repository
	config              *config.Config
}

// NewHandler creates a new like handler
func NewHandler(repo *Repository, anchorsRepo *anchors.Repository, authRepo *auth.Repository, notificationService *notifications.Service, followsRepo *follows.Repository, cfg *config.Config) *Handler {
	return &Handler{
		repo:                repo,
		anchorsRepo:         anchorsRepo,
		authRepo:            authRepo,
		notificationService: notificationService,
		followsRepo:         followsRepo,
		config:              cfg,
	}
}

// LikeAction godoc
// @Summary Like or unlike anchor
// @Description Like or unlike an anchor based on the action specified
// @Tags likes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param request body LikeActionRequest true "Like action"
// @Success 200 {object} response.APIResponse{data=LikeActionResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/like [post]
func (h *Handler) LikeAction(c *gin.Context) {
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := user.(*auth.User)

	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID format")
		return
	}

	// Bind request
	var req LikeActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", err.Error())
		return
	}

	// Validate request
	if err := ValidateLikeActionRequest(&req); err != nil {
		response.BadRequest(c, "INVALID_ACTION", err.Error())
		return
	}

	// Get anchor
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check if anchor is deleted
	if anchor.DeletedAt != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check access - user must be owner OR anchor must be public/unlisted
	if !anchor.IsOwnedBy(currentUser.ID) {
		if anchor.Visibility != anchors.VisibilityPublic && anchor.Visibility != anchors.VisibilityUnlisted {
			response.Forbidden(c, "ACCESS_DENIED", "Cannot like private anchor")
			return
		}
	}

	var hasLiked bool

	if req.Action == "like" {
		// Check if already liked (to prevent duplicate notifications)
		wasAlreadyLiked, _ := h.repo.ExistsLike(c.Request.Context(), anchorID, currentUser.ID)

		err = h.repo.CreateLike(c.Request.Context(), anchorID, currentUser.ID)
		if err != nil {
			response.InternalServerError(c, "LIKE_FAILED", "Failed to like anchor")
			return
		}

		// Only increment if this is a new like (CreateLike is idempotent)
		if !wasAlreadyLiked {
			_ = h.anchorsRepo.IncrementLikeCount(c.Request.Context(), anchorID, 1)

			// Create notification (async) - only for NEW likes
			if anchor.UserID != currentUser.ID {
				go func() {
					_ = h.notificationService.CreateLikeNotification(
						context.Background(),
						anchorID,
						anchor.Title,
						currentUser.ID,
						anchor.UserID,
					)
				}()
			}
		}

		// Update engagement score asynchronously
		go func() {
			_ = h.anchorsRepo.UpdateEngagementScore(context.Background(), anchorID)
		}()

		hasLiked = true
	} else {
		// Unlike (idempotent)
		err = h.repo.DeleteLike(c.Request.Context(), anchorID, currentUser.ID)
		if err != nil {
			response.InternalServerError(c, "UNLIKE_FAILED", "Failed to unlike anchor")
			return
		}

		// Decrement anchor's like count
		_ = h.anchorsRepo.IncrementLikeCount(c.Request.Context(), anchorID, -1)

		// Update engagement score asynchronously
		go func() {
			_ = h.anchorsRepo.UpdateEngagementScore(context.Background(), anchorID)
		}()

		hasLiked = false
	}

	// Fetch updated anchor to get current like count
	anchor, _ = h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)

	resp := LikeActionResponse{
		HasLiked:  hasLiked,
		LikeCount: anchor.LikeCount,
	}

	response.Success(c, resp)
}

// GetLikeStatus godoc
// @Summary Check like status
// @Description Check if the current user has liked the anchor
// @Tags likes
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse{data=LikeStatusResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/like/status [get]
func (h *Handler) GetLikeStatus(c *gin.Context) {
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := user.(*auth.User)

	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID format")
		return
	}

	// Get anchor
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check if anchor is deleted
	if anchor.DeletedAt != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check access
	if !anchor.IsOwnedBy(currentUser.ID) {
		if anchor.Visibility != anchors.VisibilityPublic && anchor.Visibility != anchors.VisibilityUnlisted {
			response.Forbidden(c, "ACCESS_DENIED", "Cannot access private anchor")
			return
		}
	}

	// Check if user has liked
	hasLiked, err := h.repo.ExistsLike(c.Request.Context(), anchorID, currentUser.ID)
	if err != nil {
		response.InternalServerError(c, "STATUS_CHECK_FAILED", "Failed to check like status")
		return
	}

	resp := LikeStatusResponse{
		HasLiked:  hasLiked,
		LikeCount: anchor.LikeCount,
	}

	response.Success(c, resp)
}

// ListLikers godoc
// @Summary List users who liked anchor
// @Description Get paginated list of users who liked the anchor
// @Tags likes
// @Produce json
// @Param id path string true "Anchor ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse{data=PaginatedLikersResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/likes [get]
func (h *Handler) ListLikers(c *gin.Context) {
	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID format")
		return
	}

	// Get anchor
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check if anchor is deleted
	if anchor.DeletedAt != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Get current user if authenticated (for access check and isFollowing)
	var currentUserID *primitive.ObjectID
	if user, exists := c.Get("user"); exists {
		currentUser := user.(*auth.User)
		currentUserID = &currentUser.ID
	}

	// Check access
	if currentUserID == nil || !anchor.IsOwnedBy(*currentUserID) {
		if anchor.Visibility != anchors.VisibilityPublic && anchor.Visibility != anchors.VisibilityUnlisted {
			response.Forbidden(c, "ACCESS_DENIED", "Cannot access private anchor")
			return
		}
	}

	// Parse and validate query parameters
	var query LikeListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", err.Error())
		return
	}

	if err := ValidateLikeListQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", err.Error())
		return
	}

	// Get likes
	likes, total, err := h.repo.GetLikers(c.Request.Context(), anchorID, query.Page, query.Limit)
	if err != nil {
		response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch likers")
		return
	}

	// Extract user IDs
	var userIDs []primitive.ObjectID
	for _, like := range likes {
		userIDs = append(userIDs, like.UserID)
	}

	// Fetch user details
	users, err := h.authRepo.GetUsersByIDs(c.Request.Context(), userIDs)
	if err != nil {
		response.InternalServerError(c, "FETCH_USERS_FAILED", "Failed to fetch user details")
		return
	}

	// Create user map for quick lookup
	userMap := make(map[primitive.ObjectID]*auth.User)
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}

	// Get isFollowing status if authenticated
	var followingMap map[primitive.ObjectID]bool
	if currentUserID != nil {
		followingMap, _ = h.followsRepo.GetFollowingIDs(c.Request.Context(), *currentUserID, userIDs)
	}

	// Build response
	var likerUsers []LikerUserResponse
	for _, like := range likes {
		user, exists := userMap[like.UserID]
		if !exists {
			continue
		}

		isFollowing := false
		if followingMap != nil {
			isFollowing = followingMap[like.UserID]
		}

		likerUsers = append(likerUsers, LikerUserResponse{
			ID:             user.ID,
			Username:       user.Username,
			DisplayName:    user.DisplayName,
			ProfilePicture: &user.ProfilePictureURL,
			IsFollowing:    isFollowing,
			LikedAt:        like.CreatedAt,
		})
	}

	// Calculate pagination
	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))

	paginatedResponse := PaginatedLikersResponse{
		Data: likerUsers,
	}
	paginatedResponse.Pagination.Page = query.Page
	paginatedResponse.Pagination.Limit = query.Limit
	paginatedResponse.Pagination.Total = total
	paginatedResponse.Pagination.TotalPages = totalPages
	paginatedResponse.Pagination.HasMore = query.Page < totalPages

	response.Success(c, paginatedResponse)
}

// GetLikeSummary returns like summary with prioritized followed users
func (h *Handler) GetLikeSummary(c *gin.Context, anchorID primitive.ObjectID, currentUserID *primitive.ObjectID) (*LikeSummaryResponse, error) {
	// Get anchor to check like count
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		return nil, err
	}

	// If no likes, return empty summary
	if anchor.LikeCount == 0 {
		return &LikeSummaryResponse{
			TotalCount:       0,
			HasLiked:         false,
			LikedByFollowing: []LikeSummaryUser{},
			OtherLikersCount: 0,
		}, nil
	}

	// Check if current user has liked (if authenticated)
	hasLiked := false
	if currentUserID != nil {
		hasLiked, _ = h.repo.ExistsLike(c.Request.Context(), anchorID, *currentUserID)
	}

	// Get recent likers (limit 20 for processing)
	recentLikes, err := h.repo.GetRecentLikers(c.Request.Context(), anchorID, 20)
	if err != nil {
		return nil, err
	}

	// Extract liker IDs
	var likerIDs []primitive.ObjectID
	for _, like := range recentLikes {
		likerIDs = append(likerIDs, like.UserID)
	}

	var prioritizedIDs []primitive.ObjectID

	if currentUserID == nil {
		// Not authenticated - just take first 3
		if len(likerIDs) > 3 {
			prioritizedIDs = likerIDs[:3]
		} else {
			prioritizedIDs = likerIDs
		}
	} else {
		// Authenticated - prioritize followed users
		followingMap, _ := h.followsRepo.GetFollowingIDs(c.Request.Context(), *currentUserID, likerIDs)

		// Separate into followed and not followed
		var followedLikerIDs []primitive.ObjectID
		var otherLikerIDs []primitive.ObjectID

		for _, id := range likerIDs {
			if followingMap[id] {
				followedLikerIDs = append(followedLikerIDs, id)
			} else {
				otherLikerIDs = append(otherLikerIDs, id)
			}
		}

		// Prioritize: followed first, then others (max 3 total)
		prioritizedIDs = append(followedLikerIDs, otherLikerIDs...)
		if len(prioritizedIDs) > 3 {
			prioritizedIDs = prioritizedIDs[:3]
		}
	}

	// Fetch user details for prioritized users
	users, err := h.authRepo.GetUsersByIDs(c.Request.Context(), prioritizedIDs)
	if err != nil {
		return nil, err
	}

	// Build likedByFollowing list
	var likedByFollowing []LikeSummaryUser
	for _, user := range users {
		likedByFollowing = append(likedByFollowing, LikeSummaryUser{
			ID:             user.ID,
			Username:       user.Username,
			DisplayName:    user.DisplayName,
			ProfilePicture: &user.ProfilePictureURL,
		})
	}

	otherLikersCount := anchor.LikeCount - len(likedByFollowing)
	if otherLikersCount < 0 {
		otherLikersCount = 0
	}

	return &LikeSummaryResponse{
		TotalCount:       anchor.LikeCount,
		HasLiked:         hasLiked,
		LikedByFollowing: likedByFollowing,
		OtherLikersCount: otherLikersCount,
	}, nil
}

// GetLikeSummaryEndpoint godoc
// @Summary Get like summary for anchor
// @Description Get summary of who liked an anchor, prioritizing users you follow
// @Tags likes
// @Produce json
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse{data=LikeSummaryResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/like/summary [get]
func (h *Handler) GetLikeSummaryEndpoint(c *gin.Context) {
	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID format")
		return
	}

	// Get current user if authenticated
	var currentUserID *primitive.ObjectID
	if user, exists := c.Get("user"); exists {
		currentUser := user.(*auth.User)
		currentUserID = &currentUser.ID
	}

	// Get like summary
	summary, err := h.GetLikeSummary(c, anchorID, currentUserID)
	if err != nil {
		response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch like summary")
		return
	}

	response.Success(c, summary)
}
