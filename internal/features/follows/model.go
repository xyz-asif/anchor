package follows

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Follow represents a follow relationship between two users
type Follow struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	FollowerID  primitive.ObjectID `bson:"followerId" json:"followerId"`   // User who is following
	FollowingID primitive.ObjectID `bson:"followingId" json:"followingId"` // User being followed
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
}

// FollowActionRequest for POST /users/:id/follow
type FollowActionRequest struct {
	Action string `json:"action" binding:"required,oneof=follow unfollow"`
}

// FollowListQuery for GET /users/:id/follows
type FollowListQuery struct {
	Type  string `form:"type" binding:"required,oneof=followers following"`
	Page  int    `form:"page,default=1" binding:"min=1"`
	Limit int    `form:"limit,default=20" binding:"min=1,max=50"`
}

// FollowActionResponse after follow/unfollow
type FollowActionResponse struct {
	IsFollowing bool                  `json:"isFollowing"`
	TargetUser  FollowTargetUserInfo  `json:"targetUser"`
	CurrentUser FollowCurrentUserInfo `json:"currentUser"`
}

// FollowTargetUserInfo contains target user info in follow action response
type FollowTargetUserInfo struct {
	ID            primitive.ObjectID `json:"id"`
	Username      string             `json:"username"`
	DisplayName   string             `json:"displayName"`
	FollowerCount int                `json:"followerCount"`
}

// FollowCurrentUserInfo contains current user info in follow action response
type FollowCurrentUserInfo struct {
	FollowingCount int `json:"followingCount"`
}

// FollowStatusResponse for GET /users/:id/follow/status
type FollowStatusResponse struct {
	IsFollowing  bool `json:"isFollowing"`
	IsFollowedBy bool `json:"isFollowedBy"`
	IsMutual     bool `json:"isMutual"`
}

// FollowUserResponse for items in followers/following list
type FollowUserResponse struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	ProfilePicture *string            `json:"profilePicture"`
	Bio            string             `json:"bio"`
	IsFollowing    bool               `json:"isFollowing"`
	FollowedAt     time.Time          `json:"followedAt"`
}

// PaginatedFollowResponse represents paginated follow list
type PaginatedFollowResponse struct {
	Data       []FollowUserResponse `json:"data"`
	Pagination struct {
		Page       int   `json:"page"`
		Limit      int   `json:"limit"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"totalPages"`
		HasMore    bool  `json:"hasMore"`
	} `json:"pagination"`
}
