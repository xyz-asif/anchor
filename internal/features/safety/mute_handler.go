package safety

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MutedUserInfo is the public subset of a muted user returned to the muter
type MutedUserInfo struct {
	ID                primitive.ObjectID `json:"id"`
	Username          string             `json:"username"`
	DisplayName       string             `json:"displayName"`
	ProfilePictureURL string             `json:"profilePictureUrl"`
}

// currentUser returns the authenticated user, writing a 401/500 if missing
func currentUser(c *gin.Context) (*auth.User, bool) {
	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return nil, false
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return nil, false
	}
	return user, true
}

// MuteUser godoc
// @Summary Mute a user
// @Description Muted users' anchors are hidden from your home, discover and tag feeds, and you get no notifications from them. They are not told and can still see your content.
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID to mute"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/mute [post]
func (h *Handler) MuteUser(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	targetID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}
	if targetID == user.ID {
		response.BadRequest(c, "You cannot mute yourself", "CANNOT_MUTE_SELF")
		return
	}
	if target, err := h.authRepo.GetUserByObjectID(c.Request.Context(), targetID); err != nil || target == nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}
	if err := h.repo.MuteUser(c.Request.Context(), user.ID, targetID); err != nil {
		response.InternalServerError(c, "Failed to mute user", "DATABASE_ERROR")
		return
	}
	response.Success(c, gin.H{"userId": targetID, "isMuted": true}, "User muted")
}

// UnmuteUser godoc
// @Summary Unmute a user
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID to unmute"
// @Success 200 {object} response.APIResponse
// @Router /users/{id}/mute [delete]
func (h *Handler) UnmuteUser(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	targetID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}
	if err := h.repo.UnmuteUser(c.Request.Context(), user.ID, targetID); err != nil {
		response.InternalServerError(c, "Failed to unmute user", "DATABASE_ERROR")
		return
	}
	response.Success(c, gin.H{"userId": targetID, "isMuted": false}, "User unmuted")
}

// GetMutedUsers godoc
// @Summary List users I have muted
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=[]MutedUserInfo}
// @Router /users/me/mutes [get]
func (h *Handler) GetMutedUsers(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	result := []MutedUserInfo{}
	if len(user.MutedUsers) > 0 {
		users, err := h.authRepo.GetUsersByIDs(c.Request.Context(), user.MutedUsers)
		if err != nil {
			response.InternalServerError(c, "Failed to fetch muted users", "DATABASE_ERROR")
			return
		}
		for _, u := range users {
			result = append(result, MutedUserInfo{
				ID:                u.ID,
				Username:          u.Username,
				DisplayName:       u.DisplayName,
				ProfilePictureURL: u.ProfilePictureURL,
			})
		}
	}
	response.Success(c, result)
}

// HideAnchor godoc
// @Summary Mark an anchor as "not interested"
// @Description Hides the anchor from your discover and tag feeds.
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse
// @Router /anchors/{id}/hide [post]
func (h *Handler) HideAnchor(c *gin.Context) {
	h.setAnchorHidden(c, true)
}

// UnhideAnchor godoc
// @Summary Undo "not interested" for an anchor
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse
// @Router /anchors/{id}/hide [delete]
func (h *Handler) UnhideAnchor(c *gin.Context) {
	h.setAnchorHidden(c, false)
}

func (h *Handler) setAnchorHidden(c *gin.Context, hidden bool) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	anchorID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}
	if hidden {
		exists, err := h.repo.TargetExists(c.Request.Context(), "anchor", anchorID)
		if err != nil {
			response.InternalServerError(c, "Database error", "DATABASE_ERROR")
			return
		}
		if !exists {
			response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
			return
		}
		err = h.repo.HideAnchor(c.Request.Context(), user.ID, anchorID)
		if err != nil {
			response.InternalServerError(c, "Failed to hide anchor", "DATABASE_ERROR")
			return
		}
	} else if err := h.repo.UnhideAnchor(c.Request.Context(), user.ID, anchorID); err != nil {
		response.InternalServerError(c, "Failed to unhide anchor", "DATABASE_ERROR")
		return
	}
	response.Success(c, gin.H{"anchorId": anchorID, "isHidden": hidden})
}
