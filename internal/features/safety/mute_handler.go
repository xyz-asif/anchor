package safety

import (
	"errors"
	"fmt"

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

// currentUser returns the authenticated user, writing a 401 if missing
func currentUser(c *gin.Context) (*auth.User, bool) {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return user, true
		}
	}
	response.Unauthorized(c, "Please sign in first", "UNAUTHORIZED")
	return nil, false
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
	targetID, ok := parseUserID(c)
	if !ok {
		return
	}
	if targetID == user.ID {
		response.BadRequest(c, "You can't mute yourself", "CANNOT_MUTE_SELF")
		return
	}
	if !h.userExists(c, targetID) {
		return
	}
	if err := h.repo.MuteUser(c.Request.Context(), user.ID, targetID); err != nil {
		if errors.Is(err, ErrListFull) {
			response.BadRequest(c, fmt.Sprintf("You've muted %d people, which is the maximum. Unmute someone to mute another person", maxMutedUsers), "MUTE_LIST_FULL")
			return
		}
		h.fail(c, "muting", err)
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
	targetID, ok := parseUserID(c)
	if !ok {
		return
	}
	if err := h.repo.UnmuteUser(c.Request.Context(), user.ID, targetID); err != nil {
		h.fail(c, "unmuting", err)
		return
	}
	response.Success(c, gin.H{"userId": targetID, "isMuted": false}, "User unmuted")
}

// GetMutedUsers godoc
// @Summary List users I have muted
// @Description Most recently muted first
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
	list, err := h.userList(c, user.MutedUsers)
	if err != nil {
		h.fail(c, "loading muted users", err)
		return
	}
	response.Success(c, list)
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
	raw := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		response.BadRequest(c, fmt.Sprintf(`"%s" isn't a valid anchor ID`, raw), "INVALID_ID")
		return
	}
	ctx := c.Request.Context()
	if hidden {
		// Someone else's private anchor looks like it doesn't exist
		ownerID, found, err := h.repo.VisibleTarget(ctx, TargetAnchor, anchorID, user.ID)
		if err != nil {
			h.fail(c, "loading anchor", err)
			return
		}
		if !found {
			response.NotFound(c, "This anchor doesn't exist or was deleted", "ANCHOR_NOT_FOUND")
			return
		}
		if ownerID == user.ID {
			response.BadRequest(c, "This is your own anchor, so it never shows up as a suggestion", "OWN_ANCHOR")
			return
		}
		if err := h.repo.HideAnchor(ctx, user.ID, anchorID); err != nil {
			h.fail(c, "hiding anchor", err)
			return
		}
		response.Success(c, gin.H{"anchorId": anchorID, "isHidden": true}, "You won't see this anchor in your suggestions")
		return
	}
	if err := h.repo.UnhideAnchor(ctx, user.ID, anchorID); err != nil {
		h.fail(c, "unhiding anchor", err)
		return
	}
	response.Success(c, gin.H{"anchorId": anchorID, "isHidden": false}, "This anchor can show up in your suggestions again")
}
