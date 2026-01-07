package notifications

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Notification type constants
const (
	TypeMention = "mention"
	TypeComment = "comment"
	TypeLike    = "like"
	TypeFollow  = "follow"
	TypeClone   = "clone"
)

// Notification represents a user notification
type Notification struct {
	ID           primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	RecipientID  primitive.ObjectID  `bson:"recipientId" json:"recipientId"`
	ActorID      primitive.ObjectID  `bson:"actorId" json:"actorId"`
	Type         string              `bson:"type" json:"type"`
	ResourceType string              `bson:"resourceType" json:"resourceType"`
	ResourceID   primitive.ObjectID  `bson:"resourceId" json:"resourceId"`
	AnchorID     *primitive.ObjectID `bson:"anchorId,omitempty" json:"anchorId,omitempty"`
	Preview      string              `bson:"preview" json:"preview"`
	IsRead       bool                `bson:"isRead" json:"isRead"`
	CreatedAt    time.Time           `bson:"createdAt" json:"createdAt"`
}

// Request DTOs

type NotificationListQuery struct {
	Page       int  `form:"page,default=1" binding:"min=1"`
	Limit      int  `form:"limit,default=20" binding:"min=1,max=50"`
	UnreadOnly bool `form:"unreadOnly"`
}

// Response DTOs

type NotificationActor struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	ProfilePicture *string            `json:"profilePicture"`
}

type NotificationAnchor struct {
	ID    primitive.ObjectID `json:"id"`
	Title string             `json:"title"`
}

type NotificationResponse struct {
	ID           primitive.ObjectID  `json:"id"`
	Type         string              `json:"type"`
	ResourceType string              `json:"resourceType"`
	ResourceID   primitive.ObjectID  `json:"resourceId"`
	AnchorID     *primitive.ObjectID `json:"anchorId,omitempty"`
	Preview      string              `json:"preview"`
	IsRead       bool                `json:"isRead"`
	CreatedAt    time.Time           `json:"createdAt"`
	Actor        NotificationActor   `json:"actor"`
	Anchor       *NotificationAnchor `json:"anchor,omitempty"`
}

type PaginatedNotificationsResponse struct {
	Notifications []NotificationResponse `json:"notifications"`
	Pagination    struct {
		Page       int   `json:"page"`
		Limit      int   `json:"limit"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"totalPages"`
		HasMore    bool  `json:"hasMore"`
	} `json:"pagination"`
}

type UnreadCountResponse struct {
	UnreadCount int64 `json:"unreadCount"`
}

type MarkReadResponse struct {
	ID     primitive.ObjectID `json:"id"`
	IsRead bool               `json:"isRead"`
}

type MarkAllReadResponse struct {
	MarkedCount int64 `json:"markedCount"`
}
