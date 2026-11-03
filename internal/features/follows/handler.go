package follows

import (
	"context"
	"math"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications" // Already present, but good to confirm
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Handler handles follow-related HTTP requests
type Handler struct {
	repo                *Repository
	authRepo            *auth.Repository
	notificationService *notifications.Service // Added
	config              *config.Config
}

// NewHandler creates a new follow handler
func NewHandler(repo *Repository, authRepo *auth.Repository, notificationService *notifications.Service, cfg *config.Config) *Handler { // Modified
	return &Handler{
		repo:                repo,
		authRepo:            authRepo,
		notificationService: notificationService, // Added
		config:              cfg,
	}
}

// FollowAction godoc
// @Summary Follow or unfollow user
// @Description Follow or unfollow a user based on the action specified
// @Tags follows
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Target user's ID"
// @Param request body FollowActionRequest true "Follow action"
// @Success 200 {object} response.APIResponse{data=FollowActionResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/follow [post]
func (h *Handler) FollowAction(c *gin.Context) {
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := user.(*auth.User)

	// Get target user ID from path
	targetIDStr := c.Param("id")
	targetID, err := primitive.ObjectIDFromHex(targetIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID format", "INVALID_ID")
		return
	}

	// Bind request
	var req FollowActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, `action is required and must be "follow" or "unfollow"`, "INVALID_REQUEST")
		return
	}

	// Validate request
	if err := ValidateFollowActionRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_ACTION")
		return
	}

	// Check if trying to follow self
	if currentUser.ID == targetID {
		response.BadRequest(c, "You cannot follow yourself", "CANNOT_FOLLOW_SELF")
		return
	}

	ctx := c.Request.Context()

	// Get target user
	targetUser, err := h.authRepo.GetUserByID(ctx, targetID.Hex())
	if err != nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	var isFollowing bool

	if req.Action == "follow" {
		// Blocked users (either direction) can't follow each other
		if blocked, err := h.authRepo.IsBlockedBetween(ctx, currentUser, targetID); err != nil {
			response.InternalServerError(c, "Failed to follow user", "FOLLOW_FAILED")
			return
		} else if blocked {
			response.Forbidden(c, "You can't follow this user", "USER_BLOCKED")
			return
		}

		// Only the request that actually inserted the follow updates counts and notifies
		created, err := h.repo.CreateFollow(ctx, currentUser.ID, targetID)
		if err != nil {
			response.InternalServerError(c, "Failed to follow user", "FOLLOW_FAILED")
			return
		}
		if created {
			_ = h.authRepo.IncrementFollowerCount(ctx, targetID, 1)
			_ = h.authRepo.IncrementFollowingCount(ctx, currentUser.ID, 1)

			go func(actorID, target primitive.ObjectID) {
				_ = h.notificationService.CreateFollowNotification(context.Background(), actorID, target)
			}(currentUser.ID, targetID)
		}
		isFollowing = true
	} else {
		deleted, err := h.repo.DeleteFollow(ctx, currentUser.ID, targetID)
		if err != nil {
			response.InternalServerError(c, "Failed to unfollow user", "UNFOLLOW_FAILED")
			return
		}
		// Only decrement when a follow was actually removed
		if deleted {
			_ = h.authRepo.IncrementFollowerCount(ctx, targetID, -1)
			_ = h.authRepo.IncrementFollowingCount(ctx, currentUser.ID, -1)
		}
		isFollowing = false
	}

	// Fetch updated counts (keep the earlier values if a re-read fails)
	if updated, err := h.authRepo.GetUserByID(ctx, targetID.Hex()); err == nil && updated != nil {
		targetUser = updated
	}
	if updated, err := h.authRepo.GetUserByID(ctx, currentUser.ID.Hex()); err == nil && updated != nil {
		currentUser = updated
	}

	// Build response
	resp := FollowActionResponse{
		IsFollowing: isFollowing,
		TargetUser: FollowTargetUserInfo{
			ID:            targetUser.ID,
			Username:      targetUser.Username,
			DisplayName:   targetUser.DisplayName,
			FollowerCount: targetUser.FollowerCount,
		},
		CurrentUser: FollowCurrentUserInfo{
			FollowingCount: currentUser.FollowingCount,
		},
	}

	response.Success(c, resp)
}

// GetFollowStatus godoc
// @Summary Check follow status
// @Description Check if the current user follows the target user, and if the target follows back
// @Tags follows
// @Produce json
// @Security BearerAuth
// @Param id path string true "Target user's ID"
// @Success 200 {object} response.APIResponse{data=FollowStatusResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/follow/status [get]
func (h *Handler) GetFollowStatus(c *gin.Context) {
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := user.(*auth.User)

	// Get target user ID from path
	targetIDStr := c.Param("id")
	targetID, err := primitive.ObjectIDFromHex(targetIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID format", "INVALID_ID")
		return
	}

	// Check if target user exists
	_, err = h.authRepo.GetUserByID(c.Request.Context(), targetID.Hex())
	if err != nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	// Get follow status in both directions
	isFollowing, isFollowedBy, err := h.repo.GetFollowStatus(c.Request.Context(), currentUser.ID, targetID)
	if err != nil {
		response.InternalServerError(c, "Failed to check follow status", "STATUS_CHECK_FAILED")
		return
	}

	resp := FollowStatusResponse{
		IsFollowing:  isFollowing,
		IsFollowedBy: isFollowedBy,
		IsMutual:     isFollowing && isFollowedBy,
	}

	response.Success(c, resp)
}

// ListFollows godoc
// @Summary List followers or following
// @Description Get paginated list of followers OR users being followed
// @Tags follows
// @Produce json
// @Param id path string true "Target user's ID"
// @Param type query string true "Type: followers or following"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse{data=PaginatedFollowResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/follows [get]
func (h *Handler) ListFollows(c *gin.Context) {
	// Get target user ID from path ("me" resolves to the authenticated user)
	targetIDStr := c.Param("id")
	if targetIDStr == "me" {
		val, exists := c.Get("user")
		if !exists {
			response.Unauthorized(c, "Authentication required", "AUTH_REQUIRED")
			return
		}
		me, ok := val.(*auth.User)
		if !ok {
			response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
			return
		}
		targetIDStr = me.ID.Hex()
	}
	targetID, err := primitive.ObjectIDFromHex(targetIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID format", "INVALID_ID")
		return
	}

	// Check if target user exists
	_, err = h.authRepo.GetUserByID(c.Request.Context(), targetID.Hex())
	if err != nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	// Parse and validate query parameters
	var query FollowListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}

	if err := ValidateFollowListQuery(&query); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_TYPE")
		return
	}

	// Get current user if authenticated (for isFollowing field)
	var currentUserID *primitive.ObjectID
	if user, exists := c.Get("user"); exists {
		currentUser := user.(*auth.User)
		currentUserID = &currentUser.ID
	}

	// Fetch follows based on type
	var follows []Follow
	var total int64

	if query.Type == "followers" {
		follows, total, err = h.repo.GetFollowers(c.Request.Context(), targetID, query.Page, query.Limit)
	} else {
		follows, total, err = h.repo.GetFollowing(c.Request.Context(), targetID, query.Page, query.Limit)
	}

	if err != nil {
		response.InternalServerError(c, "Failed to fetch follows", "FETCH_FAILED")
		return
	}

	// Extract user IDs to fetch
	var userIDs []primitive.ObjectID
	for _, follow := range follows {
		if query.Type == "followers" {
			userIDs = append(userIDs, follow.FollowerID)
		} else {
			userIDs = append(userIDs, follow.FollowingID)
		}
	}

	// Fetch user details
	users, err := h.authRepo.GetUsersByIDs(c.Request.Context(), userIDs)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch user details", "FETCH_USERS_FAILED")
		return
	}

	// Create user map for quick lookup
	userMap := make(map[primitive.ObjectID]*auth.User)
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}

	// Get isFollowing status for all users if authenticated
	var followingMap map[primitive.ObjectID]bool
	if currentUserID != nil {
		followingMap, _ = h.repo.GetFollowingIDs(c.Request.Context(), *currentUserID, userIDs)
	}

	// Build response
	var followUsers []FollowUserResponse
	for _, follow := range follows {
		var userID primitive.ObjectID
		if query.Type == "followers" {
			userID = follow.FollowerID
		} else {
			userID = follow.FollowingID
		}

		user, exists := userMap[userID]
		if !exists {
			continue
		}

		isFollowing := false
		if followingMap != nil {
			isFollowing = followingMap[userID]
		}

		followUsers = append(followUsers, FollowUserResponse{
			ID:             user.ID,
			Username:       user.Username,
			DisplayName:    user.DisplayName,
			ProfilePicture: &user.ProfilePictureURL,
			Bio:            user.Bio,
			IsFollowing:    isFollowing,
			FollowedAt:     follow.CreatedAt,
		})
	}

	// Calculate pagination
	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))

	paginatedResponse := PaginatedFollowResponse{
		Data: followUsers,
	}
	paginatedResponse.Pagination.Page = query.Page
	paginatedResponse.Pagination.Limit = query.Limit
	paginatedResponse.Pagination.Total = total
	paginatedResponse.Pagination.TotalPages = totalPages
	paginatedResponse.Pagination.HasMore = query.Page < totalPages

	response.Success(c, paginatedResponse)
}
