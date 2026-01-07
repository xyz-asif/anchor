# Additional Notifications Specification

## Overview

This specification extends the existing Notifications system with three new notification types: Like, Follow, and Clone. These complete the engagement loop by notifying users when others interact with their content or profile.

---

## Notification Types Summary

| Type | Status | Trigger | Message Example |
|------|--------|---------|-----------------|
| `mention` | ✅ Exists | @username in comment | "@alice mentioned you in a comment" |
| `comment` | ✅ Exists | Comment on your anchor | "@alice commented on your anchor" |
| `like` | 🆕 New | Like your anchor | "@alice liked your anchor 'Design Resources'" |
| `follow` | 🆕 New | Follow you | "@alice started following you" |
| `clone` | 🆕 New | Clone your anchor | "@alice cloned your anchor 'Design Resources'" |

---

## New Notification Types

### 1. Like Notification

**Trigger:** User likes an anchor

**Notification Data:**
```json
{
    "id": "507f1f77bcf86cd799439030",
    "recipientId": "507f1f77bcf86cd799439012",
    "actorId": "507f1f77bcf86cd799439013",
    "type": "like",
    "resourceType": "anchor",
    "resourceId": "507f1f77bcf86cd799439011",
    "anchorId": "507f1f77bcf86cd799439011",
    "preview": "UI Design Resources 2024",
    "isRead": false,
    "createdAt": "2024-01-15T10:30:00Z"
}
```

**Display:** "@alice liked your anchor 'UI Design Resources 2024'"

**Rules:**
- No notification if liking own anchor
- Only notify on LIKE action (not unlike)
- Check if already liked to prevent duplicate notifications on re-like

---

### 2. Follow Notification

**Trigger:** User follows another user

**Notification Data:**
```json
{
    "id": "507f1f77bcf86cd799439031",
    "recipientId": "507f1f77bcf86cd799439012",
    "actorId": "507f1f77bcf86cd799439013",
    "type": "follow",
    "resourceType": "user",
    "resourceId": "507f1f77bcf86cd799439013",
    "anchorId": null,
    "preview": null,
    "isRead": false,
    "createdAt": "2024-01-15T10:30:00Z"
}
```

**Display:** "@alice started following you"

**Rules:**
- No notification if following self (shouldn't be possible anyway)
- Only notify on FOLLOW action (not unfollow)
- Check if already following to prevent duplicate notifications on re-follow

---

### 3. Clone Notification

**Trigger:** User clones an anchor

**Notification Data:**
```json
{
    "id": "507f1f77bcf86cd799439032",
    "recipientId": "507f1f77bcf86cd799439012",
    "actorId": "507f1f77bcf86cd799439013",
    "type": "clone",
    "resourceType": "anchor",
    "resourceId": "507f1f77bcf86cd799439014",
    "anchorId": "507f1f77bcf86cd799439011",
    "preview": "UI Design Resources 2024",
    "isRead": false,
    "createdAt": "2024-01-15T10:30:00Z"
}
```

**Note:** 
- `resourceId` = the NEW cloned anchor ID
- `anchorId` = the ORIGINAL anchor ID (for linking back)

**Display:** "@alice cloned your anchor 'UI Design Resources 2024'"

**Rules:**
- No notification if cloning own anchor
- Notify every time (cloning is a significant action)

---

## Service Methods

### notifications/service.go

```go
// Type constants (add to existing)
const (
    TypeMention = "mention"
    TypeComment = "comment"
    TypeLike    = "like"    // NEW
    TypeFollow  = "follow"  // NEW
    TypeClone   = "clone"   // NEW
)

// CreateLikeNotification creates notification when someone likes an anchor
// Returns early if actor is the anchor owner (no self-notification)
func (s *Service) CreateLikeNotification(ctx context.Context, anchorID primitive.ObjectID, anchorTitle string, actorID, ownerID primitive.ObjectID) error

// CreateFollowNotification creates notification when someone follows a user
// Returns early if actor is the target (no self-notification)
func (s *Service) CreateFollowNotification(ctx context.Context, actorID, targetUserID primitive.ObjectID) error

// CreateCloneNotification creates notification when someone clones an anchor
// Returns early if actor is the anchor owner (no self-notification)
// clonedAnchorID is the new anchor, originalAnchorID is the source
func (s *Service) CreateCloneNotification(ctx context.Context, clonedAnchorID, originalAnchorID primitive.ObjectID, anchorTitle string, actorID, ownerID primitive.ObjectID) error
```

---

## Integration Points

### 1. Likes Handler (likes/handler.go)

**In LikeAnchor handler, after successful LIKE action:**

```go
// Only notify on like (not unlike), and only if not own anchor
if req.Action == "like" && anchor.UserID != currentUser.ID {
    // Check if this is a NEW like (not a re-like)
    // The CreateLike already handles idempotency, but we need to check
    // if the like existed BEFORE this request
    go func() {
        _ = h.notificationService.CreateLikeNotification(
            context.Background(),
            anchorID,
            anchor.Title,
            currentUser.ID,
            anchor.UserID,
        )
    }()
}
```

**Note:** To prevent duplicate notifications on re-like, we need to track if the like already existed. Options:
1. Check `wasAlreadyLiked` before creating like
2. Use unique constraint in notifications (anchorId + actorId + type)
3. Accept occasional duplicates (simpler)

**Recommendation:** Option 1 - Check before creating

---

### 2. Follows Handler (follows/handler.go)

**In Follow handler, after successful FOLLOW action:**

```go
// Only notify on follow (not unfollow), and only if not self
if req.Action == "follow" && targetUserID != currentUser.ID {
    go func() {
        _ = h.notificationService.CreateFollowNotification(
            context.Background(),
            currentUser.ID,
            targetUserID,
        )
    }()
}
```

**Note:** Same consideration for re-follow duplicates.

---

### 3. Anchors Handler (anchors/handler.go)

**In CloneAnchor handler, after successful clone:**

```go
// Notify original anchor owner (if not self)
if sourceAnchor.UserID != currentUser.ID {
    go func() {
        _ = h.notificationService.CreateCloneNotification(
            context.Background(),
            clonedAnchor.ID,      // New anchor
            sourceAnchor.ID,       // Original anchor
            sourceAnchor.Title,
            currentUser.ID,
            sourceAnchor.UserID,
        )
    }()
}
```

---

## Duplicate Prevention Strategy

### Option 1: Check Before Notify (Recommended)

For likes and follows, check if action is NEW before notifying:

**Likes:**
```go
// In handler, before creating like
wasAlreadyLiked, _ := h.repo.ExistsLike(ctx, anchorID, currentUser.ID)

// Create like (idempotent)
_ = h.repo.CreateLike(ctx, anchorID, currentUser.ID)

// Only notify if this was a NEW like
if !wasAlreadyLiked && anchor.UserID != currentUser.ID {
    go func() {
        _ = h.notificationService.CreateLikeNotification(...)
    }()
}
```

**Follows:**
```go
// In handler, before creating follow
wasAlreadyFollowing, _ := h.repo.ExistsFollow(ctx, currentUser.ID, targetUserID)

// Create follow (idempotent)
_ = h.repo.CreateFollow(ctx, currentUser.ID, targetUserID)

// Only notify if this was a NEW follow
if !wasAlreadyFollowing && targetUserID != currentUser.ID {
    go func() {
        _ = h.notificationService.CreateFollowNotification(...)
    }()
}
```

### Option 2: Unique Constraint in Notifications

Add compound unique index on notifications:
```go
{ recipientId: 1, actorId: 1, type: 1, resourceId: 1 } - unique, sparse
```

Then ignore duplicate key errors when creating.

**Note:** This prevents ALL duplicates, which might not be desired for some cases.

---

## Handler Dependencies Update

### Likes Handler

```go
type Handler struct {
    repo                *Repository
    anchorsRepo         *anchors.Repository
    authRepo            *auth.Repository
    notificationService *notifications.Service  // ADD
    config              *config.Config
}
```

### Follows Handler

```go
type Handler struct {
    repo                *Repository
    authRepo            *auth.Repository
    notificationService *notifications.Service  // ADD
    config              *config.Config
}
```

### Anchors Handler

```go
type Handler struct {
    repo                *Repository
    authRepo            *auth.Repository
    notificationService *notifications.Service  // ADD
    config              *config.Config
}
```

---

## Files to Modify

| File | Changes |
|------|---------|
| `notifications/model.go` | Add TypeLike, TypeFollow, TypeClone constants |
| `notifications/service.go` | Add 3 new methods |
| `notifications/routes.go` | Export GetService (already exists) |
| `likes/handler.go` | Add notificationService, call CreateLikeNotification |
| `likes/routes.go` | Initialize notificationService, pass to handler |
| `follows/handler.go` | Add notificationService, call CreateFollowNotification |
| `follows/routes.go` | Initialize notificationService, pass to handler |
| `anchors/handler.go` | Add notificationService, call CreateCloneNotification |
| `anchors/routes.go` | Initialize notificationService, pass to handler |

---

## Business Rules Summary

| # | Rule |
|---|------|
| 1 | No self-notifications (liking own anchor, cloning own anchor) |
| 2 | Like: Only notify on NEW like (not re-like after unlike) |
| 3 | Follow: Only notify on NEW follow (not re-follow after unfollow) |
| 4 | Clone: Always notify (every clone is significant) |
| 5 | All notifications created asynchronously |
| 6 | Notifications include preview text where applicable |

---

## Response Display (Frontend Reference)

| Type | Display Format |
|------|----------------|
| mention | "@{actor.username} mentioned you: '{preview}'" |
| comment | "@{actor.username} commented on your anchor '{anchor.title}'" |
| like | "@{actor.username} liked your anchor '{anchor.title}'" |
| follow | "@{actor.username} started following you" |
| clone | "@{actor.username} cloned your anchor '{anchor.title}'" |

---

## Testing Scenarios

### Like Notifications
- [ ] Like anchor creates notification for owner
- [ ] Like own anchor does NOT create notification
- [ ] Unlike does NOT create notification
- [ ] Re-like (after unlike) does NOT create duplicate notification

### Follow Notifications
- [ ] Follow user creates notification
- [ ] Unfollow does NOT create notification
- [ ] Re-follow (after unfollow) does NOT create duplicate notification

### Clone Notifications
- [ ] Clone anchor creates notification for original owner
- [ ] Clone own anchor does NOT create notification
- [ ] Multiple clones of same anchor create multiple notifications (expected)

### General
- [ ] All notifications appear in /notifications list
- [ ] Unread count increases correctly
- [ ] Actor info enriched correctly
- [ ] Anchor info enriched correctly (where applicable)