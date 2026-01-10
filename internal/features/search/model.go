package search

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Sort constants
const (
	SortRelevant = "relevant"
	SortRecent   = "recent"
	SortPopular  = "popular"
)

// Search type constants
const (
	TypeAll     = "all"
	TypeAnchors = "anchors"
	TypeUsers   = "users"
)

// Request DTOs

// UnifiedSearchQuery for GET /search
type UnifiedSearchQuery struct {
	Q     string `form:"q" binding:"required,min=2,max=100"`
	Type  string `form:"type,default=all"`
	Limit int    `form:"limit,default=10" binding:"min=1,max=20"`
}

// AnchorSearchQuery for GET /search/anchors
type AnchorSearchQuery struct {
	Q     string `form:"q" binding:"required,min=2,max=100"`
	Tag   string `form:"tag"`
	Sort  string `form:"sort,default=relevant"`
	Page  int    `form:"page,default=1" binding:"min=1"`
	Limit int    `form:"limit,default=20" binding:"min=1,max=50"`
}

// UserSearchQuery for GET /search/users
type UserSearchQuery struct {
	Q     string `form:"q" binding:"required,min=2,max=100"`
	Page  int    `form:"page,default=1" binding:"min=1"`
	Limit int    `form:"limit,default=20" binding:"min=1,max=50"`
}

// TagSearchQuery for GET /search/tags
type TagSearchQuery struct {
	Q     string `form:"q" binding:"required,min=1,max=50"`
	Limit int    `form:"limit,default=10" binding:"min=1,max=20"`
}

// Response DTOs

// SearchAnchorAuthor for anchor search results
type SearchAnchorAuthor struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	ProfilePicture *string            `json:"profilePicture"`
	IsVerified     bool               `json:"isVerified"`
}

// SearchAnchorResult for anchor search results
type SearchAnchorResult struct {
	ID              primitive.ObjectID `json:"id"`
	Title           string             `json:"title"`
	Description     string             `json:"description"`
	Visibility      string             `json:"visibility"`
	ItemCount       int                `json:"itemCount"`
	LikeCount       int                `json:"likeCount"`
	CommentCount    int                `json:"commentCount"`
	CloneCount      int                `json:"cloneCount"`
	EngagementScore int                `json:"engagementScore"`
	Tags            []string           `json:"tags"`
	CreatedAt       time.Time          `json:"createdAt"`
	Author          SearchAnchorAuthor `json:"author"`
}

// SearchUserResult for user search results
type SearchUserResult struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	Bio            string             `json:"bio"`
	ProfilePicture *string            `json:"profilePicture"`
	FollowerCount  int                `json:"followerCount"`
	AnchorCount    int                `json:"anchorCount"`
	IsVerified     bool               `json:"isVerified"`
	IsFollowing    bool               `json:"isFollowing"`
}

// TagResult for tag autocomplete
type TagResult struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// UnifiedSearchAnchorsResult for unified search
type UnifiedSearchAnchorsResult struct {
	Items   []SearchAnchorResult `json:"items"`
	Total   int64                `json:"total"`
	HasMore bool                 `json:"hasMore"`
}

// UnifiedSearchUsersResult for unified search
type UnifiedSearchUsersResult struct {
	Items   []SearchUserResult `json:"items"`
	Total   int64              `json:"total"`
	HasMore bool               `json:"hasMore"`
}

// UnifiedSearchResponse for GET /search
type UnifiedSearchResponse struct {
	Query   string                      `json:"query"`
	Anchors *UnifiedSearchAnchorsResult `json:"anchors,omitempty"`
	Users   *UnifiedSearchUsersResult   `json:"users,omitempty"`
	Tags    []TagResult                 `json:"tags,omitempty"`
}

// AnchorSearchMeta for anchor search metadata
type AnchorSearchMeta struct {
	Query string  `json:"query"`
	Tag   *string `json:"tag"`
	Sort  string  `json:"sort"`
}

// PaginatedAnchorSearchResponse for GET /search/anchors
type PaginatedAnchorSearchResponse struct {
	Anchors    []SearchAnchorResult `json:"anchors"`
	Pagination struct {
		Page       int   `json:"page"`
		Limit      int   `json:"limit"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"totalPages"`
		HasMore    bool  `json:"hasMore"`
	} `json:"pagination"`
	Meta AnchorSearchMeta `json:"meta"`
}

// UserSearchMeta for user search metadata
type UserSearchMeta struct {
	Query string `json:"query"`
}

// PaginatedUserSearchResponse for GET /search/users
type PaginatedUserSearchResponse struct {
	Users      []SearchUserResult `json:"users"`
	Pagination struct {
		Page       int   `json:"page"`
		Limit      int   `json:"limit"`
		Total      int64 `json:"total"`
		TotalPages int   `json:"totalPages"`
		HasMore    bool  `json:"hasMore"`
	} `json:"pagination"`
	Meta UserSearchMeta `json:"meta"`
}

// TagSearchResponse for GET /search/tags
type TagSearchResponse struct {
	Tags  []TagResult `json:"tags"`
	Query string      `json:"query"`
}

// Internal structs for database operations

// AnchorSearchDoc represents anchor document from search
type AnchorSearchDoc struct {
	ID              primitive.ObjectID `bson:"_id"`
	UserID          primitive.ObjectID `bson:"userId"`
	Title           string             `bson:"title"`
	Description     string             `bson:"description"`
	Visibility      string             `bson:"visibility"`
	ItemCount       int                `bson:"itemCount"`
	LikeCount       int                `bson:"likeCount"`
	CommentCount    int                `bson:"commentCount"`
	CloneCount      int                `bson:"cloneCount"`
	EngagementScore int                `bson:"engagementScore"`
	Tags            []string           `bson:"tags"`
	CreatedAt       time.Time          `bson:"createdAt"`
	Score           float64            `bson:"score,omitempty"`
}

// UserSearchDoc represents user document from search
type UserSearchDoc struct {
	ID                primitive.ObjectID `bson:"_id"`
	Username          string             `bson:"username"`
	DisplayName       string             `bson:"displayName"`
	Bio               string             `bson:"bio"`
	ProfilePictureURL string             `bson:"profilePictureUrl"`
	FollowerCount     int                `bson:"followerCount"`
	AnchorCount       int                `bson:"anchorCount"`
	IsVerified        bool               `bson:"isVerified"`
	Score             float64            `bson:"score,omitempty"`
}

// TagAggResult for tag aggregation
type TagAggResult struct {
	Name  string `bson:"name"`
	Count int    `bson:"count"`
}
