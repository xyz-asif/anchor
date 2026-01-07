# Likes System Specification

## Overview

The Likes System enables users to like and unlike anchors, providing social engagement metrics. A key feature is the **"Liked by people you follow"** display, similar to Instagram, which prioritizes showing users that the viewer follows.

---

## API Summary

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 1 | `POST` | `/anchors/:id/like` | Required | Like or unlike anchor |
| 2 | `GET` | `/anchors/:id/like/status` | Required | Check if user liked anchor |
| 3 | `GET` | `/anchors/:id/likes` | Optional | List all users who liked (paginated) |

**Additional:** GetAnchor response includes `likeSummary` with "liked by following" data.

---

## Data Model

### Like Collection (`likes`)

```go
type Like struct {
    ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    AnchorID  primitive.ObjectID `bson:"anchorId" json:"anchorId"`
    UserID    primitive.ObjectID `bson:"userId" json:"userId"`
    CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}
```

### Anchor Model (existing - has likeCount)

The Anchor model already contains:
- `likeCount` (int) - Number of likes on this anchor

---

## Database Indexes

```go
// In NewRepository constructor

// 1. Unique compound index - prevents duplicate likes
{
    Keys: bson.D{
        {Key: "anchorId", Value: 1},
        {Key: "userId", Value: 1},
    },
    Options: options.Index().SetUnique(true),
}

// 2. Query likes for an anchor (sorted by recent first)
{
    Keys: bson.D{
        {Key: "anchorId", Value: 1},
        {Key: "createdAt", Value: -1},
    },
}

// 3. Query anchors a user has liked (for "liked anchors" page - future)
{
    Keys: bson.D{
        {Key: "userId", Value: 1},
        {Key: "createdAt", Value: -1},
    },
}
```

---

## API Endpoints

### 1. Like/Unlike Anchor

**Endpoint:** `POST /anchors/:id/like`

**Authentication:** Required

**Description:** Like or unlike an anchor based on the action specified. Idempotent - liking twice returns success.

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Anchor ID (ObjectID) |

**Request Body:**
```json
{
    "action": "like"
}
```

| Field | Type | Required | Values | Description |
|-------|------|----------|--------|-------------|
| action | string | Yes | `like`, `unlike` | Action to perform |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "hasLiked": true,
        "likeCount": 55
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid anchor ID format |
| 400 | INVALID_ACTION | Action must be 'like' or 'unlike' |
| 401 | UNAUTHORIZED | Authentication required |
| 403 | ACCESS_DENIED | Cannot like private anchor |
| 404 | ANCHOR_NOT_FOUND | Anchor not found or deleted |

**Business Logic:**

**Like Action:**
1. Validate anchor exists and is not deleted
2. Check anchor visibility - user must be able to view it (owner OR public/unlisted)
3. Check if like already exists → return success (idempotent)
4. Create like record
5. Increment anchor's `likeCount` by 1
6. Return updated state

**Unlike Action:**
1. Validate anchor exists
2. Check if like exists → if not, return success (idempotent)
3. Delete like record
4. Decrement anchor's `likeCount` by 1 (ensure doesn't go below 0)
5. Return updated state

**Note:** Users CAN like their own anchors (Instagram allows this).

---

### 2. Check Like Status

**Endpoint:** `GET /anchors/:id/like/status`

**Authentication:** Required

**Description:** Check if the current user has liked the anchor.

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Anchor ID |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "hasLiked": true,
        "likeCount": 55
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid anchor ID format |
| 401 | UNAUTHORIZED | Authentication required |
| 403 | ACCESS_DENIED | Cannot access private anchor |
| 404 | ANCHOR_NOT_FOUND | Anchor not found |

---

### 3. List Likers

**Endpoint:** `GET /anchors/:id/likes`

**Authentication:** Optional (needed for `isFollowing` field)

**Description:** Get paginated list of users who liked the anchor. Users that the current user follows appear with `isFollowing: true`.

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Anchor ID |

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| page | int | No | 1 | Page number (min 1) |
| limit | int | No | 20 | Items per page (min 1, max 50) |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "data": [
            {
                "id": "507f1f77bcf86cd799439012",
                "username": "@asif",
                "displayName": "Asif Ahmed",
                "profilePicture": "https://cloudinary.com/...",
                "isFollowing": true,
                "likedAt": "2024-01-15T10:30:00Z"
            },
            {
                "id": "507f1f77bcf86cd799439013",
                "username": "@kiran",
                "displayName": "Kiran Kumar",
                "profilePicture": null,
                "isFollowing": false,
                "likedAt": "2024-01-14T08:20:00Z"
            }
        ],
        "pagination": {
            "page": 1,
            "limit": 20,
            "total": 55,
            "totalPages": 3,
            "hasMore": true
        }
    }
}
```

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| id | string | User's ID |
| username | string | Username with @ prefix |
| displayName | string | Display name |
| profilePicture | string/null | Profile picture URL |
| isFollowing | bool | Does current user follow this person? (false if not authenticated) |
| likedAt | datetime | When they liked the anchor |

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid anchor ID format |
| 403 | ACCESS_DENIED | Cannot access private anchor |
| 404 | ANCHOR_NOT_FOUND | Anchor not found |

---

## Like Summary Feature ("Liked by Asif, Kiran...")

### Overview

When fetching an anchor, the response includes a `likeSummary` object that shows:
1. Total like count
2. Whether current user has liked
3. Up to 3 users who liked, **prioritizing users the viewer follows**
4. Count of other likers

### Updated GetAnchor Response

**Endpoint:** `GET /anchors/:id`

**Updated Response:**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "anchor": {
            "id": "...",
            "title": "My Bookmarks",
            "likeCount": 55,
            ...
        },
        "items": [...],
        "likeSummary": {
            "totalCount": 55,
            "hasLiked": true,
            "likedByFollowing": [
                {
                    "id": "507f1f77bcf86cd799439012",
                    "username": "@asif",
                    "displayName": "Asif Ahmed",
                    "profilePicture": "https://..."
                },
                {
                    "id": "507f1f77bcf86cd799439013",
                    "username": "@kiran",
                    "displayName": "Kiran Kumar",
                    "profilePicture": null
                }
            ],
            "otherLikersCount": 53
        }
    }
}
```

### Like Summary Fields

| Field | Type | Description |
|-------|------|-------------|
| totalCount | int | Total number of likes (same as anchor.likeCount) |
| hasLiked | bool | Has current user liked this? (false if not authenticated) |
| likedByFollowing | array | Up to 3 users who liked, prioritizing followed users |
| otherLikersCount | int | totalCount - likedByFollowing.length |

### Algorithm: Get Like Summary

```
Function GetLikeSummary(anchorID, currentUserID, followsRepo):

    1. Get anchor's likeCount from anchor document
    
    2. If likeCount == 0:
        Return {
            totalCount: 0,
            hasLiked: false,
            likedByFollowing: [],
            otherLikersCount: 0
        }
    
    3. Check if currentUser has liked (if authenticated):
        hasLiked = false
        If currentUserID != nil:
            hasLiked = ExistsLike(anchorID, currentUserID)
    
    4. Get recent likers (limit 20 for processing):
        recentLikes = GetRecentLikes(anchorID, limit: 20)
        likerIDs = extract userIDs from recentLikes
    
    5. If NOT authenticated:
        - Take first 3 likers
        - Fetch their user details
        - Return summary with these 3 users
    
    6. If authenticated - prioritize followed users:
        a. Get which likers the current user follows:
           followingMap = followsRepo.GetFollowingIDs(currentUserID, likerIDs)
        
        b. Separate into followed and not followed:
           followedLikerIDs = likerIDs where followingMap[id] == true
           otherLikerIDs = likerIDs where followingMap[id] == false
        
        c. Prioritize: followed first, then others (max 3 total):
           prioritizedIDs = concat(followedLikerIDs, otherLikerIDs).slice(0, 3)
        
        d. Fetch user details for prioritized users:
           users = authRepo.GetUsersByIDs(prioritizedIDs)
    
    7. Build response:
        likedByFollowing = users.map(u => {
            id: u.ID,
            username: u.Username,
            displayName: u.DisplayName,
            profilePicture: u.ProfilePictureURL
        })
        
        otherLikersCount = totalCount - len(likedByFollowing)
        
        Return {
            totalCount,
            hasLiked,
            likedByFollowing,
            otherLikersCount
        }
```

### Edge Cases for Like Summary

| Scenario | Result |
|----------|--------|
| Anchor has 0 likes | `{ totalCount: 0, hasLiked: false, likedByFollowing: [], otherLikersCount: 0 }` |
| Anchor has 1 like (from followed user) | `{ totalCount: 1, likedByFollowing: [user], otherLikersCount: 0 }` |
| Anchor has 1 like (from stranger) | `{ totalCount: 1, likedByFollowing: [stranger], otherLikersCount: 0 }` |
| Anchor has 55 likes, 2 from followed users | `{ totalCount: 55, likedByFollowing: [followed1, followed2, stranger1], otherLikersCount: 52 }` |
| User not authenticated | `{ hasLiked: false, likedByFollowing: [recent 3 likers] }` |
| User follows no one | Show 3 most recent likers |
| All likers are followed | Show first 3 followed users |

---

## Request/Response DTOs

### Request DTOs

```go
// LikeActionRequest for POST /anchors/:id/like
type LikeActionRequest struct {
    Action string `json:"action" binding:"required,oneof=like unlike"`
}

// LikeListQuery for GET /anchors/:id/likes
type LikeListQuery struct {
    Page  int `form:"page,default=1" binding:"min=1"`
    Limit int `form:"limit,default=20" binding:"min=1,max=50"`
}
```

### Response DTOs

```go
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
```

### Updated Anchor Response DTO

```go
// AnchorWithItemsResponse - updated to include likeSummary
type AnchorWithItemsResponse struct {
    Anchor      Anchor               `json:"anchor"`
    Items       []Item               `json:"items"`
    LikeSummary *LikeSummaryResponse `json:"likeSummary,omitempty"`
}
```

---

## Business Rules

### Core Rules

| # | Rule |
|---|------|
| 1 | Users can like public and unlisted anchors |
| 2 | Users can like their own anchors (including private) |
| 3 | Users cannot like others' private anchors |
| 4 | Users cannot like deleted anchors |
| 5 | One like per user per anchor (unique constraint) |

### Idempotency Rules

| # | Scenario | Behavior |
|---|----------|----------|
| 6 | Like when already liked | Return success with current state |
| 7 | Unlike when not liked | Return success |

### Count Synchronization

| # | Rule |
|---|------|
| 8 | On like: increment anchor's likeCount by 1 |
| 9 | On unlike: decrement anchor's likeCount by 1 |
| 10 | likeCount must never go below 0 |
| 11 | Count update failures should not fail the main operation |

### Like Summary Rules

| # | Rule |
|---|------|
| 12 | Show maximum 3 users in likedByFollowing |
| 13 | Prioritize users that viewer follows |
| 14 | If not authenticated, show recent likers (no priority) |
| 15 | If no followed users liked, show recent likers |

---

## Validation Rules

### Action Validation
- `action` field is required
- `action` must be exactly "like" or "unlike"

### Pagination Validation
- `page` minimum is 1, default is 1
- `limit` minimum is 1, maximum is 50, default is 20

### Anchor ID Validation
- Must be valid 24-character hex string (MongoDB ObjectID)
- Anchor must exist in database
- Anchor must not be soft-deleted

### Access Validation
- For private anchors: only owner can like/view likes
- For public/unlisted anchors: anyone can like/view likes

---

## File Structure

```
internal/features/likes/
├── model.go       # Like struct, all DTOs
├── repository.go  # Database operations with indexes
├── handler.go     # 3 endpoint handlers + GetLikeSummary helper
├── validator.go   # Request validation functions
└── routes.go      # Route registration
```

---

## Repository Methods

```go
type Repository struct {
    collection *mongo.Collection
}

// NewRepository creates repository and ensures indexes
func NewRepository(db *mongo.Database) *Repository

// Core operations
func (r *Repository) CreateLike(ctx context.Context, anchorID, userID primitive.ObjectID) error
func (r *Repository) DeleteLike(ctx context.Context, anchorID, userID primitive.ObjectID) error
func (r *Repository) ExistsLike(ctx context.Context, anchorID, userID primitive.ObjectID) (bool, error)

// List operations
func (r *Repository) GetLikers(ctx context.Context, anchorID primitive.ObjectID, page, limit int) ([]Like, int64, error)
func (r *Repository) GetRecentLikers(ctx context.Context, anchorID primitive.ObjectID, limit int) ([]Like, error)

// Batch operations (for checking if user liked multiple anchors - useful for feeds)
func (r *Repository) GetUserLikedAnchors(ctx context.Context, userID primitive.ObjectID, anchorIDs []primitive.ObjectID) (map[primitive.ObjectID]bool, error)
```

---

## Anchors Repository Updates

Add method to update like count:

```go
// IncrementLikeCount increments or decrements an anchor's like count
func (r *Repository) IncrementLikeCount(ctx context.Context, anchorID primitive.ObjectID, delta int) error {
    filter := bson.M{"_id": anchorID}
    
    // Use aggregation to ensure count doesn't go below 0
    update := bson.M{
        "$inc": bson.M{"likeCount": delta},
        "$set": bson.M{"updatedAt": time.Now()},
    }
    
    _, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
    if err != nil {
        return err
    }
    
    // If decrementing, ensure count doesn't go negative
    if delta < 0 {
        // Fix negative count if it occurred
        _, _ = r.anchorsCollection.UpdateOne(ctx,
            bson.M{"_id": anchorID, "likeCount": bson.M{"$lt": 0}},
            bson.M{"$set": bson.M{"likeCount": 0}},
        )
    }
    
    return nil
}
```

---

## Anchors Handler Updates

The GetAnchor handler needs to be updated to include `likeSummary`. This requires:

1. **Add dependencies to Handler struct:**
```go
type Handler struct {
    repo              *Repository
    authRepo          *auth.Repository
    config            *config.Config
    cloudinary        *cloudinary.Service
    likesRepo         *likes.Repository    // NEW
    followsRepo       *follows.Repository  // NEW
}
```

2. **Update NewHandler to accept new repos**

3. **Update GetAnchor handler** to call GetLikeSummary and include in response

4. **Add GetLikeSummary helper function** (can be in likes package or anchors package)

---

## Route Registration

```go
// internal/features/likes/routes.go

package likes

import (
    "github.com/gin-gonic/gin"
    "github.com/xyz-asif/gotodo/internal/config"
    "github.com/xyz-asif/gotodo/internal/features/anchors"
    "github.com/xyz-asif/gotodo/internal/features/auth"
    "github.com/xyz-asif/gotodo/internal/features/follows"
    "github.com/xyz-asif/gotodo/internal/middleware"
    "go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
    // Initialize repositories
    repo := NewRepository(db)
    authRepo := auth.NewRepository(db)
    anchorsRepo := anchors.NewRepository(db)
    followsRepo := follows.NewRepository(db)

    // Initialize handler
    handler := NewHandler(repo, authRepo, anchorsRepo, followsRepo, cfg)

    // Initialize middlewares
    authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)
    optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

    // Like routes under /anchors
    anchorsGroup := router.Group("/anchors")
    {
        // Protected routes
        anchorsGroup.POST("/:id/like", authMiddleware, handler.LikeAction)
        anchorsGroup.GET("/:id/like/status", authMiddleware, handler.GetLikeStatus)
        
        // Public route with optional auth
        anchorsGroup.GET("/:id/likes", optionalAuth, handler.ListLikers)
    }
}
```

---

## Implementation Order

### Step 1: Create Likes Module
1. Create `internal/features/likes/model.go` - Like struct and all DTOs
2. Create `internal/features/likes/repository.go` - Database operations with indexes
3. Create `internal/features/likes/validator.go` - Validation functions
4. Create `internal/features/likes/handler.go` - 3 handlers
5. Create `internal/features/likes/routes.go` - Route registration

### Step 2: Add IncrementLikeCount to Anchors Repository
Add the `IncrementLikeCount` method to `anchors/repository.go`

### Step 3: Create GetLikeSummary Helper
Create helper function that:
- Gets recent likers
- Cross-references with follows
- Returns prioritized list

### Step 4: Update Anchors Handler
1. Add `likesRepo` and `followsRepo` to Handler struct
2. Update `NewHandler` constructor
3. Update `GetAnchor` to include likeSummary in response
4. Update `routes.go` to pass new repositories

### Step 5: Update Main Routes
Register likes routes in `internal/routes/routes.go`

---

## Testing Scenarios

### Like/Unlike Action
- [ ] Like a public anchor successfully
- [ ] Like same anchor again (idempotent)
- [ ] Unlike an anchor successfully
- [ ] Unlike same anchor again (idempotent)
- [ ] Like own private anchor (allowed)
- [ ] Try to like others' private anchor (error: ACCESS_DENIED)
- [ ] Try to like deleted anchor (error: ANCHOR_NOT_FOUND)
- [ ] Try to like without auth (error: UNAUTHORIZED)
- [ ] Verify likeCount increments on like
- [ ] Verify likeCount decrements on unlike
- [ ] Verify likeCount never goes below 0

### Like Status
- [ ] Check status when liked
- [ ] Check status when not liked
- [ ] Check status for inaccessible anchor (error)
- [ ] Check status without auth (error: UNAUTHORIZED)

### List Likers
- [ ] List with pagination (page 1)
- [ ] List page 2
- [ ] List with custom limit
- [ ] List empty (anchor has 0 likes)
- [ ] List with auth (shows isFollowing)
- [ ] List without auth (isFollowing always false)
- [ ] List for private anchor by owner (allowed)
- [ ] List for private anchor by others (error: ACCESS_DENIED)

### Like Summary (in GetAnchor)
- [ ] Anchor with 0 likes
- [ ] Anchor with 1 like from followed user
- [ ] Anchor with 1 like from stranger
- [ ] Anchor with many likes, some from followed users
- [ ] Anchor with many likes, none from followed users
- [ ] View as unauthenticated user (shows recent, no priority)
- [ ] User follows no one (shows recent likers)
- [ ] Verify likedByFollowing has max 3 users
- [ ] Verify otherLikersCount = totalCount - likedByFollowing.length

---

## Performance Considerations

### For Like Summary
- Only fetch 20 recent likers (not all)
- Use batch query for follow status (GetFollowingIDs)
- Use batch query for user details (GetUsersByIDs)

### For Feeds (Future)
- `GetUserLikedAnchors` method allows batch checking if user liked multiple anchors
- Useful when displaying a list of anchors with like status

### Caching (Future Optimization)
- Cache like counts (they're already in anchor document)
- Cache like summary for popular anchors
- Invalidate on like/unlike

---

## Future Considerations

### Unlike Confirmation (Not in current scope)
- Prompt user before unliking
- "Are you sure you want to unlike?"

### Like Notifications (Separate module)
- Notify anchor owner when someone likes
- Batch notifications for multiple likes

### Like Analytics (Not in current scope)
- Track likes over time
- Show "likes this week" trend

### Double-tap to Like (Client-side)
- UI feature, no backend changes needed