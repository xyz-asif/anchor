package likes

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Like represents a like on an anchor
type Like struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	AnchorID  primitive.ObjectID `bson:"anchorId" json:"anchorId"`
	UserID    primitive.ObjectID `bson:"userId" json:"userId"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

// LikeActionRequest for POST /anchors/:id/like
type LikeActionRequest struct {
	Action string `json:"action" binding:"required,oneof=like unlike"`
}

// LikeListQuery for GET /anchors/:id/likes
type LikeListQuery struct {
	Page  int `form:"page,default=1" binding:"min=1"`
	Limit int `form:"limit,default=20" binding:"min=1,max=50"`
}

// LikeActionResponse after like/unlike
type LikeActionResponse struct {
	HasLiked  bool `json:"hasLiked"`
	LikeCount int  `json:"likeCount"`
}

// LikeStatusResponse for GET /anchors/:id/like/status
type LikeStatusResponse struct {
	HasLiked  bool `json:"hasLiked"`
	LikeCount int  `json:"likeCount"`
}

// LikerUserResponse for items in likers list
type LikerUserResponse struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	ProfilePicture *string            `json:"profilePicture"`
	IsFollowing    bool               `json:"isFollowing"`
	LikedAt        time.Time          `json:"likedAt"`
}

// PaginatedLikersResponse for GET /anchors/:id/likes
type PaginatedLikersResponse struct {
	Data       []LikerUserResponse `json:"data"`
	Pagination struct {
		Page       int   `json:"page"`
		Limit      int   `json:"limit"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"totalPages"`
		HasMore    bool  `json:"hasMore"`
	} `json:"pagination"`
}

// LikeSummaryUser for users in like summary
type LikeSummaryUser struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	ProfilePicture *string            `json:"profilePicture"`
}

// LikeSummaryResponse for like summary in GetAnchor
type LikeSummaryResponse struct {
	TotalCount       int               `json:"totalCount"`
	HasLiked         bool              `json:"hasLiked"`
	LikedByFollowing []LikeSummaryUser `json:"likedByFollowing"`
	OtherLikersCount int               `json:"otherLikersCount"`
}
