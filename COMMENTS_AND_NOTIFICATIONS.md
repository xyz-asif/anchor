# Comments & Notifications System Specification

## Overview

This specification covers the Comments system with @mention support and the Notifications system. Users can comment on anchors, mention other users with @username, and receive notifications for mentions and comments on their anchors.

---

## API Summary

### Comments Endpoints (7)

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 1 | `POST` | `/anchors/:id/comments` | Required | Add comment (with mention detection) |
| 2 | `GET` | `/anchors/:id/comments` | Optional | List comments for anchor |
| 3 | `GET` | `/comments/:id` | Optional | Get single comment |
| 4 | `PATCH` | `/comments/:id` | Required | Edit comment |
| 5 | `DELETE` | `/comments/:id` | Required | Delete comment |
| 6 | `POST` | `/comments/:id/like` | Required | Like/unlike comment |
| 7 | `GET` | `/comments/:id/like/status` | Required | Check like status |

### Notifications Endpoints (4)

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 8 | `GET` | `/notifications` | Required | List user's notifications |
| 9 | `GET` | `/notifications/unread-count` | Required | Get unread notification count |
| 10 | `PATCH` | `/notifications/:id/read` | Required | Mark notification as read |
| 11 | `PATCH` | `/notifications/read-all` | Required | Mark all notifications as read |

**Total: 11 endpoints**

---

# PART 1: COMMENTS SYSTEM

---

## Data Models

### Comment Collection

```go
type Comment struct {
    ID        primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
    AnchorID  primitive.ObjectID   `bson:"anchorId" json:"anchorId"`
    UserID    primitive.ObjectID   `bson:"userId" json:"userId"`
    Content   string               `bson:"content" json:"content"`
    Mentions  []primitive.ObjectID `bson:"mentions" json:"mentions"` // Mentioned user IDs
    LikeCount int                  `bson:"likeCount" json:"likeCount"`
    IsEdited  bool                 `bson:"isEdited" json:"isEdited"`
    CreatedAt time.Time            `bson:"createdAt" json:"createdAt"`
    UpdatedAt time.Time            `bson:"updatedAt" json:"updatedAt"`
    DeletedAt *time.Time           `bson:"deletedAt,omitempty" json:"-"`
}
```

### CommentLike Collection

```go
type CommentLike struct {
    ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    CommentID primitive.ObjectID `bson:"commentId" json:"commentId"`
    UserID    primitive.ObjectID `bson:"userId" json:"userId"`
    CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}
```

### Comment Indexes

```go
// Comments Collection
{ anchorId: 1, deletedAt: 1, createdAt: -1 }           // List by anchor (newest)
{ anchorId: 1, deletedAt: 1, likeCount: -1, createdAt: -1 }  // List by anchor (top)
{ userId: 1, createdAt: -1 }                            // User's comments

// CommentLikes Collection
{ commentId: 1, userId: 1 } - unique                    // Prevent duplicate likes
{ commentId: 1, createdAt: -1 }                         // Likes for a comment
{ userId: 1, commentId: 1 }                             // Batch check user likes
```

---

## Mention System

### Mention Format

- Pattern: `@username` (alphanumeric + underscore, 3-30 chars)
- Regex: `@([a-zA-Z0-9_]{3,30})`
- Case-insensitive matching

### Mention Extraction

```go
// ExtractMentions extracts @usernames from content
func ExtractMentions(content string) []string {
    re := regexp.MustCompile(`@([a-zA-Z0-9_]{3,30})`)
    matches := re.FindAllStringSubmatch(content, -1)
    
    // Deduplicate
    seen := make(map[string]bool)
    var usernames []string
    for _, match := range matches {
        username := strings.ToLower(match[1])
        if !seen[username] {
            seen[username] = true
            usernames = append(usernames, username)
        }
    }
    
    return usernames // ["johndoe", "jane_smith"]
}
```

### Mention Validation

```go
// ValidateMentions validates mentioned users exist and returns their IDs
func (r *Repository) GetUserIDsByUsernames(ctx context.Context, usernames []string) (map[string]primitive.ObjectID, error) {
    // Query users collection for matching usernames
    // Returns map: {"johndoe": ObjectID, "jane_smith": ObjectID}
}
```

### Mention Rules

| # | Rule |
|---|------|
| 1 | Max 10 mentions per comment |
| 2 | Self-mentions are allowed but don't trigger notification |
| 3 | Invalid/non-existent usernames are ignored (no error) |
| 4 | Mentions are case-insensitive |
| 5 | Duplicate mentions in same comment count as one |

---

## Comments API Endpoints

### 1. Add Comment

**Endpoint:** `POST /anchors/:id/comments`

**Authentication:** Required

**Description:** Add a comment with automatic mention detection.

**Request Body:**
```json
{
    "content": "Great collection @johndoe! Also check @jane_smith's work."
}
```

**Success Response (201 Created):**
```json
{
    "success": true,
    "statusCode": 201,
    "message": "success",
    "data": {
        "id": "507f1f77bcf86cd799439011",
        "anchorId": "507f1f77bcf86cd799439012",
        "content": "Great collection @johndoe! Also check @jane_smith's work.",
        "mentions": ["507f1f77bcf86cd799439020", "507f1f77bcf86cd799439021"],
        "likeCount": 0,
        "isEdited": false,
        "createdAt": "2024-01-15T10:30:00Z",
        "updatedAt": "2024-01-15T10:30:00Z",
        "author": {
            "id": "507f1f77bcf86cd799439013",
            "username": "alice",
            "displayName": "Alice Chen",
            "profilePicture": "https://...",
            "isVerified": false
        },
        "engagement": {
            "hasLiked": false
        }
    }
}
```

**Business Logic:**
```
1. Validate anchor exists and is accessible
2. Validate content (1-1000 chars)
3. Extract mentions from content
4. Validate mentioned users (get IDs for valid usernames)
5. Create comment with mentions array
6. Increment anchor's commentCount
7. Update anchor's engagementScore (async)
8. Create notifications (async):
   a. For each mentioned user (except self): type="mention"
   b. For anchor owner (if not self): type="comment"
9. Return comment with author info
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid anchor ID |
| 400 | VALIDATION_FAILED | Content required / too long |
| 401 | UNAUTHORIZED | Authentication required |
| 403 | ACCESS_DENIED | Cannot comment on private anchor |
| 404 | ANCHOR_NOT_FOUND | Anchor not found |

---

### 2. List Comments

**Endpoint:** `GET /anchors/:id/comments`

**Authentication:** Optional

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| page | int | 1 | Page number |
| limit | int | 20 | Items per page (max 50) |
| sort | string | newest | Sort: newest, oldest, top |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "comments": [
            {
                "id": "507f1f77bcf86cd799439011",
                "anchorId": "507f1f77bcf86cd799439012",
                "content": "Great collection @johndoe!",
                "mentions": ["507f1f77bcf86cd799439020"],
                "likeCount": 15,
                "isEdited": false,
                "createdAt": "2024-01-15T10:30:00Z",
                "updatedAt": "2024-01-15T10:30:00Z",
                "author": {
                    "id": "507f1f77bcf86cd799439013",
                    "username": "alice",
                    "displayName": "Alice Chen",
                    "profilePicture": "https://...",
                    "isVerified": false
                },
                "engagement": {
                    "hasLiked": true
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
            "sort": "newest",
            "anchorId": "507f1f77bcf86cd799439012"
        }
    }
}
```

---

### 3. Get Single Comment

**Endpoint:** `GET /comments/:id`

**Authentication:** Optional

**Success Response (200 OK):**
Same structure as single comment in list.

---

### 4. Edit Comment

**Endpoint:** `PATCH /comments/:id`

**Authentication:** Required

**Request Body:**
```json
{
    "content": "Updated comment @newmention"
}
```

**Business Logic:**
```
1. Verify comment exists and not deleted
2. Verify user is comment author
3. Validate new content
4. Extract NEW mentions from updated content
5. Find newly added mentions (not in original)
6. Update comment: content, mentions, isEdited=true, updatedAt
7. Create notifications for NEW mentions only (async)
8. Return updated comment
```

**Note:** Only NEW mentions trigger notifications. If @johndoe was in original and still in edited version, no new notification.

---

### 5. Delete Comment

**Endpoint:** `DELETE /comments/:id`

**Authentication:** Required

**Business Logic:**
```
1. Verify comment exists and not deleted
2. Check permission:
   - Comment author can delete their comment
   - Anchor owner can delete any comment (moderation)
3. Soft delete comment
4. Decrement anchor's commentCount
5. Update anchor's engagementScore (async)
6. Return success
```

---

### 6. Like/Unlike Comment

**Endpoint:** `POST /comments/:id/like`

**Authentication:** Required

**Request Body:**
```json
{
    "action": "like"
}
```

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "hasLiked": true,
        "likeCount": 16
    }
}
```

---

### 7. Get Comment Like Status

**Endpoint:** `GET /comments/:id/like/status`

**Authentication:** Required

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "hasLiked": true,
        "likeCount": 16
    }
}
```

---

# PART 2: NOTIFICATIONS SYSTEM

---

## Data Model

### Notification Collection

```go
type Notification struct {
    ID           primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
    RecipientID  primitive.ObjectID  `bson:"recipientId" json:"recipientId"`   // Who receives
    ActorID      primitive.ObjectID  `bson:"actorId" json:"actorId"`           // Who triggered
    Type         string              `bson:"type" json:"type"`                 // mention, comment
    ResourceType string              `bson:"resourceType" json:"resourceType"` // comment, anchor
    ResourceID   primitive.ObjectID  `bson:"resourceId" json:"resourceId"`     // Comment or Anchor ID
    AnchorID     *primitive.ObjectID `bson:"anchorId,omitempty" json:"anchorId,omitempty"`
    Preview      string              `bson:"preview" json:"preview"`           // Truncated content
    IsRead       bool                `bson:"isRead" json:"isRead"`
    CreatedAt    time.Time           `bson:"createdAt" json:"createdAt"`
}
```

### Notification Types

| Type | Trigger | Message Format |
|------|---------|----------------|
| `mention` | @username in comment | "@alice mentioned you: 'Great work...'" |
| `comment` | Comment on your anchor | "@alice commented on your anchor 'Design Resources'" |
| `anchor_update` | Item added to followed anchor | "'Photography' has a new update: 'My new lens'" |

### Notification Indexes

```go
// Query user's notifications (unread first, then by date)
{ recipientId: 1, isRead: 1, createdAt: -1 }

// Count unread
{ recipientId: 1, isRead: 1 }

// Cleanup old notifications
{ createdAt: 1 }
```

---

## Notification Creation Logic

### When Comment is Added:

```go
func CreateCommentNotifications(ctx context.Context, comment *Comment, anchor *Anchor, actor *User) {
    var notifications []Notification
    
    // 1. Mention notifications
    for _, mentionedUserID := range comment.Mentions {
        // Skip if mentioning self
        if mentionedUserID == actor.ID {
            continue
        }
        
        notifications = append(notifications, Notification{
            RecipientID:  mentionedUserID,
            ActorID:      actor.ID,
            Type:         NotificationTypeMention,
            ResourceType: "comment",
            ResourceID:   comment.ID,
            AnchorID:     &comment.AnchorID,
            Preview:      truncate(comment.Content, 100),
            IsRead:       false,
            CreatedAt:    time.Now(),
        })
    }
    
    // 2. Comment notification to anchor owner
    // Skip if commenter is anchor owner
    // Skip if anchor owner was already mentioned (avoid duplicate)
    if anchor.UserID != actor.ID && !contains(comment.Mentions, anchor.UserID) {
        notifications = append(notifications, Notification{
            RecipientID:  anchor.UserID,
            ActorID:      actor.ID,
            Type:         NotificationTypeComment,
            ResourceType: "anchor",
            ResourceID:   anchor.ID,
            AnchorID:     &anchor.ID,
            Preview:      truncate(comment.Content, 100),
            IsRead:       false,
            CreatedAt:    time.Now(),
        })
    }
    
    // Batch insert
    if len(notifications) > 0 {
        notificationRepo.CreateMany(ctx, notifications)
    }
}

### When Item is Added to Anchor (Anchor Update):

```go
func CreateAnchorUpdateNotifications(ctx context.Context, anchorID primitive.ObjectID, itemTitle string, actorID primitive.ObjectID) {
    // 1. Get all followers who have notifications enabled
    followers = followsRepo.GetNotificationEnabledFollowers(ctx, anchorID)
    
    var notifications []Notification
    for _, f := range followers {
        // Skip actor
        if f.UserID == actorID {
            continue
        }
        
        notifications = append(notifications, Notification{
            RecipientID:  f.UserID,
            ActorID:      actorID,
            Type:         NotificationTypeAnchorUpdate,
            ResourceType: "anchor",
            ResourceID:   anchorID,
            AnchorID:     &anchorID,
            Preview:      fmt.Sprintf("'%s' has a new update: '%s'", anchor.Title, itemTitle),
            IsRead:       false,
            CreatedAt:    time.Now(),
        })
    }
    
    // Batch insert
    if len(notifications) > 0 {
        notificationRepo.CreateMany(ctx, notifications)
    }
}
```
```

### Preview Truncation:

```go
func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen-3] + "..."
}
```

---

## Notifications API Endpoints

### 8. List Notifications

**Endpoint:** `GET /notifications`

**Authentication:** Required

**Query Parameters:**
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| page | int | 1 | Page number |
| limit | int | 20 | Items per page (max 50) |
| unreadOnly | bool | false | Only show unread |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "notifications": [
            {
                "id": "507f1f77bcf86cd799439030",
                "type": "mention",
                "resourceType": "comment",
                "resourceId": "507f1f77bcf86cd799439011",
                "anchorId": "507f1f77bcf86cd799439012",
                "preview": "Great work on this collection! I especially loved...",
                "isRead": false,
                "createdAt": "2024-01-15T10:30:00Z",
                "actor": {
                    "id": "507f1f77bcf86cd799439013",
                    "username": "alice",
                    "displayName": "Alice Chen",
                    "profilePicture": "https://..."
                },
                "anchor": {
                    "id": "507f1f77bcf86cd799439012",
                    "title": "Design Resources 2024"
                }
            },
            {
                "id": "507f1f77bcf86cd799439031",
                "type": "comment",
                "resourceType": "anchor",
                "resourceId": "507f1f77bcf86cd799439012",
                "anchorId": "507f1f77bcf86cd799439012",
                "preview": "This is exactly what I needed!",
                "isRead": true,
                "createdAt": "2024-01-15T09:00:00Z",
                "actor": {
                    "id": "507f1f77bcf86cd799439014",
                    "username": "bob",
                    "displayName": "Bob Smith",
                    "profilePicture": "https://..."
                },
                "anchor": {
                    "id": "507f1f77bcf86cd799439012",
                    "title": "Design Resources 2024"
                }
            }
        ],
        "pagination": {
            "page": 1,
            "limit": 20,
            "total": 45,
            "totalPages": 3,
            "hasMore": true
        }
    }
}
```

**Business Logic:**
```
1. Query notifications for current user
2. Sort: unread first, then by createdAt DESC
3. Batch fetch actors (users who triggered)
4. Batch fetch anchors (for context)
5. Return enriched notifications with pagination
```

---

### 9. Get Unread Count

**Endpoint:** `GET /notifications/unread-count`

**Authentication:** Required

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "unreadCount": 5
    }
}
```

---

### 10. Mark as Read

**Endpoint:** `PATCH /notifications/:id/read`

**Authentication:** Required

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "id": "507f1f77bcf86cd799439030",
        "isRead": true
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid notification ID |
| 401 | UNAUTHORIZED | Authentication required |
| 403 | FORBIDDEN | Cannot mark others' notifications |
| 404 | NOT_FOUND | Notification not found |

---

### 11. Mark All as Read

**Endpoint:** `PATCH /notifications/read-all`

**Authentication:** Required

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "markedCount": 12
    }
}
```

**Business Logic:**
```
1. Update all notifications where recipientId = currentUser AND isRead = false
2. Set isRead = true
3. Return count of updated notifications
```

---

# PART 3: REQUEST/RESPONSE DTOs

---

## Comments DTOs

```go
// Sort constants
const (
    CommentSortNewest = "newest"
    CommentSortOldest = "oldest"
    CommentSortTop    = "top"
)

// CreateCommentRequest for POST /anchors/:id/comments
type CreateCommentRequest struct {
    Content string `json:"content" binding:"required,min=1,max=1000"`
}

// UpdateCommentRequest for PATCH /comments/:id
type UpdateCommentRequest struct {
    Content string `json:"content" binding:"required,min=1,max=1000"`
}

// CommentLikeActionRequest for POST /comments/:id/like
type CommentLikeActionRequest struct {
    Action string `json:"action" binding:"required,oneof=like unlike"`
}

// CommentListQuery for GET /anchors/:id/comments
type CommentListQuery struct {
    Page  int    `form:"page,default=1" binding:"min=1"`
    Limit int    `form:"limit,default=20" binding:"min=1,max=50"`
    Sort  string `form:"sort,default=newest"`
}

// CommentAuthor represents comment author
type CommentAuthor struct {
    ID             primitive.ObjectID `json:"id"`
    Username       string             `json:"username"`
    DisplayName    string             `json:"displayName"`
    ProfilePicture *string            `json:"profilePicture"`
    IsVerified     bool               `json:"isVerified"`
}

// CommentEngagement represents user's engagement
type CommentEngagement struct {
    HasLiked bool `json:"hasLiked"`
}

// CommentResponse represents enriched comment
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

// CommentLikeResponse for like action
type CommentLikeResponse struct {
    HasLiked  bool `json:"hasLiked"`
    LikeCount int  `json:"likeCount"`
}

// CommentListMeta for list metadata
type CommentListMeta struct {
    Sort     string             `json:"sort"`
    AnchorID primitive.ObjectID `json:"anchorId"`
}

// PaginatedCommentsResponse for list
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
```

---

## Notifications DTOs

```go
// Notification type constants
const (
    NotificationTypeMention      = "mention"
    NotificationTypeComment      = "comment"
    NotificationTypeAnchorUpdate = "anchor_update"
)

// NotificationListQuery for GET /notifications
type NotificationListQuery struct {
    Page       int  `form:"page,default=1" binding:"min=1"`
    Limit      int  `form:"limit,default=20" binding:"min=1,max=50"`
    UnreadOnly bool `form:"unreadOnly"`
}

// NotificationActor represents who triggered notification
type NotificationActor struct {
    ID             primitive.ObjectID `json:"id"`
    Username       string             `json:"username"`
    DisplayName    string             `json:"displayName"`
    ProfilePicture *string            `json:"profilePicture"`
}

// NotificationAnchor represents anchor context
type NotificationAnchor struct {
    ID    primitive.ObjectID `json:"id"`
    Title string             `json:"title"`
}

// NotificationResponse represents enriched notification
type NotificationResponse struct {
    ID           primitive.ObjectID  `json:"id"`
    Type         string              `json:"type"`
    ResourceType string              `json:"resourceType"`
    ResourceID   primitive.ObjectID  `json:"resourceId"`
    AnchorID     *primitive.ObjectID `json:"anchorId,omitempty"`
    Preview      string              `json:"preview"`
    IsRead       bool                `json:"isRead"`
    CreatedAt    time.Time           `json:"createdAt"`
    Actor        NotificationActor   `json:"actor"`
    Anchor       *NotificationAnchor `json:"anchor,omitempty"`
}

// PaginatedNotificationsResponse for list
type PaginatedNotificationsResponse struct {
    Notifications []NotificationResponse `json:"notifications"`
    Pagination    struct {
        Page       int   `json:"page"`
        Limit      int   `json:"limit"`
        Total      int64 `json:"total"`
        TotalPages int   `json:"totalPages"`
        HasMore    bool  `json:"hasMore"`
    } `json:"pagination"`
}

// UnreadCountResponse for unread count
type UnreadCountResponse struct {
    UnreadCount int64 `json:"unreadCount"`
}

// MarkReadResponse for mark as read
type MarkReadResponse struct {
    ID     primitive.ObjectID `json:"id"`
    IsRead bool               `json:"isRead"`
}

// MarkAllReadResponse for mark all as read
type MarkAllReadResponse struct {
    MarkedCount int64 `json:"markedCount"`
}
```

---

# PART 4: FILE STRUCTURE

---

## Comments Module

```
internal/features/comments/
├── model.go           # Comment, CommentLike, all DTOs
├── repository.go      # CRUD + like methods + indexes
├── mentions.go        # ExtractMentions, mention utilities
├── handler.go         # 7 HTTP handlers
├── validator.go       # Validation functions
└── routes.go          # Route registration
```

## Notifications Module

```
internal/features/notifications/
├── model.go           # Notification, all DTOs
├── repository.go      # CRUD + query methods + indexes
├── service.go         # CreateCommentNotifications, etc.
├── handler.go         # 4 HTTP handlers
├── validator.go       # Validation functions
└── routes.go          # Route registration
```

---

# PART 5: REPOSITORY METHODS

---

## Comments Repository

```go
type Repository struct {
    commentsCollection     *mongo.Collection
    commentLikesCollection *mongo.Collection
}

// NewRepository creates repository with indexes
func NewRepository(db *mongo.Database) *Repository

// Comment CRUD
func (r *Repository) CreateComment(ctx, comment *Comment) error
func (r *Repository) GetCommentByID(ctx, commentID ObjectID) (*Comment, error)
func (r *Repository) UpdateComment(ctx, commentID ObjectID, updates bson.M) error
func (r *Repository) SoftDeleteComment(ctx, commentID ObjectID) error

// Comment Queries
func (r *Repository) GetCommentsByAnchor(ctx, anchorID ObjectID, sort string, page, limit int) ([]Comment, int64, error)

// Comment Like Operations
func (r *Repository) CreateCommentLike(ctx, commentID, userID ObjectID) error
func (r *Repository) DeleteCommentLike(ctx, commentID, userID ObjectID) error
func (r *Repository) ExistsCommentLike(ctx, commentID, userID ObjectID) (bool, error)
func (r *Repository) GetUserLikedComments(ctx, userID ObjectID, commentIDs []ObjectID) (map[ObjectID]bool, error)

// Count Operations
func (r *Repository) IncrementCommentLikeCount(ctx, commentID ObjectID, delta int) error
```

---

## Notifications Repository

```go
type Repository struct {
    collection *mongo.Collection
}

// NewRepository creates repository with indexes
func NewRepository(db *mongo.Database) *Repository

// Create
func (r *Repository) CreateNotification(ctx, notification *Notification) error
func (r *Repository) CreateMany(ctx, notifications []Notification) error

// Query
func (r *Repository) GetNotificationByID(ctx, notificationID ObjectID) (*Notification, error)
func (r *Repository) GetUserNotifications(ctx, userID ObjectID, unreadOnly bool, page, limit int) ([]Notification, int64, error)
func (r *Repository) CountUnread(ctx, userID ObjectID) (int64, error)

// Update
func (r *Repository) MarkAsRead(ctx, notificationID ObjectID) error
func (r *Repository) MarkAllAsRead(ctx, userID ObjectID) (int64, error)
```

---

## Notifications Service

```go
type Service struct {
    repo        *Repository
    authRepo    *auth.Repository
    anchorsRepo *anchors.Repository
}

// CreateCommentNotifications creates notifications for mentions and anchor owner
func (s *Service) CreateCommentNotifications(ctx, comment *Comment, anchor *Anchor, actor *User) error

// CreateEditCommentNotifications creates notifications for NEW mentions only
func (s *Service) CreateEditCommentNotifications(ctx, comment *Comment, oldMentions []ObjectID, actor *User) error
```

---

# PART 6: CHANGES TO EXISTING CODE

---

## Auth Repository

Add method to look up users by usernames:

```go
// GetUsersByUsernames retrieves users by their usernames (for mention validation)
func (r *Repository) GetUsersByUsernames(ctx context.Context, usernames []string) ([]User, error) {
    if len(usernames) == 0 {
        return []User{}, nil
    }
    
    // Normalize to lowercase
    normalizedUsernames := make([]string, len(usernames))
    for i, u := range usernames {
        normalizedUsernames[i] = strings.ToLower(u)
    }
    
    filter := bson.M{
        "username": bson.M{"$in": normalizedUsernames},
    }
    
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

// GetUserIDsByUsernames returns map of username -> userID for valid usernames
func (r *Repository) GetUserIDsByUsernames(ctx context.Context, usernames []string) (map[string]primitive.ObjectID, error) {
    users, err := r.GetUsersByUsernames(ctx, usernames)
    if err != nil {
        return nil, err
    }
    
    result := make(map[string]primitive.ObjectID)
    for _, user := range users {
        result[strings.ToLower(user.Username)] = user.ID
    }
    
    return result, nil
}
```

---

## Anchors Repository

Add method:

```go
// IncrementCommentCount increments/decrements anchor's comment count
func (r *Repository) IncrementCommentCount(ctx context.Context, anchorID primitive.ObjectID, delta int) error {
    filter := bson.M{"_id": anchorID}
    update := bson.M{
        "$inc": bson.M{"commentCount": delta},
        "$set": bson.M{"updatedAt": time.Now()},
    }
    
    _, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
    if err != nil {
        return err
    }
    
    // Ensure count doesn't go negative
    if delta < 0 {
        _, _ = r.anchorsCollection.UpdateOne(ctx,
            bson.M{"_id": anchorID, "commentCount": bson.M{"$lt": 0}},
            bson.M{"$set": bson.M{"commentCount": 0}},
        )
    }
    
    return nil
}
```

---

# PART 7: IMPLEMENTATION ORDER

---

## Step 1: Auth Repository Updates
- Add `GetUsersByUsernames` method
- Add `GetUserIDsByUsernames` method

## Step 2: Anchors Repository Updates
- Add `IncrementCommentCount` method

## Step 3: Create Comments Module

### Step 3a: comments/model.go
- Comment struct (with Mentions field)
- CommentLike struct
- All DTOs and constants

### Step 3b: comments/mentions.go
- ExtractMentions function
- Mention validation helpers

### Step 3c: comments/repository.go
- NewRepository with indexes
- All CRUD methods
- Comment like methods

### Step 3d: comments/validator.go
- ValidateCreateCommentRequest
- ValidateUpdateCommentRequest
- ValidateCommentLikeActionRequest
- ValidateCommentListQuery

### Step 3e: comments/handler.go (WITHOUT notifications first)
- All 7 handlers
- Swagger annotations
- Skip notification creation for now

### Step 3f: comments/routes.go
- RegisterRoutes function

## Step 4: Create Notifications Module

### Step 4a: notifications/model.go
- Notification struct
- All DTOs and constants

### Step 4b: notifications/repository.go
- NewRepository with indexes
- All methods

### Step 4c: notifications/service.go
- CreateCommentNotifications
- CreateEditCommentNotifications

### Step 4d: notifications/validator.go
- ValidateNotificationListQuery

### Step 4e: notifications/handler.go
- All 4 handlers
- Swagger annotations

### Step 4f: notifications/routes.go
- RegisterRoutes function

## Step 5: Wire Up Notifications in Comments Handler
- Inject notification service into comments handler
- Add async notification creation in AddComment
- Add async notification creation in EditComment

## Step 6: Update routes/routes.go
- Register comments routes
- Register notifications routes

---

# PART 8: BUSINESS RULES SUMMARY

---

## Comment Rules

| # | Rule |
|---|------|
| 1 | Content: 1-1000 characters |
| 2 | Max 10 mentions per comment |
| 3 | Only comment author can edit |
| 4 | Comment author OR anchor owner can delete |
| 5 | Editing marks isEdited=true |
| 6 | Like/unlike are idempotent |
| 7 | One like per user per comment |

## Mention Rules

| # | Rule |
|---|------|
| 8 | Format: @username (3-30 alphanumeric + underscore) |
| 9 | Case-insensitive matching |
| 10 | Invalid usernames are silently ignored |
| 11 | Self-mentions don't trigger notifications |
| 12 | Duplicate mentions count as one |

## Notification Rules

| # | Rule |
|---|------|
| 13 | Mention notification for each valid @user |
| 14 | Comment notification to anchor owner |
| 15 | No duplicate: if owner is mentioned, only mention notification |
| 16 | No self-notification (commenting on own anchor) |
| 17 | Edit only notifies NEW mentions |
| 18 | Notifications are soft-sorted: unread first |

---

# PART 9: TESTING SCENARIOS

---

## Comments

- [ ] Add comment to public anchor
- [ ] Add comment with mentions
- [ ] Add comment with invalid mentions (silent ignore)
- [ ] Add comment to private anchor (owner) - success
- [ ] Add comment to private anchor (non-owner) - error
- [ ] List comments with newest sort
- [ ] List comments with top sort
- [ ] Edit own comment
- [ ] Edit updates isEdited flag
- [ ] Edit with new mention triggers notification
- [ ] Delete own comment
- [ ] Anchor owner delete any comment
- [ ] Like/unlike comment
- [ ] commentCount incremented/decremented

## Mentions

- [ ] Extract single mention
- [ ] Extract multiple mentions
- [ ] Deduplicate mentions
- [ ] Case-insensitive matching
- [ ] Invalid username ignored
- [ ] Max 10 mentions enforced

## Notifications

- [ ] Mention creates notification
- [ ] Comment on anchor creates notification for owner
- [ ] Self-mention no notification
- [ ] Self-comment no notification
- [ ] Owner mentioned = only mention notification (no duplicate)
- [ ] List notifications (unread first)
- [ ] Unread count correct
- [ ] Mark single as read
- [ ] Mark all as read
- [ ] Edit comment only notifies NEW mentions

---

# PART 10: FUTURE ENHANCEMENTS

---

## Additional Notification Types (Future)

| Type | Trigger |
|------|---------|
| `like` | Someone liked your anchor |
| `follow` | Someone followed you |
| `clone` | Someone cloned your anchor |
| `comment_like` | Someone liked your comment |

## Real-time (Future)
- WebSocket for instant notifications
- Push notifications (mobile)
- Email digest (daily/weekly)

## Comment Replies (Future)
- parentCommentId field
- Nested threading
- Reply notifications