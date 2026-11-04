package anchors

import (
	"context"
	"errors"
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Messages shared by several endpoints
const (
	msgAnchorNotFound = "This anchor doesn't exist or has been deleted"
	msgItemNotFound   = "This item doesn't exist in this anchor (it may have been deleted)"
	msgTryAgain       = "Something went wrong on our side. Please try again in a moment"
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
	if user := currentUser(c); user != nil {
		return &user.ID
	}
	return nil
}

// currentUser returns the authenticated user, or nil when logged out
func currentUser(c *gin.Context) *auth.User {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return user
		}
	}
	return nil
}

// requireUser returns the authenticated user or writes a 401
func requireUser(c *gin.Context, action string) (*auth.User, bool) {
	user := currentUser(c)
	if user == nil {
		response.Unauthorized(c, "Please sign in to "+action, "AUTH_REQUIRED")
		return nil, false
	}
	return user, true
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
		response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
	case viewer == nil:
		response.Unauthorized(c, "This anchor is private. Sign in as its owner to see it", "PRIVATE_ANCHOR")
	default:
		response.Forbidden(c, "This anchor is private: only its owner can see it", "PRIVATE_ANCHOR")
	}
	return false
}

// maxPage bounds ?page so (page-1)*limit can't overflow and skips stay cheap
const maxPage = 10000

// pageParams reads ?page and ?limit, clamping them to 1 <= page <= maxPage and 1 <= limit <= maxLimit
func pageParams(c *gin.Context, defaultLimit, maxLimit int) (page, limit int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
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

// parseObjectID parses a path parameter as an ID, writing a 400 that names it when invalid
func parseObjectID(c *gin.Context, param, what string) (primitive.ObjectID, bool) {
	raw := c.Param(param)
	id, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		response.BadRequest(c, `"`+raw+`" isn't a valid `+what+` ID`, "INVALID_ID")
		return primitive.NilObjectID, false
	}
	return id, true
}

// loadAnchor loads the anchor named by the :id path parameter (deleted ones included)
func (h *Handler) loadAnchor(c *gin.Context) (*Anchor, bool) {
	anchorID, ok := parseObjectID(c, "id", "anchor")
	if !ok {
		return nil, false
	}
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		if errors.Is(err, ErrAnchorNotFound) {
			response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		} else {
			log.Printf("anchors: loading %s failed: %v", anchorID.Hex(), err)
			response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		}
		return nil, false
	}
	return anchor, true
}

// loadViewableAnchor loads an anchor the current viewer may see: not deleted, not someone
// else's private anchor, and not owned by someone who blocked the viewer (reported as 404,
// like their profile).
func (h *Handler) loadViewableAnchor(c *gin.Context) (*Anchor, bool) {
	anchor, ok := h.loadAnchor(c)
	if !ok || !ensureAccess(c, anchor) {
		return nil, false
	}
	if viewer := currentUser(c); viewer != nil && viewer.ID != anchor.UserID {
		blocked, err := h.authRepo.UserHasBlocked(c.Request.Context(), anchor.UserID, viewer.ID)
		if err != nil {
			log.Printf("anchors: block check failed: %v", err)
			response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
			return nil, false
		}
		if blocked {
			response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
			return nil, false
		}
	}
	return anchor, true
}

// loadOwnedAnchor loads a non-deleted anchor that the user owns. Someone else's private anchor
// is reported as not found, so its existence isn't revealed.
func (h *Handler) loadOwnedAnchor(c *gin.Context, user *auth.User) (*Anchor, bool) {
	anchor, ok := h.loadAnchor(c)
	if !ok {
		return nil, false
	}
	if anchor.DeletedAt != nil || (anchor.UserID != user.ID && anchor.Visibility == VisibilityPrivate) {
		response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		return nil, false
	}
	if anchor.UserID != user.ID {
		response.Forbidden(c, "Only the owner of this anchor can change it", "NOT_ANCHOR_OWNER")
		return nil, false
	}
	return anchor, true
}

// loadItemOf loads the item named by :itemId and checks it belongs to the anchor
// (an item of another anchor is reported as not found)
func (h *Handler) loadItemOf(c *gin.Context, anchorID primitive.ObjectID) (*Item, bool) {
	itemID, ok := parseObjectID(c, "itemId", "item")
	if !ok {
		return nil, false
	}
	item, err := h.repo.GetItemByID(c.Request.Context(), itemID)
	if err != nil && !errors.Is(err, ErrItemNotFound) {
		log.Printf("anchors: loading item %s failed: %v", itemID.Hex(), err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return nil, false
	}
	if err != nil || item.AnchorID != anchorID {
		response.NotFound(c, msgItemNotFound, "ITEM_NOT_FOUND")
		return nil, false
	}
	return item, true
}

// hideCloneSources removes clonedFromAnchorId/clonedFromUserId from anchors the viewer doesn't
// own when the viewer couldn't open the original: an unlisted anchor's ID is its secret share
// link, and a private or deleted original is nobody else's business.
func (h *Handler) hideCloneSources(ctx context.Context, viewer *auth.User, list []*Anchor) error {
	var originalIDs []primitive.ObjectID
	for _, a := range list {
		if a.ClonedFromAnchorID != nil && (viewer == nil || viewer.ID != a.UserID) {
			originalIDs = append(originalIDs, *a.ClonedFromAnchorID)
		}
	}
	if len(originalIDs) == 0 {
		return nil
	}
	originals, err := h.repo.GetAnchorsByIDs(ctx, originalIDs) // excludes deleted ones
	if err != nil {
		return err
	}
	visible := map[primitive.ObjectID]bool{}
	for i := range originals {
		o := &originals[i]
		visible[o.ID] = o.Visibility == VisibilityPublic || (viewer != nil && viewer.ID == o.UserID)
	}
	for _, a := range list {
		if a.ClonedFromAnchorID != nil && (viewer == nil || viewer.ID != a.UserID) && !visible[*a.ClonedFromAnchorID] {
			a.ClonedFromAnchorID = nil
			a.ClonedFromUserID = nil
		}
	}
	return nil
}
