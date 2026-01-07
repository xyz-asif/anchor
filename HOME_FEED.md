# Home Feed Specification

## Overview

The Home Feed is the primary content discovery surface in the Anchor app. It displays anchors from users the current user follows, sorted by activity (most recently updated first). The feed uses cursor-based pagination for efficient infinite scrolling and includes rich engagement data for each anchor.

---

## API Summary

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 1 | `GET` | `/feed/following` | Required | Get personalized home feed |

**Total: 1 endpoint** (extensible for future feed types)

---

## Core Concepts

### Content Sources
- Anchors from users the current user follows
- Current user's own public/unlisted anchors (included by default)
- Only `public` and `unlisted` visibility (never `private`)
- Only non-deleted anchors (`deletedAt` is null)

### Sorting
- Primary: `lastItemAddedAt` descending (most recently active first)
- Secondary: `_id` descending (tiebreaker for same timestamp)

### Pagination
- **Cursor-based** pagination for infinite scroll
- Cursor encodes timestamp + anchor ID for precise positioning
- Handles real-time updates without duplicates or gaps

---

## API Endpoint

### Get Home Feed

**Endpoint:** `GET /feed/following`

**Authentication:** Required

**Description:** Returns a personalized feed of anchors from followed users and the current user's own anchors, sorted by most recent activity.

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| limit | int | No | 20 | Items per page (min 1, max 50) |
| cursor | string | No | - | Pagination cursor from previous response |
| includeOwn | bool | No | true | Include current user's own anchors |

**Example Requests:**
```
GET /feed/following
GET /feed/following?limit=20
GET /feed/following?limit=20&cursor=eyJ0IjoiMjAyNC0wMS0xNVQxMDozMDowMFoiLCJpIjoiNTA3ZjFmNzdiY2Y4NmNkNzk5NDM5MDExIn0=
GET /feed/following?limit=20&includeOwn=false
```

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "items": [
            {
                "id": "507f1f77bcf86cd799439011",
                "title": "Tech Resources 2024",
                "description": "My curated collection of tech links",
                "coverMediaType": "emoji",
                "coverMediaValue": "💻",
                "visibility": "public",
                "isPinned": false,
                "tags": ["tech", "programming"],
                "itemCount": 25,
                "likeCount": 55,
                "cloneCount": 10,
                "commentCount": 5,
                "lastItemAddedAt": "2024-01-15T10:30:00Z",
                "createdAt": "2024-01-10T00:00:00Z",
                
                "author": {
                    "id": "507f1f77bcf86cd799439012",
                    "username": "@johndoe",
                    "displayName": "John Doe",
                    "profilePicture": "https://cloudinary.com/...",
                    "isVerified": false
                },
                
                "engagement": {
                    "hasLiked": true,
                    "hasCloned": false,
                    "likeSummary": {
                        "totalCount": 55,
                        "likedByFollowing": [
                            {
                                "id": "507f1f77bcf86cd799439020",
                                "username": "@alice",
                                "displayName": "Alice Smith",
                                "profilePicture": "https://..."
                            },
                            {
                                "id": "507f1f77bcf86cd799439021",
                                "username": "@bob",
                                "displayName": "Bob Jones",
                                "profilePicture": null
                            }
                        ],
                        "otherLikersCount": 53
                    }
                },
                
                "preview": {
                    "items": [
                        {
                            "type": "url",
                            "thumbnail": "https://github.com/favicon.ico",
                            "title": "GitHub - Where the world builds software"
                        },
                        {
                            "type": "image",
                            "thumbnail": "https://cloudinary.com/..."
                        },
                        {
                            "type": "text",
                            "snippet": "This is a collection of my favorite programming resources..."
                        }
                    ]
                }
            }
        ],
        "pagination": {
            "limit": 20,
            "hasMore": true,
            "nextCursor": "eyJ0IjoiMjAyNC0wMS0xNFQwODoyMDowMFoiLCJpIjoiNTA3ZjFmNzdiY2Y4NmNkNzk5NDM5MDEwIn0=",
            "itemCount": 20
        },
        "meta": {
            "feedType": "following",
            "includesOwnAnchors": true,
            "totalFollowing": 150,
            "emptyReason": null
        }
    }
}
```

**Empty Feed Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "items": [],
        "pagination": {
            "limit": 20,
            "hasMore": false,
            "nextCursor": null,
            "itemCount": 0
        },
        "meta": {
            "feedType": "following",
            "includesOwnAnchors": true,
            "totalFollowing": 0,
            "emptyReason": "NO_FOLLOWING"
        }
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_CURSOR | Invalid pagination cursor |
| 400 | INVALID_LIMIT | Limit must be between 1 and 50 |
| 401 | UNAUTHORIZED | Authentication required |

---

## Response Field Definitions

### Feed Item (Anchor)

| Field | Type | Description |
|-------|------|-------------|
| id | string | Anchor ID |
| title | string | Anchor title |
| description | string | Anchor description |
| coverMediaType | string | Cover type: icon, emoji, image |
| coverMediaValue | string | Cover value (emoji, icon name, or image URL) |
| visibility | string | public or unlisted |
| isPinned | bool | Is anchor pinned on author's profile |
| tags | array | Array of tags |
| itemCount | int | Number of items in anchor |
| likeCount | int | Total likes |
| cloneCount | int | Total clones |
| commentCount | int | Total comments |
| lastItemAddedAt | datetime | When last item was added |
| createdAt | datetime | When anchor was created |

### Author Object

| Field | Type | Description |
|-------|------|-------------|
| id | string | Author's user ID |
| username | string | Author's username (with @) |
| displayName | string | Author's display name |
| profilePicture | string/null | Profile picture URL |
| isVerified | bool | Is author verified |

### Engagement Object

| Field | Type | Description |
|-------|------|-------------|
| hasLiked | bool | Has current user liked this anchor |
| hasCloned | bool | Has current user cloned this anchor |
| likeSummary | object | Like summary with followed users |

### Like Summary Object

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Total likes (same as likeCount) |
| likedByFollowing | array | Up to 3 users who liked that current user follows |
| otherLikersCount | int | totalCount - likedByFollowing.length |

### Preview Object

| Field | Type | Description |
|-------|------|-------------|
| items | array | Up to 3 preview items from the anchor |

### Preview Item Object

| Field | Type | Present When | Description |
|-------|------|--------------|-------------|
| type | string | Always | Item type: url, image, audio, file, text |
| thumbnail | string | url, image | Thumbnail/favicon URL |
| title | string | url | URL title |
| snippet | string | text | First 100 characters of text |

### Pagination Object

| Field | Type | Description |
|-------|------|-------------|
| limit | int | Requested limit |
| hasMore | bool | Are there more items |
| nextCursor | string/null | Cursor for next page (null if no more) |
| itemCount | int | Number of items in current response |

### Meta Object

| Field | Type | Description |
|-------|------|-------------|
| feedType | string | Always "following" for this endpoint |
| includesOwnAnchors | bool | Whether user's own anchors are included |
| totalFollowing | int | Total number of users current user follows |
| emptyReason | string/null | Reason if feed is empty (see below) |

### Empty Reason Values

| Value | Meaning | Suggested CTA |
|-------|---------|---------------|
| `NO_FOLLOWING` | User follows no one | "Find people to follow" |
| `NO_CONTENT` | Followed users have no public anchors | "Explore popular anchors" |
| `END_OF_FEED` | Reached end of available content | "You're all caught up!" |
| `null` | Feed has content | - |

---

## Cursor Format

### Structure
The cursor is a base64-encoded JSON object:

```json
{
    "t": "2024-01-15T10:30:00Z",
    "i": "507f1f77bcf86cd799439011"
}
```

| Field | Description |
|-------|-------------|
| t | ISO timestamp of last item's `lastItemAddedAt` |
| i | ObjectID of last item |

### Encoding/Decoding

```go
// Encode
cursorData := map[string]string{
    "t": lastAnchor.LastItemAddedAt.Format(time.RFC3339),
    "i": lastAnchor.ID.Hex(),
}
jsonBytes, _ := json.Marshal(cursorData)
cursor := base64.StdEncoding.EncodeToString(jsonBytes)

// Decode
jsonBytes, err := base64.StdEncoding.DecodeString(cursor)
var cursorData map[string]string
json.Unmarshal(jsonBytes, &cursorData)
timestamp, _ := time.Parse(time.RFC3339, cursorData["t"])
anchorID, _ := primitive.ObjectIDFromHex(cursorData["i"])
```

---

## Request/Response DTOs

### Query Parameters DTO

```go
// FeedQuery for GET /feed/following
type FeedQuery struct {
    Limit      int    `form:"limit,default=20" binding:"min=1,max=50"`
    Cursor     string `form:"cursor"`
    IncludeOwn *bool  `form:"includeOwn"`
}
```

### Response DTOs

```go
// FeedItemAuthor represents the anchor author
type FeedItemAuthor struct {
    ID             primitive.ObjectID `json:"id"`
    Username       string             `json:"username"`
    DisplayName    string             `json:"displayName"`
    ProfilePicture *string            `json:"profilePicture"`
    IsVerified     bool               `json:"isVerified"`
}

// FeedLikeSummaryUser represents a user in like summary
type FeedLikeSummaryUser struct {
    ID             primitive.ObjectID `json:"id"`
    Username       string             `json:"username"`
    DisplayName    string             `json:"displayName"`
    ProfilePicture *string            `json:"profilePicture"`
}

// FeedLikeSummary represents like information
type FeedLikeSummary struct {
    TotalCount       int                   `json:"totalCount"`
    LikedByFollowing []FeedLikeSummaryUser `json:"likedByFollowing"`
    OtherLikersCount int                   `json:"otherLikersCount"`
}

// FeedEngagement represents user engagement with anchor
type FeedEngagement struct {
    HasLiked    bool            `json:"hasLiked"`
    HasCloned   bool            `json:"hasCloned"`
    LikeSummary FeedLikeSummary `json:"likeSummary"`
}

// FeedPreviewItem represents a preview item
type FeedPreviewItem struct {
    Type      string  `json:"type"`
    Thumbnail *string `json:"thumbnail,omitempty"`
    Title     *string `json:"title,omitempty"`
    Snippet   *string `json:"snippet,omitempty"`
}

// FeedPreview represents anchor preview
type FeedPreview struct {
    Items []FeedPreviewItem `json:"items"`
}

// FeedItem represents a single feed item (anchor with enrichments)
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

// FeedPagination represents pagination info
type FeedPagination struct {
    Limit      int     `json:"limit"`
    HasMore    bool    `json:"hasMore"`
    NextCursor *string `json:"nextCursor"`
    ItemCount  int     `json:"itemCount"`
}

// FeedMeta represents feed metadata
type FeedMeta struct {
    FeedType           string  `json:"feedType"`
    IncludesOwnAnchors bool    `json:"includesOwnAnchors"`
    TotalFollowing     int     `json:"totalFollowing"`
    EmptyReason        *string `json:"emptyReason"`
}

// FeedResponse represents the complete feed response
type FeedResponse struct {
    Items      []FeedItem     `json:"items"`
    Pagination FeedPagination `json:"pagination"`
    Meta       FeedMeta       `json:"meta"`
}
```

---

## Business Logic

### Feed Generation Algorithm

```
Function GetHomeFeed(currentUserID, limit, cursor, includeOwn):

    1. VALIDATE INPUT
       - If cursor provided, decode and validate
       - Ensure limit is within bounds (1-50)
       - Default includeOwn to true if not specified

    2. GET FOLLOWING LIST
       - Query follows collection: followerID = currentUserID
       - Extract all followingIDs
       - If includeOwn is true, add currentUserID to the list
       - If list is empty AND includeOwn is false:
         Return empty feed with emptyReason = "NO_FOLLOWING"

    3. BUILD ANCHOR QUERY
       filter = {
           userId: { $in: followingUserIds },
           visibility: { $in: ["public", "unlisted"] },
           deletedAt: null
       }
       
       If cursor provided:
           cursorTime, cursorId = decodeCursor(cursor)
           filter["$or"] = [
               { lastItemAddedAt: { $lt: cursorTime } },
               { lastItemAddedAt: cursorTime, _id: { $lt: cursorId } }
           ]

    4. EXECUTE QUERY
       - Sort by lastItemAddedAt DESC, _id DESC
       - Limit to (limit + 1) to check hasMore
       - Execute query

    5. CHECK FOR MORE
       - If results.length > limit:
         hasMore = true
         Remove last item from results
       - Else:
         hasMore = false

    6. HANDLE EMPTY RESULTS
       - If no results AND cursor is null:
         If followingUserIds is empty:
           emptyReason = "NO_FOLLOWING"
         Else:
           emptyReason = "NO_CONTENT"
       - If no results AND cursor is not null:
         emptyReason = "END_OF_FEED"

    7. ENRICH ANCHORS (batch operations for efficiency)
       
       a. Collect all unique author IDs from anchors
       b. Batch fetch author details: authRepo.GetUsersByIDs(authorIds)
       c. Create author map for O(1) lookup
       
       d. Collect all anchor IDs
       e. Batch check which anchors current user has liked:
          likesRepo.GetUserLikedAnchors(currentUserID, anchorIds)
       
       f. Batch check which anchors current user has cloned:
          anchorsRepo.GetUserClonedAnchors(currentUserID, anchorIds)
       
       g. For each anchor, get like summary:
          - Get recent likers (limit 20)
          - Cross-reference with current user's following list
          - Prioritize followed users (max 3)
       
       h. For each anchor, get preview items:
          - Get first 3 items from anchor
          - Extract preview data based on type

    8. BUILD CURSOR FOR NEXT PAGE
       - If hasMore:
         lastAnchor = results[len(results)-1]
         nextCursor = encodeCursor(lastAnchor.LastItemAddedAt, lastAnchor.ID)
       - Else:
         nextCursor = null

    9. BUILD AND RETURN RESPONSE
       - Map anchors to FeedItem objects with all enrichments
       - Include pagination info
       - Include meta info

    Return FeedResponse
```

### Preview Item Generation

```
Function GetPreviewItems(anchorID, limit=3):
    
    items = itemsRepo.GetAnchorItems(anchorID, page=1, limit=3)
    
    previews = []
    For each item in items:
        preview = { type: item.Type }
        
        Switch item.Type:
            Case "url":
                preview.thumbnail = item.URLData.Favicon OR item.URLData.Thumbnail
                preview.title = item.URLData.Title (truncate to 50 chars)
            
            Case "image":
                preview.thumbnail = item.ImageData.CloudinaryURL
                // Could add transformation for smaller size
            
            Case "text":
                preview.snippet = item.TextData.Content[:100] + "..."
            
            Case "audio":
                preview.thumbnail = null  // Use default audio icon on client
            
            Case "file":
                preview.thumbnail = null  // Use default file icon on client
        
        previews.append(preview)
    
    Return previews
```

---

## Database Indexes

### Required Indexes

```go
// In anchors collection - compound index for feed query
{
    Keys: bson.D{
        {Key: "userId", Value: 1},
        {Key: "visibility", Value: 1},
        {Key: "deletedAt", Value: 1},
        {Key: "lastItemAddedAt", Value: -1},
        {Key: "_id", Value: -1},
    },
}
```

### Existing Indexes Used
- `follows` collection: `{followerId: 1, createdAt: -1}` - for getting following list
- `likes` collection: `{userId: 1, anchorId: 1}` - for batch like check
- `users` collection: `{_id: 1}` - for author lookup

---

## Business Rules

### Content Rules

| # | Rule |
|---|------|
| 1 | Only show anchors with visibility = "public" or "unlisted" |
| 2 | Never show deleted anchors (deletedAt != null) |
| 3 | Include current user's own anchors by default |
| 4 | Sort by lastItemAddedAt descending (most active first) |

### Pagination Rules

| # | Rule |
|---|------|
| 5 | Default limit is 20 |
| 6 | Maximum limit is 50 |
| 7 | Minimum limit is 1 |
| 8 | Cursor is required for pages after the first |
| 9 | Invalid cursor returns error (don't silently reset) |

### Engagement Rules

| # | Rule |
|---|------|
| 10 | hasLiked reflects current user's like status |
| 11 | hasCloned reflects if current user has cloned this anchor |
| 12 | likeSummary prioritizes users that current user follows |
| 13 | Maximum 3 users in likedByFollowing array |

### Preview Rules

| # | Rule |
|---|------|
| 14 | Maximum 3 preview items per anchor |
| 15 | Preview items are the first 3 items by position |
| 16 | Text snippets truncated to 100 characters |
| 17 | URL titles truncated to 50 characters |

---

## Validation Rules

### Cursor Validation
- Must be valid base64
- Must decode to valid JSON
- Must contain "t" (timestamp) and "i" (id) fields
- Timestamp must be valid ISO 8601
- ID must be valid MongoDB ObjectID

### Limit Validation
- Must be integer
- Must be >= 1
- Must be <= 50
- Defaults to 20 if not provided

---

## File Structure

```
internal/features/feed/
├── model.go       # All DTOs (FeedItem, FeedResponse, etc.)
├── repository.go  # Database operations
├── handler.go     # HTTP handler
├── service.go     # Business logic (GetHomeFeed algorithm)
├── cursor.go      # Cursor encoding/decoding utilities
├── validator.go   # Request validation
└── routes.go      # Route registration
```

---

## Repository Methods

```go
type Repository struct {
    anchorsCollection *mongo.Collection
    itemsCollection   *mongo.Collection
}

// NewRepository creates repository (uses existing collections)
func NewRepository(db *mongo.Database) *Repository

// GetFeedAnchors retrieves anchors for feed with cursor pagination
func (r *Repository) GetFeedAnchors(
    ctx context.Context,
    userIDs []primitive.ObjectID,
    cursor *FeedCursor,
    limit int,
) ([]Anchor, error)

// GetPreviewItems retrieves first N items for preview
func (r *Repository) GetPreviewItems(
    ctx context.Context,
    anchorID primitive.ObjectID,
    limit int,
) ([]Item, error)

// GetUserClonedAnchors batch checks if user cloned any of these anchors
func (r *Repository) GetUserClonedAnchors(
    ctx context.Context,
    userID primitive.ObjectID,
    anchorIDs []primitive.ObjectID,
) (map[primitive.ObjectID]bool, error)
```

---

## Service Methods

```go
type Service struct {
    feedRepo    *Repository
    authRepo    *auth.Repository
    followsRepo *follows.Repository
    likesRepo   *likes.Repository
    anchorsRepo *anchors.Repository
}

// NewService creates the feed service with all dependencies
func NewService(
    feedRepo *Repository,
    authRepo *auth.Repository,
    followsRepo *follows.Repository,
    likesRepo *likes.Repository,
    anchorsRepo *anchors.Repository,
) *Service

// GetHomeFeed returns personalized feed for user
func (s *Service) GetHomeFeed(
    ctx context.Context,
    userID primitive.ObjectID,
    query *FeedQuery,
) (*FeedResponse, error)

// Helper methods
func (s *Service) enrichAnchorsWithAuthors(ctx, anchors, authorIDs) error
func (s *Service) enrichAnchorsWithEngagement(ctx, anchors, userID) error
func (s *Service) enrichAnchorsWithPreviews(ctx, anchors) error
func (s *Service) getLikeSummaryForAnchor(ctx, anchorID, userID, followingIDs) *FeedLikeSummary
```

---

## Handler

```go
type Handler struct {
    service *Service
    config  *config.Config
}

// NewHandler creates feed handler
func NewHandler(service *Service, cfg *config.Config) *Handler

// GetFollowingFeed godoc
// @Summary Get home feed
// @Description Get personalized feed of anchors from followed users
// @Tags feed
// @Produce json
// @Security BearerAuth
// @Param limit query int false "Items per page (default 20, max 50)"
// @Param cursor query string false "Pagination cursor"
// @Param includeOwn query bool false "Include own anchors (default true)"
// @Success 200 {object} response.APIResponse{data=FeedResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /feed/following [get]
func (h *Handler) GetFollowingFeed(c *gin.Context)
```

---

## Route Registration

```go
// internal/features/feed/routes.go

package feed

import (
    "github.com/gin-gonic/gin"
    "github.com/xyz-asif/gotodo/internal/config"
    "github.com/xyz-asif/gotodo/internal/features/anchors"
    "github.com/xyz-asif/gotodo/internal/features/auth"
    "github.com/xyz-asif/gotodo/internal/features/follows"
    "github.com/xyz-asif/gotodo/internal/features/likes"
    "github.com/xyz-asif/gotodo/internal/middleware"
    "go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
    // Initialize repositories
    feedRepo := NewRepository(db)
    authRepo := auth.NewRepository(db)
    followsRepo := follows.NewRepository(db)
    likesRepo := likes.NewRepository(db)
    anchorsRepo := anchors.NewRepository(db)

    // Initialize service
    service := NewService(feedRepo, authRepo, followsRepo, likesRepo, anchorsRepo)

    // Initialize handler
    handler := NewHandler(service, cfg)

    // Initialize middleware
    authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)

    // Feed routes
    feed := router.Group("/feed")
    feed.Use(authMiddleware)
    {
        feed.GET("/following", handler.GetFollowingFeed)
        // Future: feed.GET("/discover", handler.GetDiscoverFeed)
        // Future: feed.GET("/trending", handler.GetTrendingFeed)
    }
}
```

---

## Implementation Order

### Step 1: Create Model (model.go)
- All DTOs for feed (FeedItem, FeedResponse, etc.)
- FeedCursor struct
- FeedQuery struct

### Step 2: Create Cursor Utilities (cursor.go)
- EncodeCursor function
- DecodeCursor function
- FeedCursor struct

### Step 3: Create Repository (repository.go)
- NewRepository
- GetFeedAnchors (with cursor support)
- GetPreviewItems
- GetUserClonedAnchors
- Ensure index creation

### Step 4: Update Follows Repository
- Add GetFollowingIDs method (get all IDs user follows)

### Step 5: Create Service (service.go)
- NewService with all dependencies
- GetHomeFeed main method
- All enrichment helper methods

### Step 6: Create Validator (validator.go)
- ValidateFeedQuery
- ValidateCursor

### Step 7: Create Handler (handler.go)
- GetFollowingFeed handler with Swagger

### Step 8: Create Routes (routes.go)
- RegisterRoutes function

### Step 9: Update Main Routes
- Register feed routes in routes/routes.go

---

## Dependencies on Existing Modules

| Module | Methods Used |
|--------|--------------|
| **auth** | GetUsersByIDs |
| **follows** | GetFollowingIDs (new), GetFollowingIDs (existing batch check) |
| **likes** | GetUserLikedAnchors, GetRecentLikers |
| **anchors** | Repository for anchor queries |

### New Method Needed in Follows Repository

```go
// GetAllFollowingIDs returns all user IDs that the given user follows
func (r *Repository) GetAllFollowingIDs(ctx context.Context, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
    filter := bson.M{"followerId": userID}
    
    cursor, err := r.collection.Find(ctx, filter, options.Find().SetProjection(bson.M{"followingId": 1}))
    if err != nil {
        return nil, err
    }
    defer cursor.Close(ctx)
    
    var follows []Follow
    if err = cursor.All(ctx, &follows); err != nil {
        return nil, err
    }
    
    ids := make([]primitive.ObjectID, len(follows))
    for i, f := range follows {
        ids[i] = f.FollowingID
    }
    
    return ids, nil
}
```

---

## Testing Scenarios

### Basic Feed
- [ ] Get feed when following users with content
- [ ] Get feed with default limit (20)
- [ ] Get feed with custom limit
- [ ] Get feed with cursor (pagination)
- [ ] Get feed excluding own anchors (includeOwn=false)

### Empty Feed States
- [ ] User follows no one (emptyReason: NO_FOLLOWING)
- [ ] User follows people but no public content (emptyReason: NO_CONTENT)
- [ ] End of feed reached (emptyReason: END_OF_FEED)

### Pagination
- [ ] First page returns nextCursor when hasMore
- [ ] Using cursor returns correct next set
- [ ] Last page has hasMore=false and nextCursor=null
- [ ] Invalid cursor returns error
- [ ] Cursor with non-existent anchor still works (returns next available)

### Content Filtering
- [ ] Private anchors not shown
- [ ] Deleted anchors not shown
- [ ] Unlisted anchors shown
- [ ] Own anchors included by default
- [ ] Own anchors excluded when includeOwn=false

### Engagement Data
- [ ] hasLiked is correct for each anchor
- [ ] hasCloned is correct for each anchor
- [ ] likeSummary shows followed users first
- [ ] likeSummary limited to 3 users
- [ ] otherLikersCount calculated correctly

### Author Data
- [ ] Author info included for each anchor
- [ ] Correct author linked to each anchor
- [ ] Author profilePicture can be null

### Preview Data
- [ ] Preview includes first 3 items
- [ ] URL preview has thumbnail and title
- [ ] Image preview has thumbnail
- [ ] Text preview has snippet (truncated)
- [ ] Anchor with no items has empty preview

### Sorting
- [ ] Anchors sorted by lastItemAddedAt DESC
- [ ] Same timestamp sorted by _id DESC

### Performance
- [ ] Feed loads in reasonable time with 100+ following
- [ ] Batch queries used (not N+1)
- [ ] Response size reasonable (<100KB typical)

### Authentication
- [ ] Unauthenticated request returns 401
- [ ] Invalid token returns 401

---

## Performance Considerations

### Query Optimization
1. **Compound Index** - Essential for feed query performance
2. **Batch Operations** - All enrichments done in batches, not per-anchor
3. **Projection** - Only fetch needed fields
4. **Limit + 1** - Check hasMore without extra count query

### Caching Strategy (Future)
1. **Following List** - Cache for 5 minutes (changes infrequently)
2. **Feed Results** - Cache for 30 seconds (balance freshness vs. performance)
3. **Invalidation** - On new follow, new anchor from followed user

### Scaling Considerations
For >1000 following:
1. Consider fanout-on-write (pre-computed feeds)
2. Or limit to most active N followed users
3. Or time-bounded query (last 30 days)

---

## Future Enhancements

### Additional Feed Types
- `/feed/discover` - Popular anchors from non-followed users
- `/feed/trending` - Trending anchors (by engagement velocity)
- `/feed/tags/:tag` - Anchors by tag

### Algorithmic Ranking (Future)
Score = recency_score + engagement_score + author_relationship_score

### Real-time Updates (Future)
- WebSocket for new feed items
- "New posts" indicator

### Feed Preferences (Future)
- Hide/mute specific users
- Content preferences by tags