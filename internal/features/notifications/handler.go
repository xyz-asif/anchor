package notifications

import (
	"context"
	"math"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ContentProvider defines interface to fetch content details (e.g. anchor titles)
type ContentProvider interface {
	GetAnchorTitles(ctx context.Context, anchorIDs []primitive.ObjectID) (map[primitive.ObjectID]string, error)
}

type Handler struct {
	repo            *Repository
	authRepo        *auth.Repository
	contentProvider ContentProvider
	config          *config.Config
}

func NewHandler(repo *Repository, authRepo *auth.Repository, contentProvider ContentProvider, cfg *config.Config) *Handler {
	return &Handler{
		repo:            repo,
		authRepo:        authRepo,
		contentProvider: contentProvider,
		config:          cfg,
	}
}

// ListNotifications godoc
// @Summary List notifications
// @Description Get paginated list of user's notifications
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Param unreadOnly query bool false "Only show unread"
// @Success 200 {object} response.APIResponse{data=PaginatedNotificationsResponse}
// @Failure 401 {object} response.APIResponse
// @Router /notifications [get]
func (h *Handler) ListNotifications(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	var query NotificationListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, "Invalid query parameters", "INVALID_QUERY")
		return
	}

	if err := ValidateNotificationListQuery(&query); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}

	notifications, total, err := h.repo.GetUserNotifications(
		c.Request.Context(),
		currentUser.ID,
		query.UnreadOnly,
		query.Page,
		query.Limit,
	)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch notifications", "FETCH_FAILED")
		return
	}

	// Enrich notifications
	enrichedNotifications := h.enrichNotifications(c.Request.Context(), notifications)

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))

	resp := PaginatedNotificationsResponse{
		Notifications: enrichedNotifications,
	}
	resp.Pagination.Page = query.Page
	resp.Pagination.Limit = query.Limit
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = totalPages
	resp.Pagination.HasMore = query.Page < totalPages

	response.Success(c, resp)
}

// GetUnreadCount godoc
// @Summary Get unread notification count
// @Description Get count of unread notifications
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=UnreadCountResponse}
// @Failure 401 {object} response.APIResponse
// @Router /notifications/unread-count [get]
func (h *Handler) GetUnreadCount(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	count, err := h.repo.CountUnread(c.Request.Context(), currentUser.ID)
	if err != nil {
		response.InternalServerError(c, "Failed to count notifications", "COUNT_FAILED")
		return
	}

	response.Success(c, UnreadCountResponse{UnreadCount: count})
}

// MarkAsRead godoc
// @Summary Mark notification as read
// @Description Mark a single notification as read
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param id path string true "Notification ID"
// @Success 200 {object} response.APIResponse{data=MarkReadResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /notifications/{id}/read [patch]
func (h *Handler) MarkAsRead(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	notificationIDStr := c.Param("id")
	notificationID, err := primitive.ObjectIDFromHex(notificationIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid notification ID", "INVALID_ID")
		return
	}

	// Get notification to verify ownership
	notification, err := h.repo.GetNotificationByID(c.Request.Context(), notificationID)
	if err != nil {
		response.NotFound(c, "Notification not found", "NOT_FOUND")
		return
	}

	// Verify ownership
	if notification.RecipientID != currentUser.ID {
		response.Forbidden(c, "Cannot mark others' notifications", "FORBIDDEN")
		return
	}

	if err := h.repo.MarkAsRead(c.Request.Context(), notificationID); err != nil {
		response.InternalServerError(c, "Failed to mark as read", "UPDATE_FAILED")
		return
	}

	response.Success(c, MarkReadResponse{
		ID:     notificationID,
		IsRead: true,
	})
}

// MarkAllAsRead godoc
// @Summary Mark all notifications as read
// @Description Mark all user's notifications as read
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=MarkAllReadResponse}
// @Failure 401 {object} response.APIResponse
// @Router /notifications/read-all [patch]
func (h *Handler) MarkAllAsRead(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	count, err := h.repo.MarkAllAsRead(c.Request.Context(), currentUser.ID)
	if err != nil {
		response.InternalServerError(c, "Failed to mark all as read", "UPDATE_FAILED")
		return
	}

	response.Success(c, MarkAllReadResponse{MarkedCount: count})
}

// DeleteNotification godoc
// @Summary Delete a notification
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param id path string true "Notification ID"
// @Success 200 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /notifications/{id} [delete]
func (h *Handler) DeleteNotification(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	notificationID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid notification ID", "INVALID_ID")
		return
	}

	// Ownership is enforced in the query: other users' notifications look "not found"
	deleted, err := h.repo.DeleteForRecipient(c.Request.Context(), notificationID, currentUser.ID)
	if err != nil {
		response.InternalServerError(c, "Failed to delete notification", "DELETE_FAILED")
		return
	}
	if !deleted {
		response.NotFound(c, "Notification not found", "NOT_FOUND")
		return
	}

	response.Success(c, gin.H{"id": notificationID, "deleted": true})
}

// ClearNotifications godoc
// @Summary Clear notifications
// @Description Deletes all your notifications, or only the read ones with ?readOnly=true
// @Tags notifications
// @Produce json
// @Security BearerAuth
// @Param readOnly query bool false "Only delete notifications already marked read"
// @Success 200 {object} response.APIResponse{data=DeleteNotificationsResponse}
// @Router /notifications [delete]
func (h *Handler) ClearNotifications(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	readOnly := c.Query("readOnly") == "true"
	count, err := h.repo.DeleteAllForRecipient(c.Request.Context(), currentUser.ID, readOnly)
	if err != nil {
		response.InternalServerError(c, "Failed to clear notifications", "DELETE_FAILED")
		return
	}

	response.Success(c, DeleteNotificationsResponse{DeletedCount: count})
}

// Helper to enrich notifications with actor and anchor data
func (h *Handler) enrichNotifications(ctx context.Context, notifications []Notification) []NotificationResponse {
	if len(notifications) == 0 {
		return []NotificationResponse{}
	}

	// Collect actor and anchor IDs
	actorIDs := make([]primitive.ObjectID, 0)
	anchorIDs := make([]primitive.ObjectID, 0)
	actorIDSet := make(map[primitive.ObjectID]bool)
	anchorIDSet := make(map[primitive.ObjectID]bool)

	for _, n := range notifications {
		if !actorIDSet[n.ActorID] {
			actorIDSet[n.ActorID] = true
			actorIDs = append(actorIDs, n.ActorID)
		}
		if n.AnchorID != nil && !anchorIDSet[*n.AnchorID] {
			anchorIDSet[*n.AnchorID] = true
			anchorIDs = append(anchorIDs, *n.AnchorID)
		}
	}

	// Batch fetch actors
	actors, _ := h.authRepo.GetUsersByIDs(ctx, actorIDs)
	actorMap := make(map[primitive.ObjectID]*auth.User)
	for i := range actors {
		actorMap[actors[i].ID] = &actors[i]
	}

	// Batch fetch anchor titles
	var titleMap map[primitive.ObjectID]string
	if len(anchorIDs) > 0 {
		titleMap, _ = h.contentProvider.GetAnchorTitles(ctx, anchorIDs)
	}
	if titleMap == nil {
		titleMap = make(map[primitive.ObjectID]string)
	}

	// Build enriched responses
	responses := make([]NotificationResponse, len(notifications))
	for i, n := range notifications {
		resp := NotificationResponse{
			ID:           n.ID,
			Type:         n.Type,
			ResourceType: n.ResourceType,
			ResourceID:   n.ResourceID,
			AnchorID:     n.AnchorID,
			Preview:      n.Preview,
			IsRead:       n.IsRead,
			CreatedAt:    n.CreatedAt,
		}

		// Add actor
		if actor, ok := actorMap[n.ActorID]; ok {
			var profilePic *string
			if actor.ProfilePictureURL != "" {
				profilePic = &actor.ProfilePictureURL
			}
			resp.Actor = NotificationActor{
				ID:             actor.ID,
				Username:       actor.Username,
				DisplayName:    actor.DisplayName,
				ProfilePicture: profilePic,
			}
		} else {
			resp.Actor = NotificationActor{
				ID:          n.ActorID,
				Username:    "deleted",
				DisplayName: "Deleted User",
			}
		}

		// Add anchor
		if n.AnchorID != nil {
			if title, ok := titleMap[*n.AnchorID]; ok {
				resp.Anchor = &NotificationAnchor{
					ID:    *n.AnchorID,
					Title: title,
				}
			}
		}

		responses[i] = resp
	}

	return responses
}
