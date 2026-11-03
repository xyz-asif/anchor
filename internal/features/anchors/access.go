package anchors

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// IsAccessibleBy reports whether a viewer (nil = logged out) may see this anchor.
// Deleted anchors are visible to no one; private anchors only to their owner;
// public and unlisted anchors to anyone who has the link.
func (a *Anchor) IsAccessibleBy(viewerID *primitive.ObjectID) bool {
	if a.DeletedAt != nil {
		return false
	}
	if a.Visibility == VisibilityPrivate {
		return viewerID != nil && *viewerID == a.UserID
	}
	return true
}

// viewerID returns the authenticated user's ID, or nil when logged out
func viewerID(c *gin.Context) *primitive.ObjectID {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return &user.ID
		}
	}
	return nil
}

// ensureAccess writes the right error response and returns false when the viewer may not
// see the anchor: 404 for deleted, 401/403 for someone else's private anchor.
func ensureAccess(c *gin.Context, anchor *Anchor) bool {
	viewer := viewerID(c)
	if anchor.IsAccessibleBy(viewer) {
		return true
	}
	switch {
	case anchor.DeletedAt != nil:
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
	case viewer == nil:
		response.Unauthorized(c, "This anchor is private", "PRIVATE_ANCHOR")
	default:
		response.Forbidden(c, "You cannot view this private anchor", "PRIVATE_ANCHOR")
	}
	return false
}

// pageParams reads ?page and ?limit, clamping them to page >= 1 and 1 <= limit <= maxLimit
func pageParams(c *gin.Context, defaultLimit, maxLimit int) (page, limit int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err = strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultLimit)))
	if err != nil || limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit
}
