# Follow System Specification

## Overview

The Follow System enables users to follow and unfollow other users, building a social graph for the Anchor app. This powers the Home Feed (content from followed users) and social discovery.

---

## API Summary

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 1 | `POST` | `/users/:id/follow` | Required | Follow or unfollow user |
| 2 | `GET` | `/users/:id/follow/status` | Required | Check follow status (mutual) |
| 3 | `GET` | `/users/:id/follows` | Optional | List followers OR following |

**Total: 3 endpoints**

---

## Data Models

### Follow Collection (`follows`)

```go
type Follow struct {
    ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    FollowerID  primitive.ObjectID `bson:"followerId" json:"followerId"`   // User who is following
    FollowingID primitive.ObjectID `bson:"followingId" json:"followingId"` // User being followed
    CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
}
```

### User Model (existing - in auth module)

Already contains:
- `followerCount` (int) - Number of users following this user
- `followingCount` (int) - Number of users this user follows

---

## Database Indexes

```go
// In NewRepository constructor

// 1. Unique compound index - prevents duplicate follows
{
    Keys: bson.D{
        {Key: "followerId", Value: 1}, 
        {Key: "followingId", Value: 1}
    }, 
    Options: options.Index().SetUnique(true)
}

// 2. Query followers of a user (sorted by newest first)
{
    Keys: bson.D{
        {Key: "followingId", Value: 1}, 
        {Key: "createdAt", Value: -1}
    }
}

// 3. Query who a user follows (sorted by newest first)
{
    Keys: bson.D{
        {Key: "followerId", Value: 1}, 
        {Key: "createdAt", Value: -1}
    }
}
```

---

## API Endpoints

### 1. Follow/Unfollow User

**Endpoint:** `POST /users/:id/follow`

**Authentication:** Required

**Description:** Follow or unfollow a user based on the action specified. Idempotent - calling follow twice returns success.

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Target user's ID (ObjectID) |

**Request Body:**
```json
{
    "action": "follow"
}
```

| Field | Type | Required | Values | Description |
|-------|------|----------|--------|-------------|
| action | string | Yes | `follow`, `unfollow` | Action to perform |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "isFollowing": true,
        "targetUser": {
            "id": "507f1f77bcf86cd799439011",
            "username": "@johndoe",
            "displayName": "John Doe",
            "followerCount": 151
        },
        "currentUser": {
            "followingCount": 76
        }
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid user ID format |
| 400 | INVALID_ACTION | Action must be 'follow' or 'unfollow' |
| 400 | CANNOT_FOLLOW_SELF | You cannot follow yourself |
| 401 | UNAUTHORIZED | Authentication required |
| 404 | USER_NOT_FOUND | User not found |

**Business Logic:**

**Follow Action:**
1. Validate target user exists and is not deleted
2. Check if user is trying to follow themselves → error
3. Check if follow already exists → return success (idempotent)
4. Create follow record
5. Increment target user's `followerCount` by 1
6. Increment current user's `followingCount` by 1
7. Return updated state

**Unfollow Action:**
1. Validate target user exists
2. Check if user is trying to unfollow themselves → error  
3. Check if follow exists → if not, return success (idempotent)
4. Delete follow record
5. Decrement target user's `followerCount` by 1
6. Decrement current user's `followingCount` by 1
7. Return updated state

---

### 2. Check Follow Status

**Endpoint:** `GET /users/:id/follow/status`

**Authentication:** Required

**Description:** Check if the current user follows the target user, and if the target follows back (mutual follow detection).

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Target user's ID |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "isFollowing": true,
        "isFollowedBy": false,
        "isMutual": false
    }
}
```

| Field | Type | Description |
|-------|------|-------------|
| isFollowing | bool | Does current user follow target? |
| isFollowedBy | bool | Does target follow current user? |
| isMutual | bool | Both follow each other? |

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid user ID format |
| 401 | UNAUTHORIZED | Authentication required |
| 404 | USER_NOT_FOUND | User not found |

---

### 3. List Followers/Following

**Endpoint:** `GET /users/:id/follows`

**Authentication:** Optional (needed for `isFollowing` field in response)

**Description:** Get paginated list of followers OR users being followed, based on `type` parameter.

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Target user's ID |

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| type | string | Yes | - | `followers` or `following` |
| page | int | No | 1 | Page number (min 1) |
| limit | int | No | 20 | Items per page (min 1, max 50) |

**Examples:**
```
GET /users/507f1f77bcf86cd799439011/follows?type=followers&page=1&limit=20
GET /users/507f1f77bcf86cd799439011/follows?type=following&page=2&limit=10
```

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
                "username": "@janedoe",
                "displayName": "Jane Doe",
                "profilePicture": "https://cloudinary.com/...",
                "bio": "Designer & Creator",
                "isFollowing": true,
                "followedAt": "2024-01-15T10:30:00Z"
            },
            {
                "id": "507f1f77bcf86cd799439013",
                "username": "@bobsmith",
                "displayName": "Bob Smith",
                "profilePicture": null,
                "bio": "",
                "isFollowing": false,
                "followedAt": "2024-01-14T08:20:00Z"
            }
        ],
        "pagination": {
            "page": 1,
            "limit": 20,
            "total": 150,
            "totalPages": 8,
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
| bio | string | User bio |
| isFollowing | bool | Does current user follow this person? (only if authenticated) |
| followedAt | datetime | When the follow relationship was created |

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid user ID format |
| 400 | INVALID_TYPE | Type must be 'followers' or 'following' |
| 404 | USER_NOT_FOUND | User not found |

---

## Request/Response DTOs

### Request DTOs

```go
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
```

### Response DTOs

```go
// FollowActionResponse after follow/unfollow
type FollowActionResponse struct {
    IsFollowing bool                    `json:"isFollowing"`
    TargetUser  FollowTargetUserInfo    `json:"targetUser"`
    CurrentUser FollowCurrentUserInfo   `json:"currentUser"`
}

type FollowTargetUserInfo struct {
    ID            primitive.ObjectID `json:"id"`
    Username      string             `json:"username"`
    DisplayName   string             `json:"displayName"`
    FollowerCount int                `json:"followerCount"`
}

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
```

---

## Business Rules

### Core Rules

| # | Rule | Error Code |
|---|------|------------|
| 1 | Cannot follow yourself | CANNOT_FOLLOW_SELF |
| 2 | Cannot follow deleted users | USER_NOT_FOUND |
| 3 | One follow record per user pair (unique constraint) | - |

### Idempotency Rules

| # | Scenario | Behavior |
|---|----------|----------|
| 4 | Follow when already following | Return success with current state |
| 5 | Unfollow when not following | Return success |

### Count Synchronization

| # | Rule |
|---|------|
| 6 | On follow: increment target's followerCount, increment self's followingCount |
| 7 | On unfollow: decrement target's followerCount, decrement self's followingCount |
| 8 | Count update failures should not fail the main operation (log error, continue) |

---

## Validation Rules

### Action Validation
- `action` field is required
- `action` must be exactly "follow" or "unfollow"

### Type Validation (for list endpoint)
- `type` query parameter is required
- `type` must be exactly "followers" or "following"

### Pagination Validation
- `page` minimum is 1, default is 1
- `limit` minimum is 1, maximum is 50, default is 20

### User ID Validation
- Must be valid 24-character hex string (MongoDB ObjectID)
- User must exist in database
- User must not be soft-deleted

---

## File Structure

```
internal/features/follows/
├── model.go       # Follow struct, all DTOs, constants
├── repository.go  # Database operations with indexes
├── handler.go     # 3 endpoint handlers
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
func (r *Repository) CreateFollow(ctx context.Context, followerID, followingID primitive.ObjectID) error
func (r *Repository) DeleteFollow(ctx context.Context, followerID, followingID primitive.ObjectID) error
func (r *Repository) ExistsFollow(ctx context.Context, followerID, followingID primitive.ObjectID) (bool, error)

// Status check (both directions)
func (r *Repository) GetFollowStatus(ctx context.Context, userID, targetID primitive.ObjectID) (isFollowing bool, isFollowedBy bool, error)

// List operations with pagination
func (r *Repository) GetFollowers(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]Follow, int64, error)
func (r *Repository) GetFollowing(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]Follow, int64, error)

// Batch operations (for enriching user lists)
func (r *Repository) GetFollowingIDs(ctx context.Context, userID primitive.ObjectID, targetIDs []primitive.ObjectID) (map[primitive.ObjectID]bool, error)
```

---

## Auth Repository Updates Required

Add these methods to `internal/features/auth/repository.go`:

```go
// IncrementFollowerCount increments or decrements a user's follower count
func (r *Repository) IncrementFollowerCount(ctx context.Context, userID primitive.ObjectID, delta int) error {
    filter := bson.M{"_id": userID}
    update := bson.M{
        "$inc": bson.M{"followerCount": delta},
        "$set": bson.M{"updatedAt": time.Now()},
    }
    _, err := r.collection.UpdateOne(ctx, filter, update)
    return err
}

// IncrementFollowingCount increments or decrements a user's following count
func (r *Repository) IncrementFollowingCount(ctx context.Context, userID primitive.ObjectID, delta int) error {
    filter := bson.M{"_id": userID}
    update := bson.M{
        "$inc": bson.M{"followingCount": delta},
        "$set": bson.M{"updatedAt": time.Now()},
    }
    _, err := r.collection.UpdateOne(ctx, filter, update)
    return err
}

// GetUsersByIDs fetches multiple users by their IDs (for enriching follow lists)
func (r *Repository) GetUsersByIDs(ctx context.Context, userIDs []primitive.ObjectID) ([]User, error) {
    filter := bson.M{"_id": bson.M{"$in": userIDs}}
    cursor, err := r.collection.Find(ctx, filter)
    if err != nil {
        return nil, err
    }
    defer cursor.Close(ctx)
    
    var users []User
    if err = cursor.All(ctx, &users); err != nil {
        return nil, err
    }
    return users, nil
}
```

---

## Route Registration

```go
// internal/features/follows/routes.go

package follows

import (
    "github.com/gin-gonic/gin"
    "github.com/xyz-asif/gotodo/internal/config"
    "github.com/xyz-asif/gotodo/internal/features/auth"
    "github.com/xyz-asif/gotodo/internal/middleware"
    "go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
    // Initialize repositories
    repo := NewRepository(db)
    authRepo := auth.NewRepository(db)

    // Initialize handler
    handler := NewHandler(repo, authRepo, cfg)

    // Initialize middlewares
    authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)
    optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

    // Follow routes under /users
    users := router.Group("/users")
    {
        // Protected routes (require authentication)
        users.POST("/:id/follow", authMiddleware, handler.FollowAction)
        users.GET("/:id/follow/status", authMiddleware, handler.GetFollowStatus)
        
        // Public route with optional auth
        users.GET("/:id/follows", optionalAuth, handler.ListFollows)
    }
}
```

**Note:** Register this in `internal/routes/routes.go` alongside other feature routes.

---

## Implementation Order

### Step 1: Model & DTOs
Create `model.go` with Follow struct and all DTOs

### Step 2: Repository
Create `repository.go` with all database operations and indexes

### Step 3: Auth Repository Updates
Add `IncrementFollowerCount`, `IncrementFollowingCount`, `GetUsersByIDs` to auth repository

### Step 4: Validator
Create `validator.go` with validation functions

### Step 5: Handler
Create `handler.go` with all 3 handlers

### Step 6: Routes
Create `routes.go` and register in main routes file

---

## Testing Scenarios

### Follow/Unfollow Action
- [ ] Follow a new user successfully
- [ ] Follow same user again (idempotent - returns success)
- [ ] Unfollow a user successfully  
- [ ] Unfollow same user again (idempotent - returns success)
- [ ] Try to follow self (error: CANNOT_FOLLOW_SELF)
- [ ] Try to follow non-existent user (error: USER_NOT_FOUND)
- [ ] Try to follow without auth (error: UNAUTHORIZED)
- [ ] Verify followerCount increments on follow
- [ ] Verify followingCount increments on follow
- [ ] Verify counts decrement on unfollow

### Follow Status
- [ ] Check status when following target
- [ ] Check status when not following target
- [ ] Check status when target follows back (mutual)
- [ ] Check status for non-existent user (error)
- [ ] Check status without auth (error: UNAUTHORIZED)

### List Followers/Following
- [ ] List followers with pagination (page 1)
- [ ] List followers page 2
- [ ] List following with pagination
- [ ] List with custom limit
- [ ] List empty (user has no followers)
- [ ] List with auth (shows isFollowing field)
- [ ] List without auth (isFollowing always false)
- [ ] List for non-existent user (error)
- [ ] List with invalid type (error: INVALID_TYPE)
- [ ] List without type param (error: INVALID_TYPE)

---

## Future Considerations

### Private Accounts (Not in current scope)
- Follow requests instead of instant follow
- Pending/Approved/Rejected states
- Privacy settings

### Blocking (Not in current scope)
- Block user prevents follow
- Blocked users don't appear in lists

### Notifications (Separate module)
- Notify when someone follows you
- Notify when mutual follow happens