package anchor_follows

import (
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/bind"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

const (
	msgAnchorNotFound = "This anchor doesn't exist or has been deleted"
	msgTryAgain       = "Something went wrong on our side. Please try again in a moment"
	maxListPage       = 10000
)

type Handler struct {
	repo        *Repository
	anchorsRepo *anchors.Repository
	authRepo    *auth.Repository
	config      *config.Config
}

func NewHandler(repo *Repository, anchorsRepo *anchors.Repository, authRepo *auth.Repository, cfg *config.Config) *Handler {
	return &Handler{
		repo:        repo,
		anchorsRepo: anchorsRepo,
		authRepo:    authRepo,
		config:      cfg,
	}
}

// currentUser returns the authenticated user (routes here always require auth)
func currentUser(c *gin.Context) (*auth.User, bool) {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return user, true
		}
	}
	response.Unauthorized(c, "Please sign in to follow anchors", "UNAUTHORIZED")
	return nil, false
}

// loadAnchor parses :id and loads the anchor (deleted ones included)
func (h *Handler) loadAnchor(c *gin.Context) (*anchors.Anchor, bool) {
	raw := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		response.BadRequest(c, `"`+raw+`" isn't a valid anchor ID`, "INVALID_ID")
		return nil, false
	}
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		if errors.Is(err, anchors.ErrAnchorNotFound) {
			response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		} else {
			log.Printf("anchor_follows: loading anchor %s failed: %v", raw, err)
			response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		}
		return nil, false
	}
	return anchor, true
}

// visibleTo reports whether user may see the anchor: it exists, isn't someone else's private
// anchor, and its owner hasn't blocked the user. A failed lookup writes a 500 (ok=false).
func (h *Handler) visibleTo(c *gin.Context, anchor *anchors.Anchor, user *auth.User) (visible, ok bool) {
	if !anchor.IsAccessibleBy(&user.ID) {
		return false, true
	}
	if anchor.UserID == user.ID {
		return true, true
	}
	blocked, err := h.authRepo.UserHasBlocked(c.Request.Context(), anchor.UserID, user.ID)
	if err != nil {
		log.Printf("anchor_follows: block check failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return false, false
	}
	return !blocked, true
}

// FollowAnchor godoc
// @Summary Follow or unfollow an anchor
// @Description Follow or unfollow an anchor to track updates
// @Tags anchor-follows
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param request body FollowAnchorRequest true "Follow action"
// @Success 200 {object} response.APIResponse{data=FollowAnchorResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/follow [post]
func (h *Handler) FollowAnchor(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}

	var req FollowAnchorRequest
	if !bind.JSON(c, &req) {
		return
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	if req.Action != "follow" && req.Action != "unfollow" {
		response.BadRequest(c, fmt.Sprintf(`"%s" isn't a valid action. Use "follow" or "unfollow"`, req.Action), "INVALID_ACTION")
		return
	}

	anchor, ok := h.loadAnchor(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	if req.Action == "unfollow" {
		// Unfollowing always works, even if the anchor was deleted, went private, or its
		// owner blocked you, so nobody gets stuck following something they can't open
		deleted, err := h.repo.DeleteFollow(ctx, user.ID, anchor.ID)
		if err != nil {
			log.Printf("anchor_follows: unfollow failed: %v", err)
			response.InternalServerError(c, "We couldn't unfollow this anchor right now. Please try again", "UNFOLLOW_FAILED")
			return
		}
		if deleted {
			if err := h.anchorsRepo.IncrementFollowerCount(ctx, anchor.ID, -1); err != nil {
				log.Printf("anchor_follows: followerCount -1 failed: %v", err)
			}
		}
		h.respondWithCount(c, anchor.ID, false, false)
		return
	}

	visible, ok := h.visibleTo(c, anchor, user)
	if !ok {
		return
	}
	if !visible {
		// Private, deleted, or the owner blocked you: don't reveal which
		response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		return
	}
	if anchor.UserID == user.ID {
		response.BadRequest(c, "This is your own anchor. You'll always see its updates, so there's no need to follow it", "CANNOT_FOLLOW_OWN")
		return
	}
	if anchor.Visibility == anchors.VisibilityPrivate {
		response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		return
	}
	if user.HasBlocked(anchor.UserID) {
		response.Forbidden(c, "You've blocked the owner of this anchor. Unblock them first if you want to follow it", "USER_BLOCKED")
		return
	}

	existing, err := h.repo.GetFollow(ctx, user.ID, anchor.ID)
	if err != nil {
		log.Printf("anchor_follows: follow lookup failed: %v", err)
		response.InternalServerError(c, "We couldn't follow this anchor right now. Please try again", "FOLLOW_FAILED")
		return
	}

	notify := false
	if existing != nil {
		notify = existing.NotifyOnUpdate
		if req.NotifyOnUpdate != nil && *req.NotifyOnUpdate != notify {
			if err := h.repo.UpdateNotifyOnUpdate(ctx, user.ID, anchor.ID, *req.NotifyOnUpdate); err != nil && !errors.Is(err, ErrNotFollowing) {
				log.Printf("anchor_follows: notify update failed: %v", err)
				response.InternalServerError(c, "We couldn't change your notification setting right now. Please try again", "UPDATE_FAILED")
				return
			}
			notify = *req.NotifyOnUpdate
		}
		h.respondWithCount(c, anchor.ID, true, notify)
		return
	}

	if req.NotifyOnUpdate != nil {
		notify = *req.NotifyOnUpdate
	}
	created, err := h.repo.CreateFollow(ctx, &AnchorFollow{
		UserID:          user.ID,
		AnchorID:        anchor.ID,
		NotifyOnUpdate:  notify,
		LastSeenVersion: anchor.Version,
	})
	if err != nil {
		log.Printf("anchor_follows: follow failed: %v", err)
		response.InternalServerError(c, "We couldn't follow this anchor right now. Please try again", "FOLLOW_FAILED")
		return
	}
	if created {
		// Re-check the block after inserting, in case the owner blocked this user meanwhile
		if blocked, err := h.authRepo.IsBlockedBetween(ctx, user, anchor.UserID); err != nil || blocked {
			if _, delErr := h.repo.DeleteFollow(ctx, user.ID, anchor.ID); delErr != nil {
				log.Printf("anchor_follows: rolling back follow failed: %v", delErr)
			}
			if err != nil {
				log.Printf("anchor_follows: block re-check failed: %v", err)
				response.InternalServerError(c, "We couldn't follow this anchor right now. Please try again", "FOLLOW_FAILED")
			} else {
				response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
			}
			return
		}
		// Only the request that actually inserted the follow increments the count
		if err := h.anchorsRepo.IncrementFollowerCount(ctx, anchor.ID, 1); err != nil {
			log.Printf("anchor_follows: followerCount +1 failed: %v", err)
		}
	}

	h.respondWithCount(c, anchor.ID, true, notify)
}

// respondWithCount answers a follow action with the anchor's real follower count
func (h *Handler) respondWithCount(c *gin.Context, anchorID primitive.ObjectID, isFollowing, notify bool) {
	followerCount, err := h.repo.CountFollowers(c.Request.Context(), anchorID)
	if err != nil {
		log.Printf("anchor_follows: counting followers failed: %v", err)
	}
	response.Success(c, FollowAnchorResponse{
		IsFollowing:    isFollowing,
		NotifyOnUpdate: notify,
		FollowerCount:  int(followerCount),
	})
}

// GetFollowStatus godoc
// @Summary Get follow status for an anchor
// @Description Check if current user is following an anchor and get update status
// @Tags anchor-follows
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse{data=FollowStatusResponse}
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/follow/status [get]
func (h *Handler) GetFollowStatus(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	anchor, ok := h.loadAnchor(c)
	if !ok {
		return
	}
	// Someone else's private anchor, a deleted one, or one whose owner blocked you is
	// reported as not found (don't reveal it exists)
	visible, ok := h.visibleTo(c, anchor, user)
	if !ok {
		return
	}
	if !visible {
		response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		return
	}

	follow, err := h.repo.GetFollow(c.Request.Context(), user.ID, anchor.ID)
	if err != nil {
		log.Printf("anchor_follows: follow lookup failed: %v", err)
		response.InternalServerError(c, "We couldn't check whether you follow this anchor. Please try again", "DATABASE_ERROR")
		return
	}

	resp := FollowStatusResponse{CurrentVersion: anchor.Version}
	if follow != nil {
		resp.IsFollowing = true
		resp.NotifyOnUpdate = follow.NotifyOnUpdate
		resp.LastSeenVersion = follow.LastSeenVersion
		resp.HasUpdates = anchor.Version > follow.LastSeenVersion
		resp.UpdatesSinceLastSeen = anchor.Version - follow.LastSeenVersion
		if resp.UpdatesSinceLastSeen < 0 {
			resp.UpdatesSinceLastSeen = 0
		}
		resp.FollowedAt = &follow.CreatedAt
	}

	response.Success(c, resp)
}

// ToggleNotifications godoc
// @Summary Toggle update notifications for followed anchor
// @Description Enable or disable notifications when anchor is updated
// @Tags anchor-follows
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param request body ToggleNotificationsRequest true "Notification preference"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Router /anchors/{id}/follow/notifications [patch]
func (h *Handler) ToggleNotifications(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	raw := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		response.BadRequest(c, `"`+raw+`" isn't a valid anchor ID`, "INVALID_ID")
		return
	}

	var req ToggleNotificationsRequest
	if !bind.JSON(c, &req) {
		return
	}
	if req.NotifyOnUpdate == nil {
		// Missing used to mean false, silently turning notifications off
		response.BadRequest(c, `Please send "notifyOnUpdate": true or false`, "INVALID_REQUEST")
		return
	}

	// Turning notifications off always works; turning them on only for anchors you can see
	if *req.NotifyOnUpdate {
		anchor, ok := h.loadAnchor(c)
		if !ok {
			return
		}
		visible, ok := h.visibleTo(c, anchor, user)
		if !ok {
			return
		}
		if !visible {
			response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
			return
		}
	}

	if err := h.repo.UpdateNotifyOnUpdate(c.Request.Context(), user.ID, anchorID, *req.NotifyOnUpdate); err != nil {
		if errors.Is(err, ErrNotFollowing) {
			response.BadRequest(c, "You don't follow this anchor. Follow it first to get update notifications", "NOT_FOLLOWING")
			return
		}
		log.Printf("anchor_follows: notify update failed: %v", err)
		response.InternalServerError(c, "We couldn't change your notification setting right now. Please try again", "UPDATE_FAILED")
		return
	}

	response.Success(c, gin.H{"notifyOnUpdate": *req.NotifyOnUpdate})
}

// ListFollowingAnchors godoc
// @Summary List anchors user is following
// @Description Get all anchors the current user is following with update status
// @Tags anchor-follows
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Param hasUpdates query bool false "true: only anchors with updates you haven't seen"
// @Param sort query string false "Sort: recent (default), updated, alphabetical"
// @Success 200 {object} response.APIResponse{data=ListFollowingAnchorsResponse}
// @Router /users/me/following-anchors [get]
func (h *Handler) ListFollowingAnchors(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}

	opts, errMsg := parseListQuery(c)
	if errMsg != "" {
		response.BadRequest(c, errMsg, "INVALID_QUERY")
		return
	}
	ctx := c.Request.Context()

	hidden, err := h.authRepo.HiddenUserIDs(ctx, user)
	if err != nil {
		log.Printf("anchor_follows: loading blocks failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return
	}
	opts.HiddenUsers = hidden

	rows, total, totalWithUpdates, err := h.repo.ListFollowing(ctx, user.ID, opts)
	if err != nil {
		log.Printf("anchor_follows: listing followed anchors failed: %v", err)
		response.InternalServerError(c, "We couldn't load the anchors you follow right now. Please try again", "FETCH_FAILED")
		return
	}

	authorIDs := make([]primitive.ObjectID, 0, len(rows))
	for _, r := range rows {
		authorIDs = append(authorIDs, r.Anchor.UserID)
	}
	authors, err := h.authRepo.GetUsersByIDs(ctx, authorIDs)
	if err != nil {
		log.Printf("anchor_follows: loading authors failed: %v", err)
		response.InternalServerError(c, "We couldn't load the anchors you follow right now. Please try again", "FETCH_FAILED")
		return
	}
	authorsMap := make(map[primitive.ObjectID]*auth.User, len(authors))
	for i := range authors {
		authorsMap[authors[i].ID] = &authors[i]
	}

	items := make([]FollowingAnchorItem, 0, len(rows))
	for _, r := range rows {
		a, f := r.Anchor, r.Follow
		updates := a.Version - f.LastSeenVersion
		if updates < 0 {
			updates = 0
		}

		var authorInfo AuthorInfo
		if author, ok := authorsMap[a.UserID]; ok {
			var profilePic *string
			if author.ProfilePictureURL != "" {
				pic := author.ProfilePictureURL
				profilePic = &pic
			}
			authorInfo = AuthorInfo{
				ID:                author.ID,
				Username:          author.Username,
				DisplayName:       author.DisplayName,
				ProfilePictureUrl: profilePic,
				IsVerified:        author.IsVerified,
			}
		}

		tags := a.Tags
		if tags == nil {
			tags = []string{}
		}
		lastItemAddedAt := a.LastItemAddedAt

		items = append(items, FollowingAnchorItem{
			ID:                   a.ID,
			Title:                a.Title,
			Description:          a.Description,
			Visibility:           a.Visibility,
			ItemCount:            a.ItemCount,
			LikeCount:            a.LikeCount,
			FollowerCount:        a.FollowerCount,
			Tags:                 tags,
			HasUpdates:           updates > 0,
			UpdatesSinceLastSeen: updates,
			CurrentVersion:       a.Version,
			LastSeenVersion:      f.LastSeenVersion,
			LastItemAddedAt:      &lastItemAddedAt,
			NotifyOnUpdate:       f.NotifyOnUpdate,
			FollowedAt:           f.CreatedAt,
			Author:               authorInfo,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(opts.Limit)))
	response.Success(c, gin.H{
		"data": items,
		"pagination": gin.H{
			"page":       opts.Page,
			"limit":      opts.Limit,
			"total":      total,
			"totalPages": totalPages,
			"hasMore":    opts.Page < totalPages,
		},
		"meta": gin.H{
			"totalWithUpdates": totalWithUpdates,
			"sort":             opts.Sort,
		},
	})
}

// parseListQuery reads page/limit/sort/hasUpdates, returning a readable message for bad values
func parseListQuery(c *gin.Context) (FollowingListOptions, string) {
	o := FollowingListOptions{Page: 1, Limit: 20, Sort: SortRecent}

	if v := c.Query("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return o, fmt.Sprintf(`Page must be a whole number of 1 or more (got "%s")`, v)
		}
		if n > maxListPage {
			return o, fmt.Sprintf("Page can't be higher than %d", maxListPage)
		}
		o.Page = n
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return o, fmt.Sprintf(`Limit must be a whole number between 1 and 50 (got "%s")`, v)
		}
		if n > 50 {
			n = 50
		}
		o.Limit = n
	}
	if v := strings.ToLower(strings.TrimSpace(c.Query("sort"))); v != "" {
		switch v {
		case SortRecent, SortUpdated, SortAlphabetical:
			o.Sort = v
		default:
			return o, fmt.Sprintf(`"%s" isn't a valid sort. Use recent, updated or alphabetical`, c.Query("sort"))
		}
	}
	if v := strings.ToLower(strings.TrimSpace(c.Query("hasUpdates"))); v != "" {
		switch v {
		case "true", "1":
			o.OnlyUpdated = true
		case "false", "0":
		default:
			return o, fmt.Sprintf(`hasUpdates must be true or false (got "%s")`, c.Query("hasUpdates"))
		}
	}
	return o, ""
}
