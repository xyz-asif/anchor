package anchor_follows

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
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
	currentUser, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	user := currentUser.(*auth.User)

	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	var req FollowAnchorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", "Invalid request format")
		return
	}

	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	if req.Action != "follow" && req.Action != "unfollow" {
		response.BadRequest(c, "INVALID_ACTION", "Action must be 'follow' or 'unfollow'")
		return
	}

	ctx := c.Request.Context()

	// Get anchor
	anchor, err := h.anchorsRepo.GetAnchorByID(ctx, anchorID)
	if err != nil || anchor == nil || anchor.DeletedAt != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Cannot follow own anchor
	if anchor.UserID == user.ID {
		response.BadRequest(c, "CANNOT_FOLLOW_OWN", "Cannot follow your own anchor")
		return
	}

	// Cannot follow private anchor
	if anchor.Visibility == "private" {
		response.Forbidden(c, "ACCESS_DENIED", "Cannot follow private anchor")
		return
	}

	var isFollowing bool
	var notifyOnUpdate bool

	if req.Action == "follow" {
		notifyOnUpdate = false
		if req.NotifyOnUpdate != nil {
			notifyOnUpdate = *req.NotifyOnUpdate
		}

		// Check if already following
		existingFollow, _ := h.repo.GetFollow(ctx, user.ID, anchorID)
		if existingFollow == nil {
			// New follow
			follow := &AnchorFollow{
				UserID:          user.ID,
				AnchorID:        anchorID,
				NotifyOnUpdate:  notifyOnUpdate,
				LastSeenVersion: anchor.Version,
			}

			if err := h.repo.CreateFollow(ctx, follow); err != nil {
				response.InternalServerError(c, "FOLLOW_FAILED", "Failed to follow anchor")
				return
			}

			// Increment follower count
			_ = h.anchorsRepo.IncrementFollowerCount(ctx, anchorID, 1)
		} else {
			// Update notification preference if specified
			if req.NotifyOnUpdate != nil {
				_ = h.repo.UpdateNotifyOnUpdate(ctx, user.ID, anchorID, notifyOnUpdate)
			} else {
				notifyOnUpdate = existingFollow.NotifyOnUpdate
			}
		}

		isFollowing = true
	} else {
		// Unfollow
		existingFollow, _ := h.repo.GetFollow(ctx, user.ID, anchorID)
		if existingFollow != nil {
			if err := h.repo.DeleteFollow(ctx, user.ID, anchorID); err != nil {
				response.InternalServerError(c, "UNFOLLOW_FAILED", "Failed to unfollow anchor")
				return
			}
			// Decrement follower count
			_ = h.anchorsRepo.IncrementFollowerCount(ctx, anchorID, -1)
		}

		isFollowing = false
		notifyOnUpdate = false
	}

	// Get updated follower count
	followerCount, _ := h.repo.CountFollowers(ctx, anchorID)

	response.Success(c, FollowAnchorResponse{
		IsFollowing:    isFollowing,
		NotifyOnUpdate: notifyOnUpdate,
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
	currentUser, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	user := currentUser.(*auth.User)

	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	ctx := c.Request.Context()

	// Get anchor for current version
	anchor, err := h.anchorsRepo.GetAnchorByID(ctx, anchorID)
	if err != nil || anchor == nil || anchor.DeletedAt != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Get follow relationship
	follow, _ := h.repo.GetFollow(ctx, user.ID, anchorID)

	resp := FollowStatusResponse{
		CurrentVersion: anchor.Version,
	}

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
	currentUser, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	user := currentUser.(*auth.User)

	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	var req ToggleNotificationsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", "Invalid request format")
		return
	}

	ctx := c.Request.Context()

	// Check if following
	follow, _ := h.repo.GetFollow(ctx, user.ID, anchorID)
	if follow == nil {
		response.BadRequest(c, "NOT_FOLLOWING", "You are not following this anchor")
		return
	}

	// Update
	if err := h.repo.UpdateNotifyOnUpdate(ctx, user.ID, anchorID, req.NotifyOnUpdate); err != nil {
		response.InternalServerError(c, "UPDATE_FAILED", "Failed to update notification preference")
		return
	}

	response.Success(c, gin.H{
		"notifyOnUpdate": req.NotifyOnUpdate,
	})
}

// ListFollowingAnchors godoc
// @Summary List anchors user is following
// @Description Get all anchors the current user is following with update status
// @Tags anchor-follows
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Param hasUpdates query bool false "Filter only anchors with updates"
// @Param sort query string false "Sort: recent, updated, alphabetical"
// @Success 200 {object} response.APIResponse{data=ListFollowingAnchorsResponse}
// @Router /users/me/following-anchors [get]
func (h *Handler) ListFollowingAnchors(c *gin.Context) {
	currentUser, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	user := currentUser.(*auth.User)

	// Parse query
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	sortBy := strings.ToLower(c.DefaultQuery("sort", "recent"))
	hasUpdatesStr := c.Query("hasUpdates")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	var filterHasUpdates *bool
	if hasUpdatesStr != "" {
		val := hasUpdatesStr == "true"
		filterHasUpdates = &val
	}

	ctx := c.Request.Context()

	// Get follows
	follows, total, err := h.repo.GetUserFollowingAnchors(ctx, user.ID, page, limit)
	if err != nil {
		response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch following anchors")
		return
	}

	if len(follows) == 0 {
		response.Success(c, gin.H{
			"data": []interface{}{},
			"pagination": gin.H{
				"page":       page,
				"limit":      limit,
				"total":      0,
				"totalPages": 0,
				"hasMore":    false,
			},
			"meta": gin.H{
				"totalWithUpdates": 0,
				"sort":             sortBy,
			},
		})
		return
	}

	// Collect anchor IDs
	anchorIDs := make([]primitive.ObjectID, len(follows))
	for i, f := range follows {
		anchorIDs[i] = f.AnchorID
	}

	// Batch fetch anchors
	anchorsList, _ := h.anchorsRepo.GetAnchorsByIDs(ctx, anchorIDs)
	anchorsMap := make(map[primitive.ObjectID]*anchors.Anchor)
	for i := range anchorsList {
		anchorsMap[anchorsList[i].ID] = &anchorsList[i]
	}

	// Collect author IDs
	authorIDs := make([]primitive.ObjectID, 0)
	for _, anchor := range anchorsMap {
		authorIDs = append(authorIDs, anchor.UserID)
	}

	// Batch fetch authors
	authors, _ := h.authRepo.GetUsersByIDs(ctx, authorIDs)
	authorsMap := make(map[primitive.ObjectID]*auth.User)
	for i := range authors {
		authorsMap[authors[i].ID] = &authors[i]
	}

	// Build response items
	items := make([]FollowingAnchorItem, 0)
	totalWithUpdates := 0

	for _, follow := range follows {
		anchor, ok := anchorsMap[follow.AnchorID]
		if !ok || anchor == nil || anchor.DeletedAt != nil {
			continue
		}

		hasUpdates := anchor.Version > follow.LastSeenVersion
		updatesSinceLastSeen := anchor.Version - follow.LastSeenVersion
		if updatesSinceLastSeen < 0 {
			updatesSinceLastSeen = 0
		}

		if hasUpdates {
			totalWithUpdates++
		}

		// Filter by hasUpdates if specified
		if filterHasUpdates != nil && *filterHasUpdates && !hasUpdates {
			continue
		}

		// Build author
		var authorInfo AuthorInfo
		if author, ok := authorsMap[anchor.UserID]; ok {
			var profilePic *string
			if author.ProfilePictureURL != "" {
				profilePic = &author.ProfilePictureURL
			}
			authorInfo = AuthorInfo{
				ID:                author.ID,
				Username:          author.Username,
				DisplayName:       author.DisplayName,
				ProfilePictureUrl: profilePic,
				IsVerified:        author.IsVerified,
			}
		}

		tags := anchor.Tags
		if tags == nil {
			tags = []string{}
		}

		items = append(items, FollowingAnchorItem{
			ID:                   anchor.ID,
			Title:                anchor.Title,
			Description:          anchor.Description,
			Visibility:           anchor.Visibility,
			ItemCount:            anchor.ItemCount,
			LikeCount:            anchor.LikeCount,
			FollowerCount:        anchor.FollowerCount,
			Tags:                 tags,
			HasUpdates:           hasUpdates,
			UpdatesSinceLastSeen: updatesSinceLastSeen,
			CurrentVersion:       anchor.Version,
			LastSeenVersion:      follow.LastSeenVersion,
			LastItemAddedAt:      &anchor.LastItemAddedAt,
			NotifyOnUpdate:       follow.NotifyOnUpdate,
			FollowedAt:           follow.CreatedAt,
			Author:               authorInfo,
		})
	}

	// Sort items
	switch sortBy {
	case SortUpdated:
		sort.Slice(items, func(i, j int) bool {
			if items[i].LastItemAddedAt == nil {
				return false
			}
			if items[j].LastItemAddedAt == nil {
				return true
			}
			return items[i].LastItemAddedAt.After(*items[j].LastItemAddedAt)
		})
	case SortAlphabetical:
		sort.Slice(items, func(i, j int) bool {
			return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	response.Success(c, gin.H{
		"data": items,
		"pagination": gin.H{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": totalPages,
			"hasMore":    page < totalPages,
		},
		"meta": gin.H{
			"totalWithUpdates": totalWithUpdates,
			"sort":             sortBy,
		},
	})
}
