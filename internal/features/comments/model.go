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
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	AnchorID primitive.ObjectID `bson:"anchorId" json:"anchorId"`
	UserID   primitive.ObjectID `bson:"userId" json:"userId"`
	// ParentID is set on replies and always points at a top-level comment (one level of threading)
	ParentID   *primitive.ObjectID  `bson:"parentId,omitempty" json:"parentId,omitempty"`
	ReplyCount int                  `bson:"replyCount" json:"replyCount"`
	Content    string               `bson:"content" json:"content"`
	Mentions   []primitive.ObjectID `bson:"mentions" json:"mentions"`
	LikeCount  int                  `bson:"likeCount" json:"likeCount"`
	IsEdited   bool                 `bson:"isEdited" json:"isEdited"`
	CreatedAt  time.Time            `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time            `bson:"updatedAt" json:"updatedAt"`
	DeletedAt  *time.Time           `bson:"deletedAt,omitempty" json:"-"`
}

// CommentLike represents a like on a comment
type CommentLike struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	CommentID primitive.ObjectID `bson:"commentId" json:"commentId"`
	UserID    primitive.ObjectID `bson:"userId" json:"userId"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

// Request DTOs

// Content is checked (in characters, after trimming) by ValidateCreateCommentRequest
type CreateCommentRequest struct {
	Content string `json:"content"`
}

type UpdateCommentRequest struct {
	Content string `json:"content"`
}

type CommentLikeActionRequest struct {
	Action string `json:"action"` // "like" or "unlike", any letter case
}

type CommentListQuery struct {
	Page  int
	Limit int
	Sort  string
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
	ParentID   *primitive.ObjectID  `json:"parentId,omitempty"`
	ReplyCount int                  `json:"replyCount"`
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

type PaginatedRepliesResponse struct {
	ParentID   primitive.ObjectID `json:"parentId"`
	Replies    []CommentResponse  `json:"replies"`
	Pagination struct {
		Page       int   `json:"page"`
		Limit      int   `json:"limit"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"totalPages"`
		HasMore    bool  `json:"hasMore"`
	} `json:"pagination"`
}
