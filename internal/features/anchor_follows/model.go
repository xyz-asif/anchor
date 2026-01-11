package anchor_follows

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AnchorFollow represents a user following an anchor
type AnchorFollow struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID          primitive.ObjectID `bson:"userId" json:"userId"`
	AnchorID        primitive.ObjectID `bson:"anchorId" json:"anchorId"`
	NotifyOnUpdate  bool               `bson:"notifyOnUpdate" json:"notifyOnUpdate"`
	LastSeenVersion int                `bson:"lastSeenVersion" json:"lastSeenVersion"`
	CreatedAt       time.Time          `bson:"createdAt" json:"createdAt"`
}

// FollowAnchorRequest for POST /anchors/:id/follow
type FollowAnchorRequest struct {
	Action         string `json:"action" binding:"required,oneof=follow unfollow"`
	NotifyOnUpdate *bool  `json:"notifyOnUpdate"`
}

// ToggleNotificationsRequest for PATCH /anchors/:id/follow/notifications
type ToggleNotificationsRequest struct {
	NotifyOnUpdate bool `json:"notifyOnUpdate"`
}

// ListFollowingAnchorsQuery for GET /users/me/following-anchors
type ListFollowingAnchorsQuery struct {
	Page       int    `form:"page,default=1"`
	Limit      int    `form:"limit,default=20"`
	HasUpdates *bool  `form:"hasUpdates"`
	Sort       string `form:"sort,default=recent"`
}

// FollowAnchorResponse for follow/unfollow action
type FollowAnchorResponse struct {
	IsFollowing    bool `json:"isFollowing"`
	NotifyOnUpdate bool `json:"notifyOnUpdate"`
	FollowerCount  int  `json:"followerCount"`
}

// FollowStatusResponse for GET /anchors/:id/follow/status
type FollowStatusResponse struct {
	IsFollowing          bool       `json:"isFollowing"`
	NotifyOnUpdate       bool       `json:"notifyOnUpdate"`
	HasUpdates           bool       `json:"hasUpdates"`
	UpdatesSinceLastSeen int        `json:"updatesSinceLastSeen"`
	LastSeenVersion      int        `json:"lastSeenVersion"`
	CurrentVersion       int        `json:"currentVersion"`
	FollowedAt           *time.Time `json:"followedAt"`
}

// FollowingAnchorItem for list response
type FollowingAnchorItem struct {
	ID                   primitive.ObjectID `json:"id"`
	Title                string             `json:"title"`
	Description          string             `json:"description"`
	Visibility           string             `json:"visibility"`
	ItemCount            int                `json:"itemCount"`
	LikeCount            int                `json:"likeCount"`
	FollowerCount        int                `json:"followerCount"`
	Tags                 []string           `json:"tags"`
	HasUpdates           bool               `json:"hasUpdates"`
	UpdatesSinceLastSeen int                `json:"updatesSinceLastSeen"`
	CurrentVersion       int                `json:"currentVersion"`
	LastSeenVersion      int                `json:"lastSeenVersion"`
	LastItemAddedAt      *time.Time         `json:"lastItemAddedAt"`
	NotifyOnUpdate       bool               `json:"notifyOnUpdate"`
	FollowedAt           time.Time          `json:"followedAt"`
	Author               AuthorInfo         `json:"author"`
}

type AuthorInfo struct {
	ID                primitive.ObjectID `json:"id"`
	Username          string             `json:"username"`
	DisplayName       string             `json:"displayName"`
	ProfilePictureUrl *string            `json:"profilePictureUrl"`
	IsVerified        bool               `json:"isVerified"`
}

// Sort constants
const (
	SortRecent       = "recent"
	SortUpdated      = "updated"
	SortAlphabetical = "alphabetical"
)

// ListFollowingAnchorsResponse for list anchors response
type ListFollowingAnchorsResponse struct {
	Data       []FollowingAnchorItem `json:"data"`
	Pagination struct {
		Page       int  `json:"page"`
		Limit      int  `json:"limit"`
		Total      int  `json:"total"`
		TotalPages int  `json:"totalPages"`
		HasMore    bool `json:"hasMore"`
	} `json:"pagination"`
	Meta struct {
		TotalWithUpdates int    `json:"totalWithUpdates"`
		Sort             string `json:"sort"`
	} `json:"meta"`
}
