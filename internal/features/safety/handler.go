package safety

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Handler struct {
	repo     *Repository
	authRepo *auth.Repository
	cfg      *config.Config
}

func NewHandler(repo *Repository, authRepo *auth.Repository, cfg *config.Config) *Handler {
	return &Handler{
		repo:     repo,
		authRepo: authRepo,
		cfg:      cfg,
	}
}

// @Summary Report content or user
// @Tags safety
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateReportRequest true "Report details"
// @Success 201 {object} response.APIResponse
// @Router /reports [post]
func (h *Handler) CreateReport(c *gin.Context) {
	var req CreateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", "INVALID_JSON")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	targetID, err := primitive.ObjectIDFromHex(req.TargetID)
	if err != nil {
		response.BadRequest(c, "Invalid target ID", "INVALID_ID")
		return
	}

	ctx := c.Request.Context()

	if req.TargetType == "user" && targetID == user.ID {
		response.BadRequest(c, "You cannot report yourself", "CANNOT_REPORT_SELF")
		return
	}

	targetExists, err := h.repo.TargetExists(ctx, req.TargetType, targetID)
	if err != nil {
		response.InternalServerError(c, "Database error", "DATABASE_ERROR")
		return
	}
	if !targetExists {
		response.NotFound(c, "Reported "+req.TargetType+" not found", "TARGET_NOT_FOUND")
		return
	}

	alreadyReported, err := h.repo.HasOpenReport(ctx, user.ID, targetID)
	if err != nil {
		response.InternalServerError(c, "Database error", "DATABASE_ERROR")
		return
	}
	if alreadyReported {
		response.Conflict(c, "You have already reported this", "ALREADY_REPORTED")
		return
	}

	report := &Report{
		ReporterID: user.ID,
		TargetID:   targetID,
		TargetType: req.TargetType,
		Reason:     req.Reason,
	}

	if err := h.repo.CreateReport(ctx, report); err != nil {
		response.InternalServerError(c, "Failed to create report", "DATABASE_ERROR")
		return
	}

	response.Created(c, gin.H{"id": report.ID, "status": report.Status}, "Report submitted")
}

// @Summary Block a user
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID to block"
// @Success 200 {object} response.APIResponse
// @Router /users/{id}/block [post]
func (h *Handler) BlockUser(c *gin.Context) {
	targetIDStr := c.Param("id")
	targetID, err := primitive.ObjectIDFromHex(targetIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	if targetID == user.ID {
		response.BadRequest(c, "You cannot block yourself", "CANNOT_BLOCK_SELF")
		return
	}

	ctx := c.Request.Context()
	if target, err := h.authRepo.GetUserByObjectID(ctx, targetID); err != nil || target == nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	if err := h.repo.BlockUser(ctx, user.ID, targetID); err != nil {
		response.InternalServerError(c, "Failed to block user", "DATABASE_ERROR")
		return
	}

	// Blocking ends any follow relationship in both directions
	if err := h.repo.RemoveFollowsBetween(ctx, user.ID, targetID); err != nil {
		log.Printf("Failed to remove follows between %s and %s: %v", user.ID.Hex(), targetID.Hex(), err)
	}

	response.Success(c, "User blocked successfully")
}

// @Summary Unblock a user
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID to unblock"
// @Success 200 {object} response.APIResponse
// @Router /users/{id}/block [delete]
func (h *Handler) UnblockUser(c *gin.Context) {
	targetIDStr := c.Param("id")
	targetID, err := primitive.ObjectIDFromHex(targetIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	if targetID == user.ID {
		response.BadRequest(c, "Cannot unblock yourself", "INVALID_ACTION")
		return
	}

	if err := h.repo.RemoveBlock(c.Request.Context(), user.ID, targetID); err != nil {
		response.InternalServerError(c, "Failed to unblock user", "DATABASE_ERROR")
		return
	}

	response.Success(c, gin.H{
		"message": "User unblocked successfully",
	})
}

// GetBlockedUsers returns the list of users blocked by the current user
// @Summary Get blocked users
// @Description Get list of blocked users
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=[]auth.User}
// @Failure 401 {object} response.APIResponse
// @Router /users/me/blocks [get]
func (h *Handler) GetBlockedUsers(c *gin.Context) {
	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	currentUser, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	if len(currentUser.BlockedUsers) == 0 {
		response.Success(c, []MutedUserInfo{})
		return
	}

	// Fetch user details for blocked IDs
	// We need authRepo for this. Currently Handler only has safety.Repository.
	// We need to inject authRepo or add method to safety repo to fetch users?
	// Better to inject authRepo into Handler.
	if h.authRepo == nil {
		response.InternalServerError(c, "Auth repository not injected", "INTERNAL_ERROR")
		return
	}

	users, err := h.authRepo.GetUsersByIDs(c.Request.Context(), currentUser.BlockedUsers)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch blocked users", "DATABASE_ERROR")
		return
	}

	// Public fields only: never expose blocked users' email, Google ID or block lists
	result := make([]MutedUserInfo, 0, len(users))
	for _, u := range users {
		result = append(result, MutedUserInfo{
			ID:                u.ID,
			Username:          u.Username,
			DisplayName:       u.DisplayName,
			ProfilePictureURL: u.ProfilePictureURL,
		})
	}
	response.Success(c, result)
}
