# Backend Completion - Final 5 Endpoints

```
CONTEXT:
Completing Anchor app backend. Adding final missing endpoints.
Reference: ARCHITECTURE.md, STANDARDS.md, API_DOCUMENTATION.md

EXISTING: All major features implemented including Anchor Follows

TASK: Add final 5 missing endpoints

---

## ENDPOINT 1: Unblock User

**Endpoint:** `DELETE /users/{id}/block`
**Auth:** Required
**Location:** Add to existing block handler file

```go
// UnblockUser godoc
// @Summary Unblock a user
// @Description Remove a user from blocked list
// @Tags users
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID to unblock"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/block [delete]
func (h *Handler) UnblockUser(c *gin.Context) {
    currentUser, exists := c.Get("user")
    if !exists {
        response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
        return
    }
    user := currentUser.(*auth.User)

    targetIDStr := c.Param("id")
    targetID, err := primitive.ObjectIDFromHex(targetIDStr)
    if err != nil {
        response.BadRequest(c, "INVALID_ID", "Invalid user ID")
        return
    }

    if targetID == user.ID {
        response.BadRequest(c, "INVALID_ACTION", "Cannot unblock yourself")
        return
    }

    ctx := c.Request.Context()

    // Check if blocked
    isBlocked, err := h.blocksRepo.IsBlocked(ctx, user.ID, targetID)
    if err != nil {
        response.InternalServerError(c, "CHECK_FAILED", "Failed to check block status")
        return
    }

    if !isBlocked {
        response.BadRequest(c, "NOT_BLOCKED", "User is not blocked")
        return
    }

    // Remove block
    if err := h.blocksRepo.RemoveBlock(ctx, user.ID, targetID); err != nil {
        response.InternalServerError(c, "UNBLOCK_FAILED", "Failed to unblock user")
        return
    }

    response.Success(c, gin.H{
        "message": "User unblocked successfully",
    })
}
```

**Repository method:**
```go
func (r *Repository) RemoveBlock(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
    _, err := r.blocksCollection.DeleteOne(ctx, bson.M{
        "blockerId": blockerID,
        "blockedId": blockedID,
    })
    return err
}
```

**Route:**
```go
router.DELETE("/users/:id/block", authMiddleware, handler.UnblockUser)
```

---

## ENDPOINT 2: Get User by Username

**Endpoint:** `GET /users/username/{username}`
**Auth:** Optional
**Location:** users/handler.go

```go
// GetUserByUsername godoc
// @Summary Get user profile by username
// @Description Get public profile of a user by their username
// @Tags users
// @Produce json
// @Param username path string true "Username"
// @Success 200 {object} response.APIResponse{data=PublicProfileResponse}
// @Failure 404 {object} response.APIResponse
// @Router /users/username/{username} [get]
func (h *Handler) GetUserByUsername(c *gin.Context) {
    username := c.Param("username")
    if username == "" {
        response.BadRequest(c, "INVALID_USERNAME", "Username is required")
        return
    }

    username = strings.ToLower(strings.TrimSpace(username))

    ctx := c.Request.Context()

    user, err := h.authRepo.GetUserByUsername(ctx, username)
    if err != nil || user == nil {
        response.NotFound(c, "USER_NOT_FOUND", "User not found")
        return
    }

    // Get current user if authenticated
    var currentUserID *primitive.ObjectID
    if usr, exists := c.Get("user"); exists {
        if currentUser, ok := usr.(*auth.User); ok {
            currentUserID = &currentUser.ID
        }
    }

    // Build response (reuse existing buildPublicProfileResponse)
    resp := h.buildPublicProfileResponse(ctx, user, currentUserID)

    response.Success(c, resp)
}
```

**Repository method (auth/repository.go):**
```go
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
    var user User
    err := r.collection.FindOne(ctx, bson.M{
        "username": strings.ToLower(username),
    }).Decode(&user)
    
    if err == mongo.ErrNoDocuments {
        return nil, nil
    }
    return &user, err
}
```

**Route (add BEFORE /:id to avoid conflicts):**
```go
router.GET("/users/username/:username", optionalAuth, handler.GetUserByUsername)
router.GET("/users/:id", optionalAuth, handler.GetUserProfile)  // existing
```

---

## ENDPOINT 3: Check Username Availability

**Endpoint:** `GET /auth/username/check`
**Auth:** None
**Location:** auth/handler.go

```go
// CheckUsernameAvailability godoc
// @Summary Check if username is available
// @Description Check if a username is available for registration
// @Tags auth
// @Produce json
// @Param username query string true "Username to check (3-20 chars)"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Router /auth/username/check [get]
func (h *Handler) CheckUsernameAvailability(c *gin.Context) {
    username := c.Query("username")
    if username == "" {
        response.BadRequest(c, "MISSING_USERNAME", "Username query parameter is required")
        return
    }

    username = strings.ToLower(strings.TrimSpace(username))

    // Validate length
    if len(username) < 3 || len(username) > 20 {
        response.Success(c, gin.H{
            "username":  username,
            "available": false,
            "reason":    "Username must be 3-20 characters",
        })
        return
    }

    // Validate format (alphanumeric + underscore only)
    validUsername := regexp.MustCompile(`^[a-z0-9_]+$`)
    if !validUsername.MatchString(username) {
        response.Success(c, gin.H{
            "username":  username,
            "available": false,
            "reason":    "Username can only contain letters, numbers, and underscores",
        })
        return
    }

    // Check reserved usernames
    reserved := []string{"admin", "api", "www", "app", "help", "support", "anchor", "anchors", "user", "users", "settings", "login", "logout", "signup", "register", "me", "feed", "search", "notifications"}
    for _, r := range reserved {
        if username == r {
            response.Success(c, gin.H{
                "username":  username,
                "available": false,
                "reason":    "This username is reserved",
            })
            return
        }
    }

    ctx := c.Request.Context()

    // Check if exists in database
    existingUser, _ := h.repo.GetUserByUsername(ctx, username)
    available := existingUser == nil

    resp := gin.H{
        "username":  username,
        "available": available,
    }
    if !available {
        resp["reason"] = "Username is already taken"
    }

    response.Success(c, resp)
}
```

**Route:**
```go
auth.GET("/username/check", handler.CheckUsernameAvailability)
```

**Import:**
```go
import "regexp"
```

---

## ENDPOINT 4: Get User's Liked Anchors

**Endpoint:** `GET /users/{id}/likes`
**Auth:** Optional
**Location:** users/handler.go (or likes/handler.go)

```go
// GetUserLikes godoc
// @Summary Get user's liked anchors
// @Description Get paginated list of anchors liked by a user
// @Tags users
// @Produce json
// @Param id path string true "User ID (use 'me' for current user)"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/likes [get]
func (h *Handler) GetUserLikes(c *gin.Context) {
    userIDStr := c.Param("id")
    
    // Handle "me" case
    if userIDStr == "me" {
        currentUser, exists := c.Get("user")
        if !exists {
            response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
            return
        }
        user := currentUser.(*auth.User)
        userIDStr = user.ID.Hex()
    }

    userID, err := primitive.ObjectIDFromHex(userIDStr)
    if err != nil {
        response.BadRequest(c, "INVALID_ID", "Invalid user ID")
        return
    }

    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
    if page < 1 { page = 1 }
    if limit < 1 || limit > 50 { limit = 20 }

    ctx := c.Request.Context()

    // Verify user exists
    user, err := h.authRepo.GetUserByID(ctx, userID)
    if err != nil || user == nil {
        response.NotFound(c, "USER_NOT_FOUND", "User not found")
        return
    }

    // Get user's likes
    likes, total, err := h.likesRepo.GetUserLikedAnchors(ctx, userID, page, limit)
    if err != nil {
        response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch liked anchors")
        return
    }

    // Collect anchor IDs
    anchorIDs := make([]primitive.ObjectID, len(likes))
    for i, like := range likes {
        anchorIDs[i] = like.AnchorID
    }

    // Batch fetch anchors
    anchorsMap := make(map[primitive.ObjectID]*anchors.Anchor)
    if len(anchorIDs) > 0 {
        anchorsList, _ := h.anchorsRepo.GetAnchorsByIDs(ctx, anchorIDs)
        for i := range anchorsList {
            anchorsMap[anchorsList[i].ID] = &anchorsList[i]
        }
    }

    // Collect author IDs and fetch
    authorIDs := make([]primitive.ObjectID, 0)
    for _, anchor := range anchorsMap {
        authorIDs = append(authorIDs, anchor.UserID)
    }
    authorsMap := make(map[primitive.ObjectID]*auth.User)
    if len(authorIDs) > 0 {
        authors, _ := h.authRepo.GetUsersByIDs(ctx, authorIDs)
        for i := range authors {
            authorsMap[authors[i].ID] = &authors[i]
        }
    }

    // Build response
    items := make([]gin.H, 0)
    for _, like := range likes {
        anchor, ok := anchorsMap[like.AnchorID]
        if !ok || anchor == nil || anchor.DeletedAt != nil {
            continue
        }

        var authorInfo gin.H
        if author, ok := authorsMap[anchor.UserID]; ok {
            authorInfo = gin.H{
                "id":                author.ID,
                "username":          author.Username,
                "displayName":       author.DisplayName,
                "profilePictureUrl": author.ProfilePictureURL,
            }
        }

        items = append(items, gin.H{
            "id":          anchor.ID,
            "title":       anchor.Title,
            "description": anchor.Description,
            "itemCount":   anchor.ItemCount,
            "likeCount":   anchor.LikeCount,
            "likedAt":     like.CreatedAt,
            "author":      authorInfo,
        })
    }

    totalPages := int(math.Ceil(float64(total) / float64(limit)))

    response.Success(c, gin.H{
        "data": items,
        "pagination": gin.H{
            "page":       page,
            "limit":      limit,
            "total":      total,
            "totalPages": totalPages,
            "hasMore":    page < totalPages,
        },
    })
}
```

**Repository method (likes/repository.go):**
```go
func (r *Repository) GetUserLikedAnchors(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]Like, int64, error) {
    filter := bson.M{"userId": userID}
    
    total, err := r.collection.CountDocuments(ctx, filter)
    if err != nil {
        return nil, 0, err
    }

    opts := options.Find().
        SetSort(bson.D{{Key: "createdAt", Value: -1}}).
        SetSkip(int64((page - 1) * limit)).
        SetLimit(int64(limit))

    cursor, err := r.collection.Find(ctx, filter, opts)
    if err != nil {
        return nil, 0, err
    }
    defer cursor.Close(ctx)

    var likes []Like
    if err = cursor.All(ctx, &likes); err != nil {
        return nil, 0, err
    }

    return likes, total, nil
}
```

**Routes:**
```go
router.GET("/users/me/likes", authMiddleware, handler.GetUserLikes)
router.GET("/users/:id/likes", optionalAuth, handler.GetUserLikes)
```

---

## ENDPOINT 5: Get User's Cloned Anchors

**Endpoint:** `GET /users/{id}/clones`
**Auth:** Optional
**Location:** users/handler.go

```go
// GetUserClones godoc
// @Summary Get user's cloned anchors
// @Description Get paginated list of anchors cloned by a user
// @Tags users
// @Produce json
// @Param id path string true "User ID (use 'me' for current user)"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/clones [get]
func (h *Handler) GetUserClones(c *gin.Context) {
    userIDStr := c.Param("id")
    
    if userIDStr == "me" {
        currentUser, exists := c.Get("user")
        if !exists {
            response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
            return
        }
        user := currentUser.(*auth.User)
        userIDStr = user.ID.Hex()
    }

    userID, err := primitive.ObjectIDFromHex(userIDStr)
    if err != nil {
        response.BadRequest(c, "INVALID_ID", "Invalid user ID")
        return
    }

    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
    if page < 1 { page = 1 }
    if limit < 1 || limit > 50 { limit = 20 }

    ctx := c.Request.Context()

    // Verify user exists
    user, err := h.authRepo.GetUserByID(ctx, userID)
    if err != nil || user == nil {
        response.NotFound(c, "USER_NOT_FOUND", "User not found")
        return
    }

    // Get cloned anchors (isClone=true)
    clones, total, err := h.anchorsRepo.GetUserClonedAnchors(ctx, userID, page, limit)
    if err != nil {
        response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch cloned anchors")
        return
    }

    // Collect original anchor IDs
    originalIDs := make([]primitive.ObjectID, 0)
    for _, clone := range clones {
        if clone.OriginalAnchorID != nil {
            originalIDs = append(originalIDs, *clone.OriginalAnchorID)
        }
    }

    // Batch fetch originals
    originalsMap := make(map[primitive.ObjectID]*anchors.Anchor)
    if len(originalIDs) > 0 {
        originals, _ := h.anchorsRepo.GetAnchorsByIDs(ctx, originalIDs)
        for i := range originals {
            originalsMap[originals[i].ID] = &originals[i]
        }
    }

    // Collect original authors
    authorIDs := make([]primitive.ObjectID, 0)
    for _, orig := range originalsMap {
        authorIDs = append(authorIDs, orig.UserID)
    }
    authorsMap := make(map[primitive.ObjectID]*auth.User)
    if len(authorIDs) > 0 {
        authors, _ := h.authRepo.GetUsersByIDs(ctx, authorIDs)
        for i := range authors {
            authorsMap[authors[i].ID] = &authors[i]
        }
    }

    // Build response
    items := make([]gin.H, 0)
    for _, clone := range clones {
        item := gin.H{
            "id":               clone.ID,
            "title":            clone.Title,
            "description":      clone.Description,
            "itemCount":        clone.ItemCount,
            "likeCount":        clone.LikeCount,
            "isClone":          true,
            "clonedFromId":     clone.ClonedFromID,
            "originalAnchorId": clone.OriginalAnchorID,
            "clonedAt":         clone.CreatedAt,
        }

        // Add original author info
        if clone.OriginalAnchorID != nil {
            if orig, ok := originalsMap[*clone.OriginalAnchorID]; ok {
                if author, ok := authorsMap[orig.UserID]; ok {
                    item["originalAuthor"] = gin.H{
                        "id":          author.ID,
                        "username":    author.Username,
                        "displayName": author.DisplayName,
                    }
                }
            }
        }

        items = append(items, item)
    }

    totalPages := int(math.Ceil(float64(total) / float64(limit)))

    response.Success(c, gin.H{
        "data": items,
        "pagination": gin.H{
            "page":       page,
            "limit":      limit,
            "total":      total,
            "totalPages": totalPages,
            "hasMore":    page < totalPages,
        },
    })
}
```

**Repository method (anchors/repository.go):**
```go
func (r *Repository) GetUserClonedAnchors(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]Anchor, int64, error) {
    filter := bson.M{
        "userId":    userID,
        "isClone":   true,
        "deletedAt": nil,
    }
    
    total, err := r.collection.CountDocuments(ctx, filter)
    if err != nil {
        return nil, 0, err
    }

    opts := options.Find().
        SetSort(bson.D{{Key: "createdAt", Value: -1}}).
        SetSkip(int64((page - 1) * limit)).
        SetLimit(int64(limit))

    cursor, err := r.collection.Find(ctx, filter, opts)
    if err != nil {
        return nil, 0, err
    }
    defer cursor.Close(ctx)

    var anchors []Anchor
    if err = cursor.All(ctx, &anchors); err != nil {
        return nil, 0, err
    }

    return anchors, total, nil
}
```

**Routes:**
```go
router.GET("/users/me/clones", authMiddleware, handler.GetUserClones)
router.GET("/users/:id/clones", optionalAuth, handler.GetUserClones)
```

---

## VERIFICATION CHECKLIST

Endpoints Added:
[ ] DELETE /users/{id}/block - Unblock user
[ ] GET /users/username/{username} - Get by username  
[ ] GET /auth/username/check - Check availability
[ ] GET /users/{id}/likes - User's liked anchors
[ ] GET /users/me/likes - My liked anchors
[ ] GET /users/{id}/clones - User's cloned anchors
[ ] GET /users/me/clones - My cloned anchors

Repository Methods:
[ ] RemoveBlock (blocks)
[ ] GetUserByUsername (auth)
[ ] GetUserLikedAnchors (likes)
[ ] GetUserClonedAnchors (anchors)

Routes:
[ ] Routes registered in correct order
[ ] /users/username/:username BEFORE /users/:id
[ ] Auth middleware applied correctly

Swagger:
[ ] Run: swag init -g cmd/api/main.go -o docs

---

## API DOC UPDATES

Add these to your API_DOCUMENTATION.md:

### 2.10 Get User by Username
**Endpoint:** `GET /users/username/{username}`
**Description:** Get public profile by username

### 1.7 Check Username Availability
**Endpoint:** `GET /auth/username/check?username=xxx`
**Response:** `{ "username": "xxx", "available": true/false, "reason": "..." }`

### 2.11 Get User's Liked Anchors
**Endpoint:** `GET /users/{id}/likes`
**Description:** Get anchors liked by user (supports /users/me/likes)

### 2.12 Get User's Cloned Anchors
**Endpoint:** `GET /users/{id}/clones`
**Description:** Get anchors cloned by user (supports /users/me/clones)

### 12.4 Unblock User
**Endpoint:** `DELETE /users/{id}/block`
**Description:** Remove user from blocked list

---

After implementation, total endpoints: ~77
```