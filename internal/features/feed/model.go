package feed

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	CategoryTrending = "trending"
	CategoryPopular  = "popular"
	CategoryRecent   = "recent"
)

// FeedQuery represents the query parameters for filtering the feed
type FeedQuery struct {
	Limit      int    `form:"limit"`
	Cursor     string `form:"cursor"`
	IncludeOwn *bool  `form:"includeOwn"` // Pointer to distinguish between false and missing
}

// FeedCursor represents the decoded cursor data
type FeedCursor struct {
	Timestamp time.Time          `json:"t"`
	AnchorID  primitive.ObjectID `json:"i"`
}

// FeedItemAuthor represents the anchor author details
type FeedItemAuthor struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	ProfilePicture *string            `json:"profilePicture"`
	IsVerified     bool               `json:"isVerified"`
}

// FeedLikeSummaryUser represents a user in the like summary
type FeedLikeSummaryUser struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	ProfilePicture *string            `json:"profilePicture"`
}

// FeedLikeSummary represents the summary of likes for an anchor
type FeedLikeSummary struct {
	TotalCount       int                   `json:"totalCount"`
	LikedByFollowing []FeedLikeSummaryUser `json:"likedByFollowing"`
	OtherLikersCount int                   `json:"otherLikersCount"`
}

// FeedEngagement represents the current user's engagement with an anchor
type FeedEngagement struct {
	HasLiked    bool            `json:"hasLiked"`
	HasCloned   bool            `json:"hasCloned"`
	LikeSummary FeedLikeSummary `json:"likeSummary"`
}

// FeedPreviewItem represents a single item preview in the feed
type FeedPreviewItem struct {
	Type      string  `json:"type"`
	Thumbnail *string `json:"thumbnail,omitempty"`
	Title     *string `json:"title,omitempty"`
	Snippet   *string `json:"snippet,omitempty"`
}

// FeedPreview represents the preview section of an anchor
type FeedPreview struct {
	Items []FeedPreviewItem `json:"items"`
}

// FeedItem represents a single anchor item in the feed with enrichments
type FeedItem struct {
	ID              primitive.ObjectID `json:"id"`
	Title           string             `json:"title"`
	Description     string             `json:"description"`
	CoverMediaType  string             `json:"coverMediaType"`
	CoverMediaValue string             `json:"coverMediaValue"`
	Visibility      string             `json:"visibility"`
	IsPinned        bool               `json:"isPinned"`
	Tags            []string           `json:"tags"`
	ItemCount       int                `json:"itemCount"`
	LikeCount       int                `json:"likeCount"`
	CloneCount      int                `json:"cloneCount"`
	CommentCount    int                `json:"commentCount"`
	LastItemAddedAt time.Time          `json:"lastItemAddedAt"`
	CreatedAt       time.Time          `json:"createdAt"`
	Author          FeedItemAuthor     `json:"author"`
	Engagement      FeedEngagement     `json:"engagement"`
	Preview         FeedPreview        `json:"preview"`
}

// FeedPagination represents the pagination metadata
type FeedPagination struct {
	Limit      int     `json:"limit"`
	HasMore    bool    `json:"hasMore"`
	NextCursor *string `json:"nextCursor"`
	ItemCount  int     `json:"itemCount"`
}

// FeedMeta represents the feed metadata
type FeedMeta struct {
	FeedType           string  `json:"feedType"`
	IncludesOwnAnchors bool    `json:"includesOwnAnchors"`
	TotalFollowing     int     `json:"totalFollowing"`
	EmptyReason        *string `json:"emptyReason"`
}

// FeedResponse represents the complete API response for the feed
type FeedResponse struct {
	Items      []FeedItem     `json:"items"`
	Pagination FeedPagination `json:"pagination"`
	Meta       FeedMeta       `json:"meta"`
}

// DiscoverQuery for GET /feed/discover
type DiscoverQuery struct {
	Limit    int    `form:"limit"`
	Cursor   string `form:"cursor"`
	Category string `form:"category"`
	Tag      string `form:"tag"`
}

// DiscoverCursor for pagination (includes score for trending/popular)
type DiscoverCursor struct {
	Score     *int               `json:"s,omitempty"`
	CreatedAt time.Time          `json:"c"`
	AnchorID  primitive.ObjectID `json:"i"`
}

// DiscoverItemAuthor includes followerCount for discovery context
type DiscoverItemAuthor struct {
	ID             primitive.ObjectID `json:"id"`
	Username       string             `json:"username"`
	DisplayName    string             `json:"displayName"`
	ProfilePicture *string            `json:"profilePicture"`
	IsVerified     bool               `json:"isVerified"`
	FollowerCount  int                `json:"followerCount"`
}

// DiscoverItem is a feed item with engagement score
type DiscoverItem struct {
	ID              primitive.ObjectID `json:"id"`
	Title           string             `json:"title"`
	Description     string             `json:"description"`
	CoverMediaType  string             `json:"coverMediaType"`
	CoverMediaValue string             `json:"coverMediaValue"`
	Visibility      string             `json:"visibility"`
	IsPinned        bool               `json:"isPinned"`
	Tags            []string           `json:"tags"`
	ItemCount       int                `json:"itemCount"`
	LikeCount       int                `json:"likeCount"`
	CloneCount      int                `json:"cloneCount"`
	CommentCount    int                `json:"commentCount"`
	EngagementScore int                `json:"engagementScore"`
	LastItemAddedAt time.Time          `json:"lastItemAddedAt"`
	CreatedAt       time.Time          `json:"createdAt"`
	Author          DiscoverItemAuthor `json:"author"`
	Engagement      FeedEngagement     `json:"engagement"`
	Preview         FeedPreview        `json:"preview"`
}

// DiscoverMeta for discovery feed metadata
type DiscoverMeta struct {
	FeedType        string  `json:"feedType"`
	Category        string  `json:"category"`
	Tag             *string `json:"tag"`
	IsAuthenticated bool    `json:"isAuthenticated"`
	EmptyReason     *string `json:"emptyReason"`
}

// DiscoverResponse for the complete discovery feed response
type DiscoverResponse struct {
	Items      []DiscoverItem `json:"items"`
	Pagination FeedPagination `json:"pagination"`
	Meta       DiscoverMeta   `json:"meta"`
}

// FollowingAnchorFeedItem represents an anchor in the following section with update status
type FollowingAnchorFeedItem struct {
	ID              primitive.ObjectID `json:"id"`
	Title           string             `json:"title"`
	CoverMediaType  string             `json:"coverMediaType"`
	CoverMediaValue string             `json:"coverMediaValue"`
	HasUpdate       bool               `json:"hasUpdate"`
	LastSeenVersion int                `json:"lastSeenVersion"`
	CurrentVersion  int                `json:"currentVersion"`
}

// FollowingAnchorsSection represents the section of followed anchors with updates
type FollowingAnchorsSection struct {
	Items      []FollowingAnchorFeedItem `json:"items"`
	TotalCount int                       `json:"totalCount"`
}

// SuggestedCategory represents a tag/category recommendation
type SuggestedCategory struct {
	Name        string `json:"name"`
	AnchorCount int    `json:"anchorCount"`
}

// HomeFeedResponse is the updated response for the main feed
type HomeFeedResponse struct {
	FollowingAnchors    FollowingAnchorsSection `json:"followingAnchors"`
	SuggestedCategories []SuggestedCategory     `json:"suggestedCategories"`
	Feed                FeedResponse            `json:"feed"`
}
