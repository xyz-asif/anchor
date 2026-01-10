package safety

import (
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

	report := &Report{
		ReporterID: user.ID,
		TargetID:   targetID,
		TargetType: req.TargetType,
		Reason:     req.Reason,
	}

	if err := h.repo.CreateReport(c.Request.Context(), report); err != nil {
		response.InternalServerError(c, "Failed to create report", "DATABASE_ERROR")
		return
	}

	response.Created(c, "Report submitted")
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

	if err := h.repo.BlockUser(c.Request.Context(), user.ID, targetID); err != nil {
		response.InternalServerError(c, "Failed to block user", "DATABASE_ERROR")
		return
	}

	response.Success(c, "User blocked successfully")
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
		response.BadRequest(c, "User context error", "INTERNAL_ERROR")
		return
	}

	if len(currentUser.BlockedUsers) == 0 {
		response.Success(c, []auth.User{})
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

	// Respond with public profile data only? For blocking management, maybe full user object is overkill but okay.
	// Or define a lightweight DTO. For now, returning User is consistent with other generic endpoints.
	response.Success(c, users)
}
