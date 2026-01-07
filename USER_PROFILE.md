# User Profile Specification

## Overview

The User Profile feature allows users to view public profiles, manage their own profile information, upload profile pictures and cover images, and display pinned anchors. This extends the existing Auth module.

---

## API Summary

| # | Method | Endpoint | Auth | Description |
|---|--------|----------|------|-------------|
| 1 | `GET` | `/users/:id` | Optional | Get user's public profile |
| 2 | `GET` | `/users/me` | Required | Get current user's full profile |
| 3 | `PATCH` | `/users/me` | Required | Update current user's profile |
| 4 | `POST` | `/users/me/profile-picture` | Required | Upload profile picture |
| 5 | `POST` | `/users/me/cover-image` | Required | Upload cover image |
| 6 | `DELETE` | `/users/me/profile-picture` | Required | Remove profile picture |
| 7 | `DELETE` | `/users/me/cover-image` | Required | Remove cover image |
| 8 | `GET` | `/users/:id/pinned` | Optional | Get user's pinned anchors |

**Total: 8 endpoints**

---

## Data Model Updates

### User Model (update existing in auth/model.go)

Add these fields to the existing User struct:

```go
type User struct {
    // ... existing fields ...
    
    // New fields for profile images
    CoverImageURL          string `bson:"coverImageUrl" json:"coverImageUrl"`
    ProfilePicturePublicID string `bson:"profilePicturePublicId" json:"-"` // Cloudinary public ID
    CoverImagePublicID     string `bson:"coverImagePublicId" json:"-"`     // Cloudinary public ID
}
```

---

## API Endpoints

### 1. Get Public Profile

**Endpoint:** `GET /users/:id`

**Authentication:** Optional (needed for follow status)

**Description:** Get a user's public profile information. If authenticated, includes follow relationship status.

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | User ID (ObjectID) |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "id": "507f1f77bcf86cd799439011",
        "username": "@johndoe",
        "displayName": "John Doe",
        "bio": "Software developer & bookmark enthusiast",
        "profilePicture": "https://cloudinary.com/...",
        "coverImage": "https://cloudinary.com/...",
        "followerCount": 150,
        "followingCount": 75,
        "anchorCount": 12,
        "createdAt": "2024-01-01T00:00:00Z",
        "isFollowing": true,
        "isFollowedBy": false,
        "isMutual": false
    }
}
```

**Response Fields:**
| Field | Type | Description |
|-------|------|-------------|
| id | string | User's ID |
| username | string | Username with @ prefix |
| displayName | string | Display name |
| bio | string | User's bio |
| profilePicture | string | Profile picture URL |
| coverImage | string | Cover image URL |
| followerCount | int | Number of followers |
| followingCount | int | Number following |
| anchorCount | int | Number of anchors |
| createdAt | datetime | Account creation date |
| isFollowing | bool | Does current user follow this user? (only if authenticated) |
| isFollowedBy | bool | Does this user follow current user? (only if authenticated) |
| isMutual | bool | Both follow each other? (only if authenticated) |

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid user ID format |
| 404 | USER_NOT_FOUND | User not found |

---

### 2. Get Own Profile

**Endpoint:** `GET /users/me`

**Authentication:** Required

**Description:** Get the current authenticated user's full profile, including private fields like email.

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "id": "507f1f77bcf86cd799439011",
        "email": "john@example.com",
        "username": "@johndoe",
        "displayName": "John Doe",
        "bio": "Software developer & bookmark enthusiast",
        "profilePicture": "https://cloudinary.com/...",
        "coverImage": "https://cloudinary.com/...",
        "followerCount": 150,
        "followingCount": 75,
        "anchorCount": 12,
        "createdAt": "2024-01-01T00:00:00Z",
        "updatedAt": "2024-01-15T10:30:00Z"
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 401 | UNAUTHORIZED | Authentication required |

---

### 3. Update Profile

**Endpoint:** `PATCH /users/me`

**Authentication:** Required

**Description:** Update the current user's profile information. Only provided fields are updated.

**Request Body:**
```json
{
    "displayName": "John D.",
    "bio": "Updated bio text"
}
```

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| displayName | string | No | 2-50 chars | Display name |
| bio | string | No | 0-200 chars | User bio (empty string to clear) |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "id": "507f1f77bcf86cd799439011",
        "email": "john@example.com",
        "username": "@johndoe",
        "displayName": "John D.",
        "bio": "Updated bio text",
        "profilePicture": "https://...",
        "coverImage": "https://...",
        "followerCount": 150,
        "followingCount": 75,
        "anchorCount": 12,
        "createdAt": "2024-01-01T00:00:00Z",
        "updatedAt": "2024-01-15T12:00:00Z"
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | VALIDATION_FAILED | Validation error details |
| 401 | UNAUTHORIZED | Authentication required |

**Note:** Username cannot be changed via this endpoint.

---

### 4. Upload Profile Picture

**Endpoint:** `POST /users/me/profile-picture`

**Authentication:** Required

**Description:** Upload a new profile picture. Replaces existing picture if one exists (deletes old from Cloudinary).

**Request:** `multipart/form-data`

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| file | file | Yes | Max 5MB, .jpg/.jpeg/.png/.webp | Profile picture file |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "profilePicture": "https://cloudinary.com/..."
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | MISSING_FILE | File is required |
| 400 | INVALID_FILE | Invalid file type or size |
| 401 | UNAUTHORIZED | Authentication required |
| 500 | UPLOAD_FAILED | Failed to upload file |

**Business Logic:**
1. Validate file (type and size)
2. If user has existing profile picture, delete from Cloudinary
3. Upload new image to Cloudinary (folder: `profiles/pictures`)
4. Update user's profilePictureUrl and profilePicturePublicId
5. Return new URL

---

### 5. Upload Cover Image

**Endpoint:** `POST /users/me/cover-image`

**Authentication:** Required

**Description:** Upload a new cover image. Replaces existing image if one exists.

**Request:** `multipart/form-data`

| Field | Type | Required | Constraints | Description |
|-------|------|----------|-------------|-------------|
| file | file | Yes | Max 10MB, .jpg/.jpeg/.png/.webp | Cover image file |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "coverImage": "https://cloudinary.com/..."
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | MISSING_FILE | File is required |
| 400 | INVALID_FILE | Invalid file type or size |
| 401 | UNAUTHORIZED | Authentication required |
| 500 | UPLOAD_FAILED | Failed to upload file |

**Business Logic:**
1. Validate file (type and size)
2. If user has existing cover image, delete from Cloudinary
3. Upload new image to Cloudinary (folder: `profiles/covers`)
4. Update user's coverImageUrl and coverImagePublicId
5. Return new URL

---

### 6. Remove Profile Picture

**Endpoint:** `DELETE /users/me/profile-picture`

**Authentication:** Required

**Description:** Remove the current user's profile picture.

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "message": "Profile picture removed"
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 401 | UNAUTHORIZED | Authentication required |

**Business Logic:**
1. If user has profile picture, delete from Cloudinary
2. Set profilePictureUrl and profilePicturePublicId to empty string
3. Return success

---

### 7. Remove Cover Image

**Endpoint:** `DELETE /users/me/cover-image`

**Authentication:** Required

**Description:** Remove the current user's cover image.

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": {
        "message": "Cover image removed"
    }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 401 | UNAUTHORIZED | Authentication required |

---

### 8. Get Pinned Anchors

**Endpoint:** `GET /users/:id/pinned`

**Authentication:** Optional

**Description:** Get a user's pinned anchors (max 3). If viewing own profile, includes private pinned anchors. If viewing others, only public/unlisted.

**Path Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | User ID (ObjectID) |

**Success Response (200 OK):**
```json
{
    "success": true,
    "statusCode": 200,
    "message": "success",
    "data": [
        {
            "id": "507f1f77bcf86cd799439012",
            "title": "Tech Resources",
            "description": "My favorite tech links",
            "coverMediaType": "emoji",
            "coverMediaValue": "💻",
            "visibility": "public",
            "itemCount": 25,
            "likeCount": 10,
            "cloneCount": 3,
            "createdAt": "2024-01-10T00:00:00Z"
        },
        {
            "id": "507f1f77bcf86cd799439013",
            "title": "Design Inspiration",
            "description": "UI/UX resources",
            "coverMediaType": "emoji",
            "coverMediaValue": "🎨",
            "visibility": "public",
            "itemCount": 15,
            "likeCount": 5,
            "cloneCount": 1,
            "createdAt": "2024-01-12T00:00:00Z"
        }
    ]
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 400 | INVALID_ID | Invalid user ID format |
| 404 | USER_NOT_FOUND | User not found |

**Business Logic:**
1. Get target user ID from path
2. Check if current user is authenticated
3. Query anchors where userId = targetId AND isPinned = true
4. If viewer is NOT the owner: filter to only public/unlisted
5. Return anchors (max 3)

---

## Request/Response DTOs

### Request DTOs

```go
// UpdateProfileRequest for PATCH /users/me
type UpdateProfileRequest struct {
    DisplayName *string `json:"displayName" binding:"omitempty,min=2,max=50"`
    Bio         *string `json:"bio" binding:"omitempty,max=200"`
}
```

### Response DTOs

```go
// PublicProfileResponse for GET /users/:id
type PublicProfileResponse struct {
    ID             primitive.ObjectID `json:"id"`
    Username       string             `json:"username"`
    DisplayName    string             `json:"displayName"`
    Bio            string             `json:"bio"`
    ProfilePicture string             `json:"profilePicture"`
    CoverImage     string             `json:"coverImage"`
    FollowerCount  int                `json:"followerCount"`
    FollowingCount int                `json:"followingCount"`
    AnchorCount    int                `json:"anchorCount"`
    CreatedAt      time.Time          `json:"createdAt"`
    IsFollowing    bool               `json:"isFollowing,omitempty"`
    IsFollowedBy   bool               `json:"isFollowedBy,omitempty"`
    IsMutual       bool               `json:"isMutual,omitempty"`
}

// OwnProfileResponse for GET /users/me
type OwnProfileResponse struct {
    ID             primitive.ObjectID `json:"id"`
    Email          string             `json:"email"`
    Username       string             `json:"username"`
    DisplayName    string             `json:"displayName"`
    Bio            string             `json:"bio"`
    ProfilePicture string             `json:"profilePicture"`
    CoverImage     string             `json:"coverImage"`
    FollowerCount  int                `json:"followerCount"`
    FollowingCount int                `json:"followingCount"`
    AnchorCount    int                `json:"anchorCount"`
    CreatedAt      time.Time          `json:"createdAt"`
    UpdatedAt      time.Time          `json:"updatedAt"`
}

// ProfilePictureResponse for upload responses
type ProfilePictureResponse struct {
    ProfilePicture string `json:"profilePicture"`
}

// CoverImageResponse for upload responses
type CoverImageResponse struct {
    CoverImage string `json:"coverImage"`
}

// PinnedAnchorResponse for pinned anchors list
type PinnedAnchorResponse struct {
    ID              primitive.ObjectID `json:"id"`
    Title           string             `json:"title"`
    Description     string             `json:"description"`
    CoverMediaType  string             `json:"coverMediaType"`
    CoverMediaValue string             `json:"coverMediaValue"`
    Visibility      string             `json:"visibility"`
    ItemCount       int                `json:"itemCount"`
    LikeCount       int                `json:"likeCount"`
    CloneCount      int                `json:"cloneCount"`
    CreatedAt       time.Time          `json:"createdAt"`
}
```

---

## Business Rules

### Profile Viewing
| # | Rule |
|---|------|
| 1 | Anyone can view public profile information |
| 2 | Email is only visible in own profile (/me) |
| 3 | Follow status only shown if viewer is authenticated |

### Profile Updates
| # | Rule |
|---|------|
| 4 | Username cannot be changed after registration |
| 5 | DisplayName must be 2-50 characters |
| 6 | Bio can be 0-200 characters (empty to clear) |
| 7 | Only non-nil fields in request are updated |

### Profile Pictures
| # | Rule |
|---|------|
| 8 | Profile picture max size: 5MB |
| 9 | Cover image max size: 10MB |
| 10 | Allowed types: .jpg, .jpeg, .png, .webp |
| 11 | Old image is deleted from Cloudinary when replaced |
| 12 | Images stored in Cloudinary folders: `profiles/pictures`, `profiles/covers` |

### Pinned Anchors
| # | Rule |
|---|------|
| 13 | Maximum 3 pinned anchors per user |
| 14 | Only owner can see their private pinned anchors |
| 15 | Others see only public/unlisted pinned anchors |

---

## Validation Rules

### DisplayName
- Minimum: 2 characters
- Maximum: 50 characters
- Optional in update (only update if provided)

### Bio
- Minimum: 0 characters (can be empty)
- Maximum: 200 characters
- Optional in update

### Profile Picture
- Max size: 5MB (5 * 1024 * 1024 bytes)
- Allowed types: image/jpeg, image/png, image/webp
- Allowed extensions: .jpg, .jpeg, .png, .webp

### Cover Image
- Max size: 10MB (10 * 1024 * 1024 bytes)
- Allowed types: image/jpeg, image/png, image/webp
- Allowed extensions: .jpg, .jpeg, .png, .webp

---

## Files to Update/Create

### Update Existing Files

```
internal/features/auth/
├── model.go       # Add new fields to User, add DTOs
├── repository.go  # Add GetUserByObjectID, update user methods
├── handler.go     # Add 8 new handlers
└── routes.go      # Add new routes
```

### Handler Dependencies

The auth handler needs access to:
- `cloudinary.Service` - for image uploads
- `follows.Repository` - for follow status in public profile
- `anchors.Repository` - for pinned anchors

Update Handler struct:
```go
type Handler struct {
    repo        *Repository
    config      *config.Config
    cloudinary  *cloudinary.Service   // NEW
    followsRepo *follows.Repository   // NEW
    anchorsRepo *anchors.Repository   // NEW
}
```

---

## Repository Methods

### Existing (may need updates)
- `GetUserByID(ctx, userID string)` - already exists
- `UpdateUser(ctx, userID string, updates map[string]interface{})` - already exists

### New Methods Needed
```go
// GetUserByObjectID fetches user by ObjectID directly
func (r *Repository) GetUserByObjectID(ctx context.Context, userID primitive.ObjectID) (*User, error)
```

---

## Route Registration

```go
// In auth/routes.go - add to existing routes

// Profile routes
users := router.Group("/users")
{
    // Public profile (with optional auth for follow status)
    users.GET("/:id", optionalAuth, handler.GetPublicProfile)
    users.GET("/:id/pinned", optionalAuth, handler.GetPinnedAnchors)
    
    // Own profile (requires auth)
    me := users.Group("/me")
    me.Use(authMiddleware)
    {
        me.GET("", handler.GetOwnProfile)
        me.PATCH("", handler.UpdateProfile)
        me.POST("/profile-picture", handler.UploadProfilePicture)
        me.POST("/cover-image", handler.UploadCoverImage)
        me.DELETE("/profile-picture", handler.RemoveProfilePicture)
        me.DELETE("/cover-image", handler.RemoveCoverImage)
    }
}
```

**Note:** Be careful with route ordering - `/users/me` must be registered before `/users/:id` to avoid conflicts.

---

## Implementation Order

### Step 1: Update User Model
- Add CoverImageURL field
- Add ProfilePicturePublicID field
- Add CoverImagePublicID field
- Add all DTOs (UpdateProfileRequest, PublicProfileResponse, etc.)

### Step 2: Update Auth Repository
- Add GetUserByObjectID method

### Step 3: Update Auth Handler
- Add cloudinary, followsRepo, anchorsRepo to Handler struct
- Update NewHandler constructor
- Add GetPublicProfile handler
- Add GetOwnProfile handler
- Add UpdateProfile handler
- Add UploadProfilePicture handler
- Add UploadCoverImage handler
- Add RemoveProfilePicture handler
- Add RemoveCoverImage handler
- Add GetPinnedAnchors handler

### Step 4: Add Validation Functions
- Add ValidateUpdateProfileRequest
- Add ValidateProfilePicture
- Add ValidateCoverImage

### Step 5: Update Routes
- Add new routes with proper middleware
- Ensure /me routes are before /:id routes

### Step 6: Update Auth Routes Initialization
- Initialize cloudinary service
- Initialize follows repository
- Initialize anchors repository
- Pass to NewHandler

---

## Testing Scenarios

### Get Public Profile
- [ ] Get existing user's profile
- [ ] Get non-existent user (error)
- [ ] Get profile while authenticated (shows follow status)
- [ ] Get profile while not authenticated (no follow status)
- [ ] Get own profile via /:id (works, public fields only)

### Get Own Profile
- [ ] Get own profile (includes email)
- [ ] Get without auth (error: UNAUTHORIZED)

### Update Profile
- [ ] Update displayName only
- [ ] Update bio only
- [ ] Update both fields
- [ ] Clear bio (empty string)
- [ ] Invalid displayName (too short/long)
- [ ] Invalid bio (too long)
- [ ] Update without auth (error)

### Profile Picture Upload
- [ ] Upload valid image
- [ ] Upload replaces existing (old deleted from Cloudinary)
- [ ] Upload invalid file type (error)
- [ ] Upload too large file (error)
- [ ] Upload without auth (error)
- [ ] Upload without file (error)

### Cover Image Upload
- [ ] Upload valid image
- [ ] Upload replaces existing
- [ ] Invalid file type (error)
- [ ] Too large file (error)

### Remove Profile Picture
- [ ] Remove existing picture
- [ ] Remove when no picture exists (still success)
- [ ] Remove without auth (error)

### Remove Cover Image
- [ ] Remove existing image
- [ ] Remove when no image exists (still success)

### Get Pinned Anchors
- [ ] User has pinned anchors
- [ ] User has no pinned anchors (empty array)
- [ ] View other user's pinned (only public/unlisted)
- [ ] View own pinned (all including private)
- [ ] Non-existent user (error)

---

## Cloudinary Folder Structure

```
profiles/
├── pictures/     # Profile pictures
│   ├── user123_abc.jpg
│   └── user456_def.png
└── covers/       # Cover images
    ├── user123_xyz.jpg
    └── user456_uvw.png
```

---

## Future Considerations

### Username Changes (Not in current scope)
- Complex: need uniqueness validation
- Need to update all references
- Consider adding later with rate limiting

### Profile Verification (Not in current scope)
- Verified badge for popular users
- Manual or automated verification

### Profile Privacy Settings (Not in current scope)
- Hide follower/following counts
- Private account option