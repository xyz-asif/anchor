package comments

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Sort constants
const (
	SortNewest = "newest"
	SortOldest = "oldest"
	SortTop    = "top"
)

// Comment represents a comment on an anchor
type Comment struct {
	ID        primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	AnchorID  primitive.ObjectID   `bson:"anchorId" json:"anchorId"`
	UserID    primitive.ObjectID   `bson:"userId" json:"userId"`
	Content   string               `bson:"content" json:"content"`
	Mentions  []primitive.ObjectID `bson:"mentions" json:"mentions"`
	LikeCount int                  `bson:"likeCount" json:"likeCount"`
	IsEdited  bool                 `bson:"isEdited" json:"isEdited"`
	CreatedAt time.Time            `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time            `bson:"updatedAt" json:"updatedAt"`
	DeletedAt *time.Time           `bson:"deletedAt,omitempty" json:"-"`
}

// CommentLike represents a like on a comment
type CommentLike struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	CommentID primitive.ObjectID `bson:"commentId" json:"commentId"`
	UserID    primitive.ObjectID `bson:"userId" json:"userId"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

// Request DTOs

type CreateCommentRequest struct {
	Content string `json:"content" binding:"required,min=1,max=1000"`
}

type UpdateCommentRequest struct {
	Content string `json:"content" binding:"required,min=1,max=1000"`
}

type CommentLikeActionRequest struct {
	Action string `json:"action" binding:"required,oneof=like unlike"`
}

type CommentListQuery struct {
	Page  int    `form:"page,default=1" binding:"min=1"`
	Limit int    `form:"limit,default=20" binding:"min=1,max=50"`
	Sort  string `form:"sort,default=newest"`
}

// Response DTOs

type CommentAuthor struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	ProfilePicture *string            `json:"profilePicture"`
	IsVerified     bool               `json:"isVerified"`
}

type CommentEngagement struct {
	HasLiked bool `json:"hasLiked"`
}

type CommentResponse struct {
	ID         primitive.ObjectID   `json:"id"`
	AnchorID   primitive.ObjectID   `json:"anchorId"`
	Content    string               `json:"content"`
	Mentions   []primitive.ObjectID `json:"mentions"`
	LikeCount  int                  `json:"likeCount"`
	IsEdited   bool                 `json:"isEdited"`
	CreatedAt  time.Time            `json:"createdAt"`
	UpdatedAt  time.Time            `json:"updatedAt"`
	Author     CommentAuthor        `json:"author"`
	Engagement CommentEngagement    `json:"engagement"`
}

type CommentLikeResponse struct {
	HasLiked  bool `json:"hasLiked"`
	LikeCount int  `json:"likeCount"`
}

type CommentListMeta struct {
	Sort     string             `json:"sort"`
	AnchorID primitive.ObjectID `json:"anchorId"`
}

type PaginatedCommentsResponse struct {
	Comments   []CommentResponse `json:"comments"`
	Pagination struct {
		Page       int   `json:"page"`
		Limit      int   `json:"limit"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"totalPages"`
		HasMore    bool  `json:"hasMore"`
	} `json:"pagination"`
	Meta CommentListMeta `json:"meta"`
}
