# Search Feature Specification

## Overview

The Search feature enables users to discover anchors, users, and tags through text-based search. It uses MongoDB's built-in text search capabilities for efficient full-text matching with relevance scoring.

---

## API Summary

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 1 | `GET` | `/search` | Optional | Unified search (anchors + users) |
| 2 | `GET` | `/search/anchors` | Optional | Search anchors with filters |
| 3 | `GET` | `/search/users` | Optional | Search users |
| 4 | `GET` | `/search/tags` | Optional | Tag autocomplete |

**Total: 4 endpoints**

---

## MongoDB Text Indexes

### Anchors Collection - Text Index

```go
// Add to anchors repository NewRepository
{
    Keys: bson.D{
        {Key: "title", Value: "text"},
        {Key: "description", Value: "text"},
        {Key: "tags", Value: "text"},
    },
    Options: options.Index().
        SetWeights(bson.D{
            {Key: "title", Value: 10},
            {Key: "tags", Value: 5},
            {Key: "description", Value: 1},
        }).
        SetName("anchor_text_search"),
}
```

### Users Collection - Text Index

```go
// Add to auth repository NewRepository
{
    Keys: bson.D{
        {Key: "username", Value: "text"},
        {Key: "displayName", Value: "text"},
        {Key: "bio", Value: "text"},
    },
    Options: options.Index().
        SetWeights(bson.D{
            {Key: "username", Value: 10},
            {Key: "displayName", Value: 5},
            {Key: "bio", Value: 1},
        }).
        SetName("user_text_search"),
}
```

---

## API Endpoints

### 1. Unified Search

**Endpoint:** `GET /search`

**Authentication:** Optional (needed for isFollowing)

**Description:** Search across anchors and users in a single request. Ideal for search-as-you-type UI.

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| q | string | Yes | - | Search query (2-100 chars) |
| type | string | No | all | Filter: all, anchors, users |
| limit | int | No | 10 | Results per type (1-20) |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "query": "design",
        "anchors": {
            "items": [
                {
                    "id": "507f1f77bcf86cd799439011",
                    "title": "UI Design Resources 2024",
                    "description": "Curated collection of design tools and resources...",
                    "visibility": "public",
                    "itemCount": 15,
                    "likeCount": 42,
                    "commentCount": 8,
                    "tags": ["design", "ui", "resources"],
                    "createdAt": "2024-01-15T10:30:00Z",
                    "author": {
                        "id": "507f1f77bcf86cd799439012",
                        "username": "johndoe",
                        "displayName": "John Doe",
                        "profilePicture": "https://..."
                    }
                }
            ],
            "total": 25,
            "hasMore": true
        },
        "users": {
            "items": [
                {
                    "id": "507f1f77bcf86cd799439013",
                    "username": "designer_jane",
                    "displayName": "Jane Designer",
                    "bio": "UI/UX Designer passionate about...",
                    "profilePicture": "https://...",
                    "followerCount": 1250,
                    "isVerified": true,
                    "isFollowing": false
                }
            ],
            "total": 8,
            "hasMore": false
        }
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_QUERY | Query must be 2-100 characters |
| 400 | INVALID_TYPE | Type must be: all, anchors, or users |

---

### 2. Search Anchors

**Endpoint:** `GET /search/anchors`

**Authentication:** Optional (for future personalization)

**Description:** Search anchors with pagination, tag filtering, and sorting options.

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| q | string | Yes | - | Search query (2-100 chars) |
| tag | string | No | - | Filter by specific tag |
| sort | string | No | relevant | Sort: relevant, recent, popular |
| page | int | No | 1 | Page number |
| limit | int | No | 20 | Items per page (1-50) |

**Sort Options:**
| Value | Sort By | Description |
|-------|---------|-------------|
| relevant | Text score | MongoDB text search relevance (default) |
| recent | createdAt DESC | Most recently created |
| popular | engagementScore DESC | Highest engagement |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "anchors": [
            {
                "id": "507f1f77bcf86cd799439011",
                "title": "UI Design Resources 2024",
                "description": "Curated collection of design tools...",
                "visibility": "public",
                "itemCount": 15,
                "likeCount": 42,
                "commentCount": 8,
                "cloneCount": 5,
                "engagementScore": 118,
                "tags": ["design", "ui", "resources"],
                "createdAt": "2024-01-15T10:30:00Z",
                "author": {
                    "id": "507f1f77bcf86cd799439012",
                    "username": "johndoe",
                    "displayName": "John Doe",
                    "profilePicture": "https://...",
                    "isVerified": false
                }
            }
        ],
        "pagination": {
            "page": 1,
            "limit": 20,
            "total": 45,
            "totalPages": 3,
            "hasMore": true
        },
        "meta": {
            "query": "design",
            "tag": null,
            "sort": "relevant"
        }
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_QUERY | Query must be 2-100 characters |
| 400 | INVALID_SORT | Sort must be: relevant, recent, or popular |

---

### 3. Search Users

**Endpoint:** `GET /search/users`

**Authentication:** Optional (needed for isFollowing)

**Description:** Search users by username, display name, or bio.

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| q | string | Yes | - | Search query (2-100 chars) |
| page | int | No | 1 | Page number |
| limit | int | No | 20 | Items per page (1-50) |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "users": [
            {
                "id": "507f1f77bcf86cd799439013",
                "username": "designer_jane",
                "displayName": "Jane Designer",
                "bio": "UI/UX Designer passionate about creating...",
                "profilePicture": "https://...",
                "followerCount": 1250,
                "anchorCount": 15,
                "isVerified": true,
                "isFollowing": false
            }
        ],
        "pagination": {
            "page": 1,
            "limit": 20,
            "total": 15,
            "totalPages": 1,
            "hasMore": false
        },
        "meta": {
            "query": "jane"
        }
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_QUERY | Query must be 2-100 characters |

---

### 4. Tag Autocomplete

**Endpoint:** `GET /search/tags`

**Authentication:** Optional

**Description:** Get tag suggestions based on prefix. Used for tag autocomplete in UI.

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| q | string | Yes | - | Tag prefix (1-50 chars) |
| limit | int | No | 10 | Max suggestions (1-20) |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "tags": [
            { "name": "design", "count": 156 },
            { "name": "designer", "count": 42 },
            { "name": "design-system", "count": 28 },
            { "name": "design-patterns", "count": 15 }
        ],
        "query": "des"
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_QUERY | Query must be 1-50 characters |

**Implementation Note:** Tag autocomplete uses MongoDB aggregation to:
1. Filter anchors where tags start with query prefix
2. Unwind tags array
3. Filter unwound tags by prefix
4. Group and count
5. Sort by count DESC

---

## Data Models

### Request DTOs

```go
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
```

### Response DTOs

```go
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
```

---

## Business Rules

### General Rules
| # | Rule |
|---|------|
| 1 | Minimum query length: 2 characters (1 for tags) |
| 2 | Maximum query length: 100 characters (50 for tags) |
| 3 | Search is case-insensitive |
| 4 | Empty results return empty array, not error |

### Anchor Search Rules
| # | Rule |
|---|------|
| 5 | Only search PUBLIC anchors (not unlisted, not private) |
| 6 | Exclude deleted anchors (deletedAt != nil) |
| 7 | Tag filter is case-insensitive exact match |
| 8 | Relevant sort uses MongoDB text score |
| 9 | Results include author info |

### User Search Rules
| # | Rule |
|---|------|
| 10 | Search all users (no visibility restriction) |
| 11 | isFollowing is false if not authenticated |
| 12 | isFollowing is false for self |
| 13 | Results sorted by text relevance |

### Tag Autocomplete Rules
| # | Rule |
|---|------|
| 14 | Prefix match (tag starts with query) |
| 15 | Only count tags from public, non-deleted anchors |
| 16 | Sort by count DESC (most used first) |
| 17 | Tag names are lowercase |

---

## Repository Methods

### Search Repository

```go
type Repository struct {
    anchorsCollection *mongo.Collection
    usersCollection   *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository

// SearchAnchors performs text search on anchors
func (r *Repository) SearchAnchors(ctx context.Context, query string, tag *string, sort string, page, limit int) ([]Anchor, int64, error)

// SearchUsers performs text search on users
func (r *Repository) SearchUsers(ctx context.Context, query string, page, limit int) ([]User, int64, error)

// SearchTags returns tags matching prefix with usage counts
func (r *Repository) SearchTags(ctx context.Context, prefix string, limit int) ([]TagResult, error)
```

### MongoDB Text Search Query

```go
// Anchor search filter
filter := bson.M{
    "$text":      bson.M{"$search": query},
    "visibility": VisibilityPublic,
    "deletedAt":  nil,
}

// Add tag filter if provided
if tag != nil {
    filter["tags"] = bson.M{"$regex": primitive.Regex{Pattern: "^" + *tag + "$", Options: "i"}}
}

// Projection with text score
projection := bson.M{
    "score": bson.M{"$meta": "textScore"},
}

// Sort options
var sortOrder bson.D
switch sort {
case SortRecent:
    sortOrder = bson.D{{Key: "createdAt", Value: -1}}
case SortPopular:
    sortOrder = bson.D{{Key: "engagementScore", Value: -1}}
default: // relevant
    sortOrder = bson.D{{Key: "score", Value: bson.M{"$meta": "textScore"}}}
}
```

### Tag Aggregation Pipeline

```go
pipeline := mongo.Pipeline{
    // Match public, non-deleted anchors with matching tag prefix
    {{Key: "$match", Value: bson.M{
        "visibility": VisibilityPublic,
        "deletedAt":  nil,
        "tags": bson.M{"$regex": primitive.Regex{
            Pattern: "^" + strings.ToLower(prefix),
            Options: "i",
        }},
    }}},
    // Unwind tags array
    {{Key: "$unwind", Value: "$tags"}},
    // Filter tags by prefix
    {{Key: "$match", Value: bson.M{
        "tags": bson.M{"$regex": primitive.Regex{
            Pattern: "^" + strings.ToLower(prefix),
            Options: "i",
        }},
    }}},
    // Group by tag and count
    {{Key: "$group", Value: bson.M{
        "_id":   bson.M{"$toLower": "$tags"},
        "count": bson.M{"$sum": 1},
    }}},
    // Sort by count DESC
    {{Key: "$sort", Value: bson.M{"count": -1}}},
    // Limit results
    {{Key: "$limit", Value: limit}},
    // Project final shape
    {{Key: "$project", Value: bson.M{
        "_id":   0,
        "name":  "$_id",
        "count": 1,
    }}},
}
```

---

## File Structure

```
internal/features/search/
├── model.go       # All DTOs and constants
├── repository.go  # Search queries
├── handler.go     # 4 HTTP handlers
├── validator.go   # Validation functions
└── routes.go      # Route registration
```

---

## Changes to Existing Code

### 1. anchors/repository.go - Add Text Index

In `NewRepository`, add the text index:

```go
// Text index for search
{
    Keys: bson.D{
        {Key: "title", Value: "text"},
        {Key: "description", Value: "text"},
        {Key: "tags", Value: "text"},
    },
    Options: options.Index().
        SetWeights(bson.D{
            {Key: "title", Value: 10},
            {Key: "tags", Value: 5},
            {Key: "description", Value: 1},
        }).
        SetName("anchor_text_search"),
}
```

### 2. auth/repository.go - Add Text Index

In `NewRepository`, add the text index:

```go
// Text index for search
{
    Keys: bson.D{
        {Key: "username", Value: "text"},
        {Key: "displayName", Value: "text"},
        {Key: "bio", Value: "text"},
    },
    Options: options.Index().
        SetWeights(bson.D{
            {Key: "username", Value: 10},
            {Key: "displayName", Value: 5},
            {Key: "bio", Value: 1},
        }).
        SetName("user_text_search"),
}
```

---

## Implementation Order

### Step 1: Add Text Indexes
- Update anchors/repository.go with text index
- Update auth/repository.go with text index

### Step 2: Create search/model.go
- All query DTOs
- All response DTOs
- Constants

### Step 3: Create search/repository.go
- NewRepository
- SearchAnchors method
- SearchUsers method
- SearchTags method

### Step 4: Create search/validator.go
- ValidateUnifiedSearchQuery
- ValidateAnchorSearchQuery
- ValidateUserSearchQuery
- ValidateTagSearchQuery

### Step 5: Create search/handler.go
- UnifiedSearch handler
- SearchAnchors handler
- SearchUsers handler
- SearchTags handler
- Enrichment helpers

### Step 6: Create search/routes.go
- RegisterRoutes function

### Step 7: Update routes/routes.go
- Register search routes

---

## Testing Scenarios

### Unified Search
- [ ] Search returns both anchors and users
- [ ] Type=anchors returns only anchors
- [ ] Type=users returns only users
- [ ] Empty query returns 400
- [ ] Query too short returns 400
- [ ] No results returns empty arrays

### Anchor Search
- [ ] Text search matches title
- [ ] Text search matches description
- [ ] Text search matches tags
- [ ] Tag filter works
- [ ] Sort by relevant (default)
- [ ] Sort by recent
- [ ] Sort by popular
- [ ] Pagination works
- [ ] Only public anchors returned
- [ ] Deleted anchors excluded
- [ ] Author info included

### User Search
- [ ] Text search matches username
- [ ] Text search matches displayName
- [ ] Text search matches bio
- [ ] Pagination works
- [ ] isFollowing correct when authenticated
- [ ] isFollowing false when not authenticated

### Tag Autocomplete
- [ ] Prefix matching works
- [ ] Case-insensitive
- [ ] Sorted by count DESC
- [ ] Only tags from public anchors
- [ ] Limit respected

---

## Performance Considerations

### Indexes
- Text indexes enable efficient full-text search
- Compound indexes for filtered queries
- Weights prioritize important fields

### Pagination
- Use skip/limit for simplicity
- For high-volume, consider cursor-based pagination

### Caching (Future)
- Cache popular tag counts
- Cache trending searches
- Cache autocomplete results

---

## Future Enhancements

### Search Suggestions
- Recent searches (per user)
- Popular searches
- Trending topics

### Advanced Filters
- Date range
- Minimum likes
- Verified users only

### Elasticsearch Migration
- Better relevance ranking
- Fuzzy matching
- Synonyms support
- Faceted search