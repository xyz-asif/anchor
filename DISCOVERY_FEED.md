# Discovery Feed Specification

## Overview

The Discovery Feed helps users find new content from people they don't follow. It surfaces popular, trending, and recent public anchors, enabling users to discover new creators and curated collections. Unlike the Home Feed, the Discovery Feed is accessible without authentication (with reduced personalization).

---

## API Summary

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 1 | `GET` | `/feed/discover` | Optional | Get discovery feed |

**Total: 1 endpoint**

---

## Core Concepts

### Content Sources
- Public anchors from users the current user does **NOT** follow
- Excludes current user's own anchors (if authenticated)
- Only `public` visibility (NOT `unlisted` - those are "hidden from discovery")
- Only non-deleted anchors

### Categories

| Category | Description | Sorting | Time Window |
|----------|-------------|---------|-------------|
| `trending` | Rising content with recent engagement | engagementScore DESC | Last 48 hours |
| `popular` | All-time highest engagement | engagementScore DESC | None |
| `recent` | Newest public anchors | createdAt DESC | None |

### Engagement Score Formula
```
engagementScore = (likeCount × 2) + (cloneCount × 3) + (commentCount × 1)
```

- **Clones weighted highest** - indicates high curation value
- **Likes weighted medium** - shows appreciation
- **Comments weighted lowest** - engagement but less signal of quality

### Pagination
- Cursor-based pagination (same pattern as Home Feed)
- Cursor includes score + createdAt + ID for stable pagination

---

## API Endpoint

### Get Discovery Feed

**Endpoint:** `GET /feed/discover`

**Authentication:** Optional (personalization requires auth)

**Description:** Returns a discovery feed of public anchors from users the current user doesn't follow. Without authentication, returns general trending/popular content.

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| limit | int | No | 20 | Items per page (min 1, max 50) |
| cursor | string | No | - | Pagination cursor |
| category | string | No | trending | Filter: trending, popular, recent |
| tag | string | No | - | Filter by tag (case-insensitive) |

**Example Requests:**
```
GET /feed/discover
GET /feed/discover?category=trending
GET /feed/discover?category=popular&limit=20
GET /feed/discover?category=recent&tag=tech
GET /feed/discover?tag=programming
GET /feed/discover?cursor=eyJzIjoxNTAsImMiOiIyMDI0LTAxLTE1VDEwOjMwOjAwWiIsImkiOiI1MDdmMWY3N2JjZjg2Y2Q3OTk0MzkwMTEifQ==
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
                "title": "Ultimate Design Resources 2024",
                "description": "500+ curated design tools and inspiration",
                "coverMediaType": "emoji",
                "coverMediaValue": "🎨",
                "visibility": "public",
                "isPinned": true,
                "tags": ["design", "resources", "ui"],
                "itemCount": 85,
                "likeCount": 342,
                "cloneCount": 156,
                "commentCount": 28,
                "engagementScore": 1180,
                "lastItemAddedAt": "2024-01-15T10:30:00Z",
                "createdAt": "2024-01-01T00:00:00Z",
                
                "author": {
                    "id": "507f1f77bcf86cd799439012",
                    "username": "@designguru",
                    "displayName": "Sarah Chen",
                    "profilePicture": "https://cloudinary.com/...",
                    "isVerified": true,
                    "followerCount": 5420
                },
                
                "engagement": {
                    "hasLiked": false,
                    "hasCloned": false,
                    "likeSummary": {
                        "totalCount": 342,
                        "likedByFollowing": [],
                        "otherLikersCount": 342
                    }
                },
                
                "preview": {
                    "items": [
                        {
                            "type": "url",
                            "thumbnail": "https://figma.com/favicon.ico",
                            "title": "Figma - Design Tool"
                        },
                        {
                            "type": "image",
                            "thumbnail": "https://cloudinary.com/..."
                        },
                        {
                            "type": "url",
                            "thumbnail": "https://dribbble.com/favicon.ico",
                            "title": "Dribbble - Design Inspiration"
                        }
                    ]
                }
            }
        ],
        "pagination": {
            "limit": 20,
            "hasMore": true,
            "nextCursor": "eyJzIjoxMTgwLCJjIjoiMjAyNC0wMS0wMVQwMDowMDowMFoiLCJpIjoiNTA3ZjFmNzdiY2Y4NmNkNzk5NDM5MDExIn0=",
            "itemCount": 20
        },
        "meta": {
            "feedType": "discover",
            "category": "trending",
            "tag": null,
            "isAuthenticated": true,
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
            "feedType": "discover",
            "category": "trending",
            "tag": "blockchain",
            "isAuthenticated": true,
            "emptyReason": "NO_TAG_CONTENT"
        }
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_CURSOR | Invalid pagination cursor |
| 400 | INVALID_LIMIT | Limit must be between 1 and 50 |
| 400 | INVALID_CATEGORY | Category must be: trending, popular, or recent |

---

## Response Field Definitions

### Discovery Feed Item

Extends the Home Feed FeedItem with additional fields:

| Field | Type | Description |
|-------|------|-------------|
| engagementScore | int | Computed engagement score |
| author.followerCount | int | Author's follower count (for discovery context) |

### Discovery Author Object

| Field | Type | Description |
|-------|------|-------------|
| id | string | Author's user ID |
| username | string | Author's username (with @) |
| displayName | string | Author's display name |
| profilePicture | string/null | Profile picture URL |
| isVerified | bool | Is author verified |
| followerCount | int | Number of followers (helps users decide to follow) |

### Discovery Meta Object

| Field | Type | Description |
|-------|------|-------------|
| feedType | string | Always "discover" |
| category | string | Current category: trending, popular, recent |
| tag | string/null | Tag filter if applied |
| isAuthenticated | bool | Whether user is authenticated |
| emptyReason | string/null | Reason if feed is empty |

### Empty Reason Values

| Value | Meaning | Suggested CTA |
|-------|---------|---------------|
| `NO_CONTENT` | No public anchors exist | "Be the first to create!" |
| `NO_TAG_CONTENT` | No anchors with specified tag | "Try a different tag" |
| `FOLLOWING_ALL` | User follows all creators with content | "Check your home feed" |
| `END_OF_FEED` | Reached end of available content | "Check back later!" |
| `null` | Feed has content | - |

---

## Cursor Format

### Structure for Scored Categories (trending, popular)
```json
{
    "s": 1180,
    "c": "2024-01-01T00:00:00Z",
    "i": "507f1f77bcf86cd799439011"
}
```

| Field | Description |
|-------|-------------|
| s | engagementScore of last item |
| c | createdAt of last item (ISO timestamp) |
| i | ObjectID of last item |

### Structure for Recent Category
```json
{
    "c": "2024-01-15T10:30:00Z",
    "i": "507f1f77bcf86cd799439011"
}
```

| Field | Description |
|-------|-------------|
| c | createdAt of last item |
| i | ObjectID of last item |

---

## Request/Response DTOs

### Query Parameters DTO

```go
// DiscoverQuery for GET /feed/discover
type DiscoverQuery struct {
    Limit    int    `form:"limit,default=20" binding:"min=1,max=50"`
    Cursor   string `form:"cursor"`
    Category string `form:"category,default=trending"`
    Tag      string `form:"tag"`
}

// Valid categories
const (
    CategoryTrending = "trending"
    CategoryPopular  = "popular"
    CategoryRecent   = "recent"
)
```

### Discovery Cursor DTO

```go
// DiscoverCursor for pagination
type DiscoverCursor struct {
    Score     *int               `json:"s,omitempty"` // Only for scored categories
    CreatedAt time.Time          `json:"c"`
    AnchorID  primitive.ObjectID `json:"i"`
}
```

### Discovery Feed Item Author

```go
// DiscoverItemAuthor extends FeedItemAuthor with follower count
type DiscoverItemAuthor struct {
    ID             primitive.ObjectID `json:"id"`
    Username       string             `json:"username"`
    DisplayName    string             `json:"displayName"`
    ProfilePicture *string            `json:"profilePicture"`
    IsVerified     bool               `json:"isVerified"`
    FollowerCount  int                `json:"followerCount"`
}
```

### Discovery Feed Item

```go
// DiscoverItem extends FeedItem with engagement score
type DiscoverItem struct {
    ID              primitive.ObjectID   `json:"id"`
    Title           string               `json:"title"`
    Description     string               `json:"description"`
    CoverMediaType  string               `json:"coverMediaType"`
    CoverMediaValue string               `json:"coverMediaValue"`
    Visibility      string               `json:"visibility"`
    IsPinned        bool                 `json:"isPinned"`
    Tags            []string             `json:"tags"`
    ItemCount       int                  `json:"itemCount"`
    LikeCount       int                  `json:"likeCount"`
    CloneCount      int                  `json:"cloneCount"`
    CommentCount    int                  `json:"commentCount"`
    EngagementScore int                  `json:"engagementScore"`
    LastItemAddedAt time.Time            `json:"lastItemAddedAt"`
    CreatedAt       time.Time            `json:"createdAt"`
    Author          DiscoverItemAuthor   `json:"author"`
    Engagement      FeedEngagement       `json:"engagement"` // Reuse from feed
    Preview         FeedPreview          `json:"preview"`    // Reuse from feed
}
```

### Discovery Meta

```go
// DiscoverMeta for discovery feed metadata
type DiscoverMeta struct {
    FeedType        string  `json:"feedType"`
    Category        string  `json:"category"`
    Tag             *string `json:"tag"`
    IsAuthenticated bool    `json:"isAuthenticated"`
    EmptyReason     *string `json:"emptyReason"`
}
```

### Discovery Response

```go
// DiscoverResponse for the complete discovery feed response
type DiscoverResponse struct {
    Items      []DiscoverItem `json:"items"`
    Pagination FeedPagination `json:"pagination"` // Reuse from feed
    Meta       DiscoverMeta   `json:"meta"`
}
```

---

## Business Logic

### Discovery Feed Algorithm

```
Function GetDiscoverFeed(currentUserID *ObjectID, query *DiscoverQuery):

    1. VALIDATE INPUT
       - Validate category (trending, popular, recent)
       - Validate cursor format if provided
       - Validate tag format if provided
       - Ensure limit is within bounds (1-50)

    2. BUILD EXCLUSION LIST (if authenticated)
       If currentUserID != nil:
           - Get all following IDs
           - Add currentUserID to exclusion list
       Else:
           - Exclusion list is empty

    3. BUILD BASE FILTER
       filter = {
           visibility: "public",  // NOT unlisted!
           deletedAt: null
       }
       
       If exclusionList is not empty:
           filter["userId"] = { "$nin": exclusionList }
       
       If tag is provided:
           filter["tags"] = tag.toLowerCase()

    4. APPLY CATEGORY-SPECIFIC LOGIC
       
       If category == "trending":
           // Only anchors from last 48 hours
           cutoff = now - 48 hours
           filter["createdAt"] = { "$gte": cutoff }
           sort = { engagementScore: -1, createdAt: -1, _id: -1 }
           
       If category == "popular":
           // All time, sorted by score
           sort = { engagementScore: -1, createdAt: -1, _id: -1 }
           
       If category == "recent":
           // Pure chronological
           sort = { createdAt: -1, _id: -1 }

    5. APPLY CURSOR PAGINATION
       If cursor provided:
           cursorData = decodeCursor(cursor)
           
           If category == "recent":
               filter["$or"] = [
                   { createdAt: { $lt: cursorData.createdAt } },
                   { 
                       createdAt: cursorData.createdAt,
                       _id: { $lt: cursorData.anchorID }
                   }
               ]
           Else (trending/popular):
               filter["$or"] = [
                   { engagementScore: { $lt: cursorData.score } },
                   { 
                       engagementScore: cursorData.score,
                       createdAt: { $lt: cursorData.createdAt }
                   },
                   {
                       engagementScore: cursorData.score,
                       createdAt: cursorData.createdAt,
                       _id: { $lt: cursorData.anchorID }
                   }
               ]

    6. EXECUTE QUERY
       - Apply filter and sort
       - Limit to (limit + 1) to check hasMore
       - Execute query

    7. CHECK FOR MORE
       If results.length > limit:
           hasMore = true
           Remove last item
       Else:
           hasMore = false

    8. HANDLE EMPTY RESULTS
       If no results:
           If tag provided:
               emptyReason = "NO_TAG_CONTENT"
           Else If exclusion list contains all content creators:
               emptyReason = "FOLLOWING_ALL"
           Else If cursor provided:
               emptyReason = "END_OF_FEED"
           Else:
               emptyReason = "NO_CONTENT"
       Return empty response with emptyReason

    9. ENRICH ANCHORS
       
       a. Collect unique author IDs
       b. Batch fetch authors with follower counts
       
       c. If authenticated:
          - Batch check which anchors user has liked
          - Batch check which anchors user has cloned
          - Get like summaries (with following priority)
       Else:
          - Set hasLiked/hasCloned to false
          - Get like summaries (no following priority)
       
       d. Get preview items for each anchor

    10. BUILD CURSOR FOR NEXT PAGE
        If hasMore:
            lastAnchor = results[len-1]
            If category == "recent":
                nextCursor = encodeCursor(nil, lastAnchor.CreatedAt, lastAnchor.ID)
            Else:
                nextCursor = encodeCursor(lastAnchor.EngagementScore, lastAnchor.CreatedAt, lastAnchor.ID)
        Else:
            nextCursor = null

    11. BUILD AND RETURN RESPONSE
        - Map anchors to DiscoverItem objects
        - Include pagination info
        - Include meta info with category and tag

    Return DiscoverResponse
```

---

## Database Changes

### Anchor Model Update

Add to existing Anchor struct in `anchors/model.go`:

```go
type Anchor struct {
    // ... existing fields ...
    
    EngagementScore int `bson:"engagementScore" json:"engagementScore"`
}
```

### New Indexes for Discovery

Add in `anchors/repository.go` NewRepository:

```go
// Discovery feed index - public anchors sorted by engagement
{
    Keys: bson.D{
        {Key: "visibility", Value: 1},
        {Key: "deletedAt", Value: 1},
        {Key: "engagementScore", Value: -1},
        {Key: "createdAt", Value: -1},
        {Key: "_id", Value: -1},
    },
},

// Discovery with tag filter
{
    Keys: bson.D{
        {Key: "visibility", Value: 1},
        {Key: "deletedAt", Value: 1},
        {Key: "tags", Value: 1},
        {Key: "engagementScore", Value: -1},
    },
},
```

### Anchors Repository - Add UpdateEngagementScore

```go
// UpdateEngagementScore recalculates and updates the engagement score
func (r *Repository) UpdateEngagementScore(ctx context.Context, anchorID primitive.ObjectID) error {
    anchor, err := r.GetAnchorByID(ctx, anchorID)
    if err != nil {
        return err
    }
    
    // Calculate score: (likes * 2) + (clones * 3) + (comments * 1)
    score := (anchor.LikeCount * 2) + (anchor.CloneCount * 3) + (anchor.CommentCount * 1)
    
    return r.UpdateAnchor(ctx, anchorID, bson.M{
        "engagementScore": score,
        "updatedAt":       time.Now(),
    })
}
```

### Likes Handler - Trigger Score Update

In `likes/handler.go` LikeAction, after incrementing like count:

```go
// After successful like/unlike
go func() {
    _ = h.anchorsRepo.UpdateEngagementScore(context.Background(), anchorID)
}()
```

### Anchors Handler - Trigger Score Update on Clone

In `anchors/handler.go` CloneAnchor, after creating clone:

```go
// After successful clone, update source anchor's score
go func() {
    _ = h.repo.UpdateEngagementScore(context.Background(), sourceAnchorID)
}()
```

---

## Business Rules

### Content Rules

| # | Rule |
|---|------|
| 1 | Only show anchors with visibility = "public" (NOT unlisted) |
| 2 | Never show deleted anchors |
| 3 | Exclude anchors from followed users (if authenticated) |
| 4 | Exclude current user's own anchors (if authenticated) |
| 5 | Trending category only shows content from last 48 hours |

### Scoring Rules

| # | Rule |
|---|------|
| 6 | engagementScore = (likeCount × 2) + (cloneCount × 3) + (commentCount × 1) |
| 7 | Score is recalculated on like/unlike and clone |
| 8 | Tie-breaking: score → createdAt → _id |

### Pagination Rules

| # | Rule |
|---|------|
| 9 | Default limit is 20 |
| 10 | Maximum limit is 50 |
| 11 | Cursor format varies by category |
| 12 | Invalid cursor returns error |

### Tag Rules

| # | Rule |
|---|------|
| 13 | Tags are case-insensitive |
| 14 | Tag filter can combine with any category |
| 15 | Non-existent tag returns empty with NO_TAG_CONTENT |

---

## File Structure

Extend the existing feed module:

```
internal/features/feed/
├── model.go           # Add DiscoverItem, DiscoverMeta, etc.
├── cursor.go          # Add EncodeDiscoverCursor, DecodeDiscoverCursor
├── repository.go      # Add GetDiscoverAnchors method
├── service.go         # Add GetDiscoverFeed method
├── handler.go         # Add GetDiscoverFeed handler
├── validator.go       # Add ValidateDiscoverQuery
└── routes.go          # Add discover route
```

---

## Repository Methods

### New Method in feed/repository.go

```go
// GetDiscoverAnchors retrieves anchors for discovery feed
func (r *Repository) GetDiscoverAnchors(
    ctx context.Context,
    excludeUserIDs []primitive.ObjectID,
    category string,
    tag *string,
    cursor *DiscoverCursor,
    limit int,
) ([]anchors.Anchor, error)
```

---

## Service Methods

### New Method in feed/service.go

```go
// GetDiscoverFeed returns discovery feed with trending/popular content
func (s *Service) GetDiscoverFeed(
    ctx context.Context,
    userID *primitive.ObjectID, // nil if not authenticated
    query *DiscoverQuery,
) (*DiscoverResponse, error)
```

---

## Handler

### New Handler Method

```go
// GetDiscoverFeed godoc
// @Summary Get discovery feed
// @Description Get discovery feed of trending/popular public anchors
// @Tags feed
// @Produce json
// @Param limit query int false "Items per page (default 20, max 50)"
// @Param cursor query string false "Pagination cursor"
// @Param category query string false "Category: trending, popular, recent (default trending)"
// @Param tag query string false "Filter by tag"
// @Success 200 {object} response.APIResponse{data=DiscoverResponse}
// @Failure 400 {object} response.APIResponse
// @Router /feed/discover [get]
func (h *Handler) GetDiscoverFeed(c *gin.Context)
```

---

## Route Registration

Update `feed/routes.go`:

```go
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
    // ... existing initialization ...
    
    // Initialize optional auth middleware
    optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)
    
    feed := router.Group("/feed")
    {
        // Existing - requires auth
        feed.GET("/following", authMiddleware, handler.GetFollowingFeed)
        
        // New - optional auth
        feed.GET("/discover", optionalAuth, handler.GetDiscoverFeed)
    }
}
```

---

## Implementation Order

### Step 1: Update Anchor Model
- Add EngagementScore field to Anchor struct in `anchors/model.go`

### Step 2: Add Discovery Indexes
- Add indexes in `anchors/repository.go` NewRepository

### Step 3: Add UpdateEngagementScore Method
- Add method to `anchors/repository.go`

### Step 4: Update Likes Handler
- Call UpdateEngagementScore after like/unlike in `likes/handler.go`

### Step 5: Update Anchors Handler (Clone)
- Call UpdateEngagementScore after clone in `anchors/handler.go`

### Step 6: Update feed/model.go
- Add DiscoverQuery
- Add DiscoverCursor
- Add DiscoverItemAuthor
- Add DiscoverItem
- Add DiscoverMeta
- Add DiscoverResponse
- Add category constants

### Step 7: Update feed/cursor.go
- Add EncodeDiscoverCursor
- Add DecodeDiscoverCursor

### Step 8: Update feed/validator.go
- Add ValidateDiscoverQuery
- Add ValidateCategory

### Step 9: Update feed/repository.go
- Add GetDiscoverAnchors method

### Step 10: Update feed/service.go
- Add GetDiscoverFeed method
- Add enrichDiscoverAuthors helper (includes followerCount)

### Step 11: Update feed/handler.go
- Add GetDiscoverFeed handler

### Step 12: Update feed/routes.go
- Add optional auth middleware
- Add GET /feed/discover route

---

## Migration: Backfill Engagement Scores

For existing anchors without engagement scores, run once:

```go
// Can be called during app startup or as a one-time migration
func BackfillEngagementScores(ctx context.Context, db *mongo.Database) error {
    collection := db.Collection("anchors")
    
    // Find anchors without engagementScore or with 0
    cursor, err := collection.Find(ctx, bson.M{
        "$or": []bson.M{
            {"engagementScore": bson.M{"$exists": false}},
            {"engagementScore": 0},
        },
    })
    if err != nil {
        return err
    }
    defer cursor.Close(ctx)
    
    for cursor.Next(ctx) {
        var anchor struct {
            ID           primitive.ObjectID `bson:"_id"`
            LikeCount    int                `bson:"likeCount"`
            CloneCount   int                `bson:"cloneCount"`
            CommentCount int                `bson:"commentCount"`
        }
        if err := cursor.Decode(&anchor); err != nil {
            continue
        }
        
        score := (anchor.LikeCount * 2) + (anchor.CloneCount * 3) + (anchor.CommentCount * 1)
        
        _, _ = collection.UpdateOne(ctx, 
            bson.M{"_id": anchor.ID},
            bson.M{"$set": bson.M{"engagementScore": score}},
        )
    }
    
    return nil
}
```

---

## Testing Scenarios

### Basic Discovery
- [ ] Get discover feed without auth
- [ ] Get discover feed with auth
- [ ] Default category is trending
- [ ] Custom limit works
- [ ] Pagination with cursor works

### Categories
- [ ] Trending shows only last 48 hours
- [ ] Trending sorted by engagement score
- [ ] Popular shows all time
- [ ] Popular sorted by engagement score
- [ ] Recent sorted by createdAt DESC
- [ ] Invalid category returns error

### Tag Filtering
- [ ] Filter by existing tag returns results
- [ ] Filter by non-existent tag returns empty
- [ ] Tag is case-insensitive
- [ ] Tag + category combination works

### Content Filtering
- [ ] Private anchors never shown
- [ ] Unlisted anchors never shown
- [ ] Deleted anchors never shown
- [ ] Followed users' anchors excluded (if auth)
- [ ] Own anchors excluded (if auth)
- [ ] All public content shown (if no auth)

### Engagement Score
- [ ] Score calculated correctly
- [ ] Score updates on like
- [ ] Score updates on unlike
- [ ] Score updates on clone

### Empty States
- [ ] No public content returns NO_CONTENT
- [ ] No tag content returns NO_TAG_CONTENT
- [ ] End of feed returns END_OF_FEED

### Author Enrichment
- [ ] Author includes followerCount
- [ ] Author info correct for each anchor

### Engagement Data (Authenticated)
- [ ] hasLiked correct
- [ ] hasCloned correct
- [ ] likeSummary populated

### Engagement Data (Unauthenticated)
- [ ] hasLiked always false
- [ ] hasCloned always false
- [ ] likeSummary still populated (no following priority)

---

## Performance Considerations

### Index Usage
- Compound index on (visibility, deletedAt, engagementScore, createdAt)
- Tag index for filtered queries

### Score Updates
- Update score asynchronously (goroutine) to not block main request
- Score slightly stale is acceptable for discovery

### Exclusion List
- For users following 1000+ people, $nin can be slow
- Consider caching following list
- Or limit exclusion to most recent N follows

### Caching (Future)
- Cache trending results for 5 minutes
- Invalidate on new high-engagement anchor
- Per-user cache for exclusion list

---

## Future Enhancements

### Personalized Discovery
- Based on user's liked/cloned anchors' tags
- Similar users' interests
- ML-based recommendations

### Geographic Discovery
- Location-based content
- Regional trending

### Topic Feeds
- Pre-defined topics (Tech, Design, Finance, etc.)
- User-created topic feeds

### Social Proof
- "Rising fast" badges
- "Staff picks" curation
- "New creator" highlights