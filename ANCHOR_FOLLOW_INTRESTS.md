# Anchor Follows & Interests Specification

## Overview

This feature allows users to follow anchors (not just people), receive update notifications, and discover content through personalized interest-based categories.

---

## Feature Summary

| Feature | Description |
|---------|-------------|
| **Follow Anchors** | Follow any public/unlisted anchor |
| **Update Tracking** | See when followed anchors have new content |
| **Update Notifications** | Opt-in alerts when anchor is updated |
| **Home Feed Integration** | "Following Anchors" section with update indicators |
| **Interest Categories** | Personalized topic suggestions based on user behavior |

---

## New API Endpoints (6 Total)

### Anchor Follow Endpoints (4)

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 1 | `POST` | `/anchors/:id/follow` | Required | Follow/unfollow anchor |
| 2 | `GET` | `/anchors/:id/follow/status` | Required | Get follow status |
| 3 | `PATCH` | `/anchors/:id/follow/notifications` | Required | Toggle update notifications |
| 4 | `GET` | `/users/me/following-anchors` | Required | List followed anchors |

### Interest Endpoints (1)

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 5 | `GET` | `/interests/suggested` | Optional | Get suggested categories |

### Updated Endpoints (1)

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 6 | `GET` | `/feed/home` | Required | **Updated** - Includes followed anchors |

---

## Data Models

### New Collection: `anchor_follows`

```go
type AnchorFollow struct {
    ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    UserID          primitive.ObjectID `bson:"userId" json:"userId"`
    AnchorID        primitive.ObjectID `bson:"anchorId" json:"anchorId"`
    NotifyOnUpdate  bool               `bson:"notifyOnUpdate" json:"notifyOnUpdate"`
    LastSeenVersion int                `bson:"lastSeenVersion" json:"lastSeenVersion"`
    CreatedAt       time.Time          `bson:"createdAt" json:"createdAt"`
}
```

**Indexes:**
- `(userId, anchorId)` - unique compound
- `(anchorId)` - for counting followers
- `(userId, notifyOnUpdate)` - for listing with filters

### Updates to Anchors Collection

Add these fields to existing Anchor struct:

```go
type Anchor struct {
    // ... existing fields ...
    
    Version         int        `bson:"version" json:"version"`                   // Increments on content updates
    LastItemAddedAt *time.Time `bson:"lastItemAddedAt" json:"lastItemAddedAt"`   // When last item was added
    FollowerCount   int        `bson:"followerCount" json:"followerCount"`       // How many follow this anchor
}
```

### New Notification Type

Add to notifications/model.go:

```go
const (
    TypeMention      = "mention"
    TypeComment      = "comment"
    TypeLike         = "like"
    TypeFollow       = "follow"
    TypeClone        = "clone"
    TypeAnchorUpdate = "anchor_update"  // NEW
)
```

---

## API Endpoint Details

### 1. Follow/Unfollow Anchor

**Endpoint:** `POST /anchors/:id/follow`

**Authentication:** Required

**Request Body:**
```json
{
    "action": "follow",
    "notifyOnUpdate": true
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| action | string | Yes | "follow" or "unfollow" |
| notifyOnUpdate | bool | No | Enable update notifications (default: false) |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "isFollowing": true,
        "notifyOnUpdate": true,
        "followerCount": 156
    }
}
```

**Error Responses:**

| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ACTION | Action must be 'follow' or 'unfollow' |
| 400 | CANNOT_FOLLOW_OWN | Cannot follow your own anchor |
| 403 | ACCESS_DENIED | Cannot follow private anchor |
| 404 | ANCHOR_NOT_FOUND | Anchor not found |

**Business Rules:**
- Cannot follow own anchors
- Cannot follow private anchors (unless owner, but owners can't follow own)
- Can follow public and unlisted anchors
- Follow is idempotent (following twice = still following)
- Unfollow deletes the record (hard delete)

---

### 2. Get Follow Status

**Endpoint:** `GET /anchors/:id/follow/status`

**Authentication:** Required

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "isFollowing": true,
        "notifyOnUpdate": true,
        "hasUpdates": true,
        "updatesSinceLastSeen": 3,
        "lastSeenVersion": 5,
        "currentVersion": 8,
        "followedAt": "2024-01-01T00:00:00Z"
    }
}
```

| Field | Description |
|-------|-------------|
| isFollowing | Whether user follows this anchor |
| notifyOnUpdate | Whether notifications are enabled |
| hasUpdates | True if currentVersion > lastSeenVersion |
| updatesSinceLastSeen | currentVersion - lastSeenVersion |
| lastSeenVersion | Version when user last viewed anchor |
| currentVersion | Current anchor version |
| followedAt | When user started following |

**Not Following Response:**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "isFollowing": false,
        "notifyOnUpdate": false,
        "hasUpdates": false,
        "updatesSinceLastSeen": 0,
        "lastSeenVersion": 0,
        "currentVersion": 8,
        "followedAt": null
    }
}
```

---

### 3. Toggle Update Notifications

**Endpoint:** `PATCH /anchors/:id/follow/notifications`

**Authentication:** Required

**Request Body:**
```json
{
    "notifyOnUpdate": true
}
```

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "notifyOnUpdate": true
    }
}
```

**Error Responses:**

| Status | Code | Message |
|--------|------|---------|
| 400 | NOT_FOLLOWING | You are not following this anchor |
| 404 | ANCHOR_NOT_FOUND | Anchor not found |

---

### 4. List Following Anchors

**Endpoint:** `GET /users/me/following-anchors`

**Authentication:** Required

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| page | int | 1 | Page number |
| limit | int | 20 | Items per page (max 50) |
| hasUpdates | bool | - | Filter: only anchors with updates |
| sort | string | recent | Sort: recent, updated, alphabetical |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "anchors": [
            {
                "id": "65a9b2c3d4e5f67890abcdef",
                "title": "Design Resources 2024",
                "description": "Curated collection of design tools and resources",
                "visibility": "public",
                "itemCount": 25,
                "likeCount": 150,
                "commentCount": 12,
                "followerCount": 89,
                "tags": ["design", "tools", "resources"],
                "hasUpdates": true,
                "updatesSinceLastSeen": 3,
                "currentVersion": 8,
                "lastSeenVersion": 5,
                "lastUpdatedAt": "2024-01-15T10:00:00Z",
                "notifyOnUpdate": true,
                "followedAt": "2024-01-01T00:00:00Z",
                "author": {
                    "id": "65a9b2c3d4e5f67890abc123",
                    "username": "designguru",
                    "displayName": "Design Guru",
                    "profilePicture": "https://...",
                    "isVerified": true
                }
            }
        ],
        "pagination": {
            "page": 1,
            "limit": 20,
            "total": 12,
            "totalPages": 1,
            "hasMore": false
        },
        "meta": {
            "totalWithUpdates": 3,
            "sort": "recent"
        }
    }
}
```

---

### 5. Get Suggested Interests/Categories

**Endpoint:** `GET /interests/suggested`

**Authentication:** Optional (personalized if authenticated)

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| limit | int | 10 | Max categories to return |

**Success Response (200 OK) - Authenticated:**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "categories": [
            {
                "name": "design",
                "displayName": "Design",
                "anchorCount": 1250,
                "relevanceScore": 0.95
            },
            {
                "name": "technology",
                "displayName": "Technology",
                "anchorCount": 3420,
                "relevanceScore": 0.82
            },
            {
                "name": "books",
                "displayName": "Books",
                "anchorCount": 890,
                "relevanceScore": 0.75
            },
            {
                "name": "music",
                "displayName": "Music",
                "anchorCount": 2100,
                "relevanceScore": 0.68
            },
            {
                "name": "programming",
                "displayName": "Programming",
                "anchorCount": 4500,
                "relevanceScore": 0.62
            }
        ],
        "personalized": true,
        "basedOn": {
            "ownAnchorTags": ["design", "ui", "figma"],
            "likedAnchorTags": ["tech", "tools"],
            "followedAnchorTags": ["resources", "tutorials"]
        }
    }
}
```

**Success Response (200 OK) - Not Authenticated (Popular Categories):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "categories": [
            {
                "name": "technology",
                "displayName": "Technology",
                "anchorCount": 3420,
                "relevanceScore": 1.0
            },
            {
                "name": "design",
                "displayName": "Design",
                "anchorCount": 1250,
                "relevanceScore": 0.95
            }
        ],
        "personalized": false,
        "basedOn": null
    }
}
```

**Interest Calculation Algorithm:**
1. Collect tags from user's own anchors (weight: 3)
2. Collect tags from user's liked anchors (weight: 2)
3. Collect tags from user's followed anchors (weight: 2)
4. Collect tags from followed users' anchors (weight: 1)
5. Normalize tag frequencies
6. Calculate relevance score (0-1)
7. Return top N categories sorted by relevance

---

### 6. Updated Home Feed

**Endpoint:** `GET /feed/home`

**Authentication:** Required

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| limit | int | 20 | Items per section |
| cursor | string | - | Pagination cursor for followingUsers |

**Updated Response Structure:**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "followingAnchors": {
            "items": [
                {
                    "id": "65a9b2c3d4e5f67890abcdef",
                    "title": "Design Resources 2024",
                    "description": "Curated collection...",
                    "itemCount": 25,
                    "hasUpdates": true,
                    "updatesSinceLastSeen": 3,
                    "lastUpdatedAt": "2024-01-15T10:00:00Z",
                    "author": {
                        "id": "...",
                        "username": "designguru",
                        "displayName": "Design Guru",
                        "profilePicture": "..."
                    }
                }
            ],
            "totalWithUpdates": 3,
            "total": 15,
            "hasMore": true
        },
        "followingUsers": {
            "items": [
                {
                    "id": "...",
                    "title": "New React Patterns",
                    "description": "...",
                    "author": {...},
                    "createdAt": "...",
                    "engagement": {
                        "hasLiked": false,
                        "hasCloned": false
                    }
                }
            ],
            "hasMore": true,
            "nextCursor": "eyJjIjoiMjAyNC..."
        },
        "suggestedCategories": [
            {"name": "design", "displayName": "Design", "anchorCount": 1250},
            {"name": "tech", "displayName": "Technology", "anchorCount": 3420},
            {"name": "books", "displayName": "Books", "anchorCount": 890}
        ]
    }
}
```

---

## Version Tracking System

### When Version Increments

| Action | Version Change | LastItemAddedAt |
|--------|----------------|-----------------|
| Add item | +1 | Updated |
| Remove item | No change | No change |
| Edit anchor details | No change | No change |
| Reorder items | No change | No change |

### When LastSeenVersion Updates

When a user who follows an anchor views that anchor (GET /anchors/:id), their `lastSeenVersion` is updated to the anchor's current `version`.

---

## Notification Trigger

### When Anchor is Updated (Item Added)

```go
// After adding item to anchor:
// 1. Increment version
// 2. Update lastItemAddedAt
// 3. Notify followers with notifyOnUpdate=true

notification := Notification{
    Type:         TypeAnchorUpdate,
    ResourceType: "anchor",
    ResourceID:   anchorID,
    AnchorID:     &anchorID,
    Preview:      "New item added to: " + anchorTitle,
}
```

**Notification Display:** "@designguru added new content to 'Design Resources 2024'"

---

## Business Rules Summary

### Follow Rules

| # | Rule |
|---|------|
| 1 | Cannot follow own anchors |
| 2 | Cannot follow private anchors |
| 3 | Can follow public and unlisted anchors |
| 4 | Follow is idempotent |
| 5 | Unfollow is hard delete |
| 6 | Following increments anchor's followerCount |
| 7 | Unfollowing decrements anchor's followerCount |

### Version Rules

| # | Rule |
|---|------|
| 8 | Version starts at 0 for new anchors |
| 9 | Version increments only when items are added |
| 10 | lastSeenVersion updates when follower views anchor |
| 11 | hasUpdates = version > lastSeenVersion |

### Notification Rules

| # | Rule |
|---|------|
| 12 | Only notify followers with notifyOnUpdate=true |
| 13 | Don't notify anchor owner |
| 14 | Notifications are created async |

### Interest Rules

| # | Rule |
|---|------|
| 15 | Authenticated users get personalized categories |
| 16 | Unauthenticated users get popular categories |
| 17 | Categories are derived from tags dynamically |
| 18 | Display name = capitalized tag name |

---

## File Structure

```
internal/features/
├── anchor_follows/              # NEW MODULE
│   ├── model.go                # AnchorFollow, DTOs
│   ├── repository.go           # CRUD + queries + indexes
│   ├── handler.go              # 4 handlers
│   ├── validator.go            # Validation functions
│   └── routes.go               # Route registration
│
├── interests/                   # NEW MODULE
│   ├── model.go                # Category DTOs
│   ├── repository.go           # Tag aggregation queries
│   ├── service.go              # Interest calculation
│   ├── handler.go              # 1 handler
│   └── routes.go               # Route registration
│
├── anchors/
│   ├── model.go                # UPDATE: Add Version, LastItemAddedAt, FollowerCount
│   ├── repository.go           # UPDATE: Add IncrementVersion, UpdateLastItemAddedAt
│   └── handler.go              # UPDATE: GetAnchor updates lastSeenVersion, AddItem increments version
│
├── notifications/
│   ├── model.go                # UPDATE: Add TypeAnchorUpdate constant
│   └── service.go              # UPDATE: Add CreateAnchorUpdateNotifications
│
└── feed/
    ├── handler.go              # UPDATE: Add followingAnchors section
    └── repository.go           # UPDATE: Add GetFollowingAnchorsForFeed
```

---

## Changes to Existing Files

### anchors/model.go
- Add `Version int` field
- Add `LastItemAddedAt *time.Time` field
- Add `FollowerCount int` field

### anchors/repository.go
- Add `IncrementVersion(ctx, anchorID)` method
- Add `IncrementFollowerCount(ctx, anchorID, delta)` method

### anchors/handler.go
- GetAnchor: Update lastSeenVersion for followers
- AddItem: Increment version, trigger notifications

### notifications/model.go
- Add `TypeAnchorUpdate = "anchor_update"` constant

### notifications/service.go
- Add `CreateAnchorUpdateNotifications(ctx, anchorID, title, authorID)` method

### feed/handler.go
- Add followingAnchors section to home feed response
- Add suggestedCategories to response

---

## Testing Scenarios

### Anchor Follow
- [ ] Follow public anchor - success
- [ ] Follow unlisted anchor - success
- [ ] Follow private anchor - ACCESS_DENIED
- [ ] Follow own anchor - CANNOT_FOLLOW_OWN
- [ ] Follow twice (idempotent) - success, no duplicate
- [ ] Unfollow - success, record deleted
- [ ] Unfollow when not following - success (no error)

### Version Tracking
- [ ] New anchor has version 0
- [ ] Adding item increments version
- [ ] Removing item does NOT increment version
- [ ] Editing anchor does NOT increment version
- [ ] Viewing anchor updates lastSeenVersion

### Notifications
- [ ] Add item notifies followers with notifyOnUpdate=true
- [ ] Add item does NOT notify followers with notifyOnUpdate=false
- [ ] Author is NOT notified of own updates

### Home Feed
- [ ] Following anchors section appears
- [ ] hasUpdates is true when version > lastSeenVersion
- [ ] updatesSinceLastSeen is correct count
- [ ] Suggested categories appear

### Interests
- [ ] Authenticated user gets personalized categories
- [ ] Unauthenticated user gets popular categories
- [ ] Categories sorted by relevance/popularity