# Anchor API Documentation

**Version:** 1.0  
**Base URL:** `/api/v1`  
**Authentication:** Bearer Token (JWT)

---

## Table of Contents

1. [Authentication](#1-authentication)
   - 2.9 [Delete Account](#29-delete-account)
   - 2.10 [Get by Username](#210-get-by-username)
   - 2.11 [Get User's Likes](#211-get-users-likes)
   - 2.12 [Get User's Clones](#212-get-users-clones)
3. [Anchors](#3-anchors)
   ...
   - 3.8 [Get Anchor Clones](#38-get-anchor-clones)
4. [Items](#4-items)
...
7. [Follows](#7-follows)
   - 7.1 [Follow/Unfollow User](#71-followunfollow-user)
   - 7.2 [Get Follow Status](#72-get-follow-status)
   - 7.3 [List Followers/Following](#73-list-followersfollowing)
   - 7.4 [Follow/Unfollow Anchor](#74-followunfollow-anchor)
8. [Feed](#8-feed)
...
12. [Safety & Moderation](#12-safety--moderation)
    - 12.1 [Create Report](#121-create-report)
    - 12.2 [Block User](#122-block-user)
    - 12.3 [Get Blocked Users](#123-get-blocked-users)
    - 12.4 [Unblock User](#124-unblock-user)
13. [Media](#13-media)
14. [Common Models](#14-common-models)

---

## 1. Authentication

### 1.1 Google Login

**Endpoint:** `POST /auth/google`  
**Authentication:** None  
**Description:** Authenticate user using Google ID token

**Request Body:**
```json
{
  "googleIdToken": "string (required)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "ObjectId",
      "googleId": "string",
      "email": "string",
      "username": "string",
      "displayName": "string",
      "bio": "string",
      "profilePictureUrl": "string",
      "coverImageUrl": "string",
      "followerCount": 0,
      "followingCount": 0,
      "anchorCount": 0,
      "isVerified": false,
      "joinedAt": "ISO8601",
      "createdAt": "ISO8601",
      "updatedAt": "ISO8601"
    },
    "accessToken": "string (JWT)",
    "refreshToken": "string (JWT)"
  }
}
```

---

### 1.2 Dev Login (Development Only)

**Endpoint:** `POST /auth/dev-login`  
**Authentication:** None  
**Description:** Development-only login bypassing Google OAuth

**Request Body:**
```json
{
  "email": "string (required, email format)",
  "displayName": "string (optional)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "token": "string (JWT - Access Token)",
    "refreshToken": "string (JWT)",
    "user": { /* User object */ },
    "isNewUser": true,
    "requiresUsername": true
  }
}
```

---

### 1.3 Refresh Token

**Endpoint:** `POST /auth/refresh`
**Authentication:** None (relies on refresh token in body)
**Description:** Get a new access token using a valid refresh token.

**Request Body:**
```json
{
  "refreshToken": "string (required)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "accessToken": "string (New Access Token)",
    "refreshToken": "string (Existing or Rotated Refresh Token)",
    "user": { /* User object (optional) */ }
  }
}
```

**Errors:**
- `401` - Invalid or revoked refresh token

---

### 1.4 Logout

**Endpoint:** `POST /auth/logout`
**Authentication:** None (relies on refresh token in body)
**Description:** Revoke a specific refresh token.

**Request Body:**
```json
{
  "refreshToken": "string (required)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "Logged out successfully"
}
```

---

### 1.5 Revoke All Sessions

**Endpoint:** `POST /auth/revoke-all`
**Authentication:** Required (Access Token)
**Description:** Revoke all refresh tokens for the current user (Logout from all devices).

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "All sessions revoked"
}
```

---

### 1.6 Update Username

**Endpoint:** `PATCH /auth/username`  
**Authentication:** Required  
**Description:** Change username (can only be done once)

**Request Body:**
```json
{
  "username": "string (required, 3-20 chars)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": { /* Updated User object */ }
}
```

- `400` - Username already taken or already changed
- `401` - Unauthorized

---

### 1.7 Check Username Availability

**Endpoint:** `GET /auth/username/check`
**Authentication:** None
**Description:** Check if a username is available. Returns 200 OK for both available and unavailable cases.

**Query Parameters:**
- `username` - Username to check (required, 3-20 chars)

**Response (Available):** `200 OK`
```json
{
  "success": true,
  "data": {
    "username": "example",
    "available": true
  }
}
```

**Response (Unavailable):** `200 OK`
```json
{
  "success": true,
  "data": {
    "username": "admin",
    "available": false,
    "reason": "This username is reserved"
  }
}
```

---

## 2. User Management

### 2.1 Get Own Profile

**Endpoint:** `GET /users/me`  
**Authentication:** Required  
**Description:** Get authenticated user's complete profile

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "id": "ObjectId",
    "googleId": "string",
    "email": "string",
    "username": "string",
    "displayName": "string",
    "bio": "string",
    "profilePictureUrl": "string",
    "coverImageUrl": "string",
    "followerCount": 0,
    "followingCount": 0,
    "anchorCount": 0,
    "isVerified": false,
    "joinedAt": "ISO8601",
    "createdAt": "ISO8601",
    "updatedAt": "ISO8601"
  }
}
```

---

### 2.2 Get Public Profile

**Endpoint:** `GET /users/{id}`  
**Authentication:** Optional  
**Description:** Get user's public profile by ID

**Path Parameters:**
- `id` - User ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "id": "ObjectId",
    "username": "string",
    "displayName": "string",
    "bio": "string",
    "profilePictureUrl": "string",
    "coverImageUrl": "string",
    "followerCount": 0,
    "followingCount": 0,
    "anchorCount": 0,
    "isVerified": false,
    "joinedAt": "ISO8601",
    "isFollowing": false,
    "isFollowedBy": false,
    "isMutual": false
  }
}
```

---

### 2.3 Update Profile

**Endpoint:** `PATCH /users/me`  
**Authentication:** Required  
**Description:** Update display name and/or bio

**Request Body:**
```json
{
  "displayName": "string (optional, 2-50 chars)",
  "bio": "string (optional, max 200 chars)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": { /* Updated OwnProfileResponse */ }
}
```

---

### 2.4 Upload Profile Picture

**Endpoint:** `POST /users/me/profile-picture`  
**Authentication:** Required  
**Description:** Upload profile picture

**Request:** `multipart/form-data`
- `file` - Image file (required)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "profilePictureUrl": "string (Cloudinary URL)"
  }
}
```

---

### 2.5 Upload Cover Image

**Endpoint:** `POST /users/me/cover-image`  
**Authentication:** Required  
**Description:** Upload cover image

**Request:** `multipart/form-data`
- `file` - Image file (required)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "coverImageUrl": "string (Cloudinary URL)"
  }
}
```

---

### 2.6 Remove Profile Picture

**Endpoint:** `DELETE /users/me/profile-picture`  
**Authentication:** Required  
**Description:** Remove profile picture

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "Profile picture removed"
}
```

---

### 2.7 Remove Cover Image

**Endpoint:** `DELETE /users/me/cover-image`  
**Authentication:** Required  
**Description:** Remove cover image

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "Cover image removed"
}
```

---

### 2.8 Get Pinned Anchors

**Endpoint:** `GET /users/{id}/pinned`  
**Authentication:** Optional  
**Description:** Get user's pinned anchors

**Path Parameters:**
- `id` - User ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": [
    {
      "id": "ObjectId",
      "title": "string",
      "description": "string",
      "coverMediaType": "icon|emoji|image",
      "coverMediaValue": "string",
      "visibility": "public|unlisted|private",
      "itemCount": 0,
      "likeCount": 0,
      "cloneCount": 0,
      "createdAt": "ISO8601"
    }
  ]
}
```

---

### 2.9 Delete Account

**Endpoint:** `DELETE /users/me`  
**Authentication:** Required  
**Description:** Permanently delete account and all associated data

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "Account deleted successfully"
}
```

> - Profile and cover images

---

### 2.10 Get by Username

**Endpoint:** `GET /users/username/{username}`
**Authentication:** Optional
**Description:** Get user profile by username. Includes follow status if viewer is authenticated.

**Path Parameters:**
- `username` - Username (required)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "id": "ObjectId",
    "username": "string",
    "displayName": "string",
    "bio": "string",
    "profilePictureUrl": "string",
    "coverImageUrl": "string",
    "followerCount": 0,
    "followingCount": 0,
    "anchorCount": 0,
    "isVerified": false,
    "joinedAt": "ISO8601",
    "isFollowing": false,
    "isFollowedBy": false,
    "isMutual": false
  }
}
```

---

### 2.11 Get User's Likes

**Endpoint:** `GET /users/{id}/likes`
**Authentication:** Optional (unless using 'me')
**Description:** Get paginated list of anchors liked by a user. Use 'me' as id for current user.

**Path Parameters:**
- `id` - User ID or 'me'

**Query Parameters:**
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [
      {
        "id": "ObjectId",
        "title": "string",
        "description": "string",
        "itemCount": 5,
        "likeCount": 10,
        "likedAt": "ISO8601",
        "author": {
          "id": "ObjectId",
          "username": "string",
          "displayName": "string",
          "profilePictureUrl": "string"
        }
      }
    ],
    "pagination": { /* Pagination object */ }
  }
}
```

---

### 2.12 Get User's Clones

**Endpoint:** `GET /users/{id}/clones`
**Authentication:** Optional (unless using 'me')
**Description:** Get paginated list of anchors cloned by a user. Use 'me' as id for current user.

**Path Parameters:**
- `id` - User ID or 'me'

**Query Parameters:**
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [
      {
        "id": "ObjectId",
        "title": "string",
        "description": "string",
        "itemCount": 5,
        "likeCount": 10,
        "clonedAt": "ISO8601",
        "originalAuthor": {
          "id": "ObjectId",
          "username": "string",
          "displayName": "string"
        }
      }
    ],
    "pagination": { /* Pagination object */ }
  }
}
```

---

## 3. Anchors

### 3.1 Create Anchor

**Endpoint:** `POST /anchors`  
**Authentication:** Required  
**Description:** Create a new anchor

**Request Body:**
```json
{
  "title": "string (required, 3-100 chars)",
  "description": "string (optional, max 500 chars)",
  "coverMediaType": "icon|emoji|image (optional)",
  "coverMediaValue": "string (optional)",
  "visibility": "private|unlisted|public (optional, default: public)",
  "tags": ["string"] // optional, max 5 tags, each 3-20 chars. Normalized to lowercase.
}
```

**Response:** `201 Created`
```json
{
  "success": true,
  "data": {
    "id": "ObjectId",
    "userId": "ObjectId",
    "title": "string",
    "description": "string",
    "coverMediaType": "string",
    "coverMediaValue": "string",
    "visibility": "public",
    "isPinned": false,
    "tags": ["string"],
    "likeCount": 0,
    "cloneCount": 0,
    "commentCount": 0,
    "viewCount": 0,
    "itemCount": 0,
    "engagementScore": 0,
    "createdAt": "ISO8601",
    "updatedAt": "ISO8601",
    "lastItemAddedAt": "ISO8601"
  }
}
```

---

### 3.2 Get Anchor

**Endpoint:** `GET /anchors/{id}`  
**Authentication:** Optional  
**Description:** Get anchor details with items

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "anchor": { /* Anchor object */ },
    "items": [
      {
        "id": "ObjectId",
        "anchorId": "ObjectId",
        "type": "url|image|audio|file|text",
        "position": 0,
        "urlData": { /* if type=url */ },
        "imageData": { /* if type=image */ },
        "audioData": { /* if type=audio */ },
        "fileData": { /* if type=file */ },
        "textData": { /* if type=text */ },
        "createdAt": "ISO8601",
        "updatedAt": "ISO8601"
      }
    ],
    "likeSummary": { /* Like summary if available */ }
  }
}
```

---

### 3.3 Update Anchor

**Endpoint:** `PATCH /anchors/{id}`  
**Authentication:** Required  
**Description:** Update anchor details

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Request Body:**
```json
{
  "title": "string (optional, 3-100 chars)",
  "description": "string (optional, max 500 chars)",
  "coverMediaType": "icon|emoji|image (optional)",
  "coverMediaValue": "string (optional)",
  "visibility": "private|unlisted|public (optional)",
  "tags": ["string"] // optional, max 5 tags. Normalized to lowercase.
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": { /* Updated Anchor object */ }
}
```

**Errors:**
- `403` - Not anchor owner
- `404` - Anchor not found

---

### 3.4 Delete Anchor

**Endpoint:** `DELETE /anchors/{id}`  
**Authentication:** Required  
**Description:** Soft delete an anchor

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "Anchor deleted successfully"
}
```

---

### 3.5 List User Anchors

**Endpoint:** `GET /anchors`  
**Authentication:** Optional  
**Description:** List anchors for a user

**Query Parameters:**
- `userId` - User ID (required)
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [ /* Array of Anchor objects */ ],
    "pagination": {
      "page": 1,
      "limit": 20,
      "total": 100,
      "totalPages": 5,
      "hasMore": true
    }
  }
}
```

---

### 3.6 Toggle Pin

**Endpoint:** `PATCH /anchors/{id}/pin`  
**Authentication:** Required  
**Description:** Toggle pinned status (max 3 pinned anchors)

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "isPinned": true
  }
}
```

**Errors:**
- `400` - Maximum 3 anchors can be pinned

---

### 3.7 Clone Anchor

**Endpoint:** `POST /anchors/{id}/clone`  
**Authentication:** Required  
**Description:** Create a copy of an anchor

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Response:** `201 Created`
```json
{
  "success": true,
  "data": { /* Cloned Anchor object */ }
}
```

---

---

### 3.8 Get Anchor Clones

**Endpoint:** `GET /anchors/{id}/clones`  
**Authentication:** Optional  
**Description:** Get paginated list of clones for an anchor

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Query Parameters:**
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [
      {
        "id": "ObjectId",
        "title": "string",
        "createdAt": "ISO8601",
        "cloner": {
          "id": "ObjectId",
          "username": "string",
          "displayName": "string",
          "profilePictureUrl": "string"
        }
      }
    ],
    "pagination": {
      "page": 1,
      "limit": 20,
      "total": 5,
      "totalPages": 1,
      "hasMore": false
    }
  }
}
```

---

## 4. Items

### 4.1 List Anchor Items

**Endpoint:** `GET /anchors/{id}/items`  
**Authentication:** Optional  
**Description:** Get paginated items for an anchor

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Query Parameters:**
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [ /* Array of Item objects */ ],
    "pagination": {
      "page": 1,
      "limit": 20,
      "total": 50,
      "totalPages": 3,
      "hasMore": true
    }
  }
}
```

---

### 4.2 Add Item

**Endpoint:** `POST /anchors/{id}/items`  
**Authentication:** Required  
**Description:** Add a new item to an anchor

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Request Body:**
```json
{
  "type": "url|image|audio|file|text (required)",
  "url": "string (required if type=url)",
  "content": "string (required if type=text, max 10000 chars)"
}
```

**Response:** `201 Created`
```json
{
  "success": true,
  "data": { /* Created Item object */ }
}
```

---

### 4.3 Upload Item

**Endpoint:** `POST /anchors/{id}/items/upload`  
**Authentication:** Required  
**Description:** Upload a file as an item

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Request:** `multipart/form-data`
- `file` - File to upload (required)
- `type` - Item type: `image|audio|file` (required)

**Response:** `201 Created`
```json
{
  "success": true,
  "data": { /* Created Item object with file data */ }
}
```

---

### 4.4 Delete Item

**Endpoint:** `DELETE /items/{id}`  
**Authentication:** Required  
**Description:** Delete an item

**Path Parameters:**
- `id` - Item ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "Item deleted successfully"
}
```

---

### 4.5 Reorder Items

**Endpoint:** `PATCH /anchors/{id}/items/reorder`  
**Authentication:** Required  
**Description:** Update the order of items

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Request Body:**
```json
{
  "itemIds": ["ObjectId", "ObjectId", ...] // required, min 1
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "Items reordered successfully"
}
```

---

## 5. Likes

### 5.1 Like/Unlike Anchor

**Endpoint:** `POST /anchors/{id}/like`  
**Authentication:** Required  
**Description:** Toggle like on an anchor

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Request Body:**
```json
{
  "action": "like|unlike (required)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "liked": true,
    "likeCount": 42
  }
}
```

---

### 5.2 Get Like Status

**Endpoint:** `GET /anchors/{id}/like/status`  
**Authentication:** Required  
**Description:** Check if user has liked an anchor

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "liked": true
  }
}
```

---

### 5.3 List Likers

**Endpoint:** `GET /anchors/{id}/likes`  
**Authentication:** Optional  
**Description:** Get paginated list of users who liked the anchor

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Query Parameters:**
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [
      {
        "id": "ObjectId",
        "username": "string",
        "displayName": "string",
        "profilePictureUrl": "string",
        "isFollowing": false,
        "likedAt": "ISO8601"
      }
    ],
    "pagination": { /* Pagination object */ }
  }
}
```

---

### 5.4 Get Like Summary

**Endpoint:** `GET /anchors/{id}/like/summary`  
**Authentication:** Optional  
**Description:** Get like summary prioritizing followed users

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "totalLikes": 42,
    "topLikers": [
      {
        "id": "ObjectId",
        "username": "string",
        "displayName": "string",
        "profilePictureUrl": "string"
      }
    ],
    "hasMore": true
  }
}
```

---

## 6. Comments

### 6.1 Add Comment

**Endpoint:** `POST /anchors/{id}/comments`  
**Authentication:** Required  
**Description:** Add a comment to an anchor (supports @mentions)

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Request Body:**
```json
{
  "content": "string (required, 1-1000 chars)"
}
```

**Response:** `201 Created`
```json
{
  "success": true,
  "data": {
    "id": "ObjectId",
    "anchorId": "ObjectId",
    "userId": "ObjectId",
    "content": "string",
    "mentions": ["ObjectId"],
    "likeCount": 0,
    "hasLiked": false,
    "createdAt": "ISO8601",
    "updatedAt": "ISO8601",
    "author": {
      "id": "ObjectId",
      "username": "string",
      "displayName": "string",
      "profilePictureUrl": "string"
    }
  }
}
```

---

### 6.2 List Comments

**Endpoint:** `GET /anchors/{id}/comments`  
**Authentication:** Optional  
**Description:** Get paginated comments for an anchor

**Path Parameters:**
- `id` - Anchor ID (ObjectId)

**Query Parameters:**
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)
- `sortBy` - Sort order: `recent|popular` (default: recent)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [ /* Array of Comment objects */ ],
    "pagination": { /* Pagination object */ }
  }
}
```

---

### 6.3 Get Comment

**Endpoint:** `GET /comments/{id}`  
**Authentication:** Optional  
**Description:** Get single comment by ID

**Path Parameters:**
- `id` - Comment ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": { /* Comment object */ }
}
```

---

### 6.4 Edit Comment

**Endpoint:** `PATCH /comments/{id}`  
**Authentication:** Required  
**Description:** Edit own comment

**Path Parameters:**
- `id` - Comment ID (ObjectId)

**Request Body:**
```json
{
  "content": "string (required, 1-1000 chars)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": { /* Updated Comment object */ }
}
```

---

### 6.5 Delete Comment

**Endpoint:** `DELETE /comments/{id}`  
**Authentication:** Required  
**Description:** Delete own comment or any comment on own anchor

**Path Parameters:**
- `id` - Comment ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "Comment deleted"
}
```

---

### 6.6 Like/Unlike Comment

**Endpoint:** `POST /comments/{id}/like`  
**Authentication:** Required  
**Description:** Toggle like on a comment

**Path Parameters:**
- `id` - Comment ID (ObjectId)

**Request Body:**
```json
{
  "action": "like|unlike (required)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "liked": true,
    "likeCount": 5
  }
}
```

---

### 6.7 Get Comment Like Status

**Endpoint:** `GET /comments/{id}/like/status`  
**Authentication:** Required  
**Description:** Check if user has liked a comment

**Path Parameters:**
- `id` - Comment ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "liked": true
  }
}
```

---

## 7. Follows

### 7.1 Follow/Unfollow User

**Endpoint:** `POST /users/{id}/follow`  
**Authentication:** Required  
**Description:** Follow or unfollow a user

**Path Parameters:**
- `id` - Target user ID (ObjectId)

**Request Body:**
```json
{
  "action": "follow|unfollow (required)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "following": true
  }
}
```

---

### 7.2 Get Follow Status

**Endpoint:** `GET /users/{id}/follow/status`  
**Authentication:** Required  
**Description:** Check follow relationship with a user

**Path Parameters:**
- `id` - Target user ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "isFollowing": true,
    "isFollowedBy": false,
    "isMutual": false
  }
}
```

---

### 7.3 List Followers/Following

**Endpoint:** `GET /users/{id}/follows`  
**Authentication:** Optional  
**Description:** Get paginated list of followers or following

**Path Parameters:**
- `id` - User ID (ObjectId)

**Query Parameters:**
- `type` - `followers|following` (required)
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [
      {
        "id": "ObjectId",
        "username": "string",
        "displayName": "string",
        "profilePictureUrl": "string",
        "bio": "string",
        "isFollowing": false,
        "isFollowedBy": false,
        "isMutual": false
      }
    ],
    "pagination": { /* Pagination object */ }
  }
}
```

---

```

---

### 7.4 Anchor Follows

#### 7.4.1 Follow/Unfollow Anchor
**Endpoint:** `POST /anchors/{id}/follow`  
**Authentication:** Required  
**Description:** Follow or unfollow an anchor to track updates

**Request Body:**
```json
{
  "action": "follow|unfollow (required)",
  "notifyOnUpdate": "boolean (optional, default: false)"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "isFollowing": true,
    "notifyOnUpdate": true,
    "followerCount": 42
  }
}
```

#### 7.4.2 Get Anchor Follow Status
**Endpoint:** `GET /anchors/{id}/follow/status`  
**Authentication:** Required  
**Description:** Check if current user is following an anchor and get update status

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "isFollowing": true,
    "notifyOnUpdate": true,
    "hasUpdates": true,
    "updatesSinceLastSeen": 3,
    "lastSeenVersion": 5,
    "currentVersion": 8,
    "followedAt": "ISO8601"
  }
}
```

#### 7.4.3 Toggle Anchor Notifications
**Endpoint:** `PATCH /anchors/{id}/follow/notifications`  
**Authentication:** Required  
**Description:** Enable or disable notifications for anchor updates

**Request Body:**
```json
{
  "notifyOnUpdate": "boolean (required)"
}
```

#### 7.4.4 List Following Anchors
**Endpoint:** `GET /users/me/following-anchors`  
**Authentication:** Required  
**Description:** Get all anchors the current user is following

**Query Parameters:**
- `page`, `limit` - Pagination
- `sort` - `recent|updated|alphabetical`
- `hasUpdates` - `true|false` (filter only anchors with updates)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [ /* Array of FollowingAnchorItem */ ],
    "pagination": { ... },
    "meta": {
      "totalWithUpdates": 5,
      "sort": "recent"
    }
  }
}
```

---

## 8. Feed

### 8.1 Get Home Feed

**Endpoint:** `GET /feed/following`  
**Authentication:** Required  
**Description:** Get personalized home feed with following sections and suggestions

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "followingAnchors": {
      "items": [
        {
          "id": "ObjectId",
          "title": "string",
          "coverMediaType": "string",
          "coverMediaValue": "string",
          "hasUpdate": true,
          "lastSeenVersion": 5,
          "currentVersion": 8
        }
      ],
      "totalCount": 12
    },
    "suggestedCategories": [
      { "name": "Technology", "anchorCount": 150 }
    ],
    "feed": {
      "items": [ /* Standard feed anchors */ ],
      "pagination": {
        "limit": 20,
        "hasMore": true,
        "nextCursor": "string",
        "itemCount": 20
      },
      "meta": {
        "feedType": "following",
        "totalFollowing": 25
      }
    }
  }
}
```

---

### 8.2 Get Discovery Feed

**Endpoint:** `GET /feed/discover`  
**Authentication:** Optional  
**Description:** Get trending/popular public anchors

**Query Parameters:**
- `limit` - Items per page (default: 20, max: 50)
- `cursor` - Pagination cursor (optional)
- `category` - `trending|popular|recent` (default: trending)
- `tag` - Filter by tag (optional)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "anchors": [ /* Array of Anchor objects */ ],
    "nextCursor": "string|null",
    "hasMore": true
  }
}
```

---

### 8.3 Get Tag Feed

**Endpoint:** `GET /feed/tags/{tagName}`  
**Authentication:** Optional  
**Description:** Get public anchors for a specific tag

**Path Parameters:**
- `tagName` - Tag name (string)

**Query Parameters:**
- `limit` - Items per page (default: 20, max: 50)
- `cursor` - Pagination cursor (optional)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "anchors": [ /* Array of Anchor objects */ ],
    "nextCursor": "string|null",
    "hasMore": true
  }
}
```

---

## 9. Search

### 9.1 Unified Search

**Endpoint:** `GET /search`  
**Authentication:** Optional  
**Description:** Search across anchors and users

**Query Parameters:**
- `q` - Search query (required, min 2 chars)
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "anchors": [ /* Array of Anchor search results */ ],
    "users": [ /* Array of User search results */ ],
    "pagination": { /* Pagination object */ }
  }
}
```

---

### 9.2 Combined Search (Type-ahead)

**Endpoint:** `GET /search/combined`  
**Authentication:** Optional  
**Description:** Aggregate top results for type-ahead

**Query Parameters:**
- `q` - Search query (required, min 2 chars)
- `limit` - Results per type (default: 3, max: 10)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "top_anchors": [ /* Top 3 anchor matches */ ],
    "top_users": [ /* Top 3 user matches */ ],
    "top_tags": [ /* Top 3 tag matches */ ]
  }
}
```

---

### 9.3 Search Anchors

**Endpoint:** `GET /search/anchors`  
**Authentication:** Optional  
**Description:** Search anchors with filters

**Query Parameters:**
- `q` - Search query (required, min 2 chars)
- `tag` - Filter by tag (optional)
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [ /* Array of Anchor search results */ ],
    "pagination": { /* Pagination object */ }
  }
}
```

---

### 9.4 Search Users

**Endpoint:** `GET /search/users`  
**Authentication:** Optional  
**Description:** Search users with pagination

**Query Parameters:**
- `q` - Search query (required, min 2 chars)
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "data": [ /* Array of User search results */ ],
    "pagination": { /* Pagination object */ }
  }
}
```

---

### 9.5 Search Tags (Autocomplete)

**Endpoint:** `GET /search/tags`  
**Authentication:** Optional  
**Description:** Get tag suggestions based on prefix

**Query Parameters:**
- `q` - Tag prefix (required, min 1 char)
- `limit` - Max suggestions (default: 10, max: 20)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "tags": [
      {
        "name": "string",
        "count": 42
      }
    ]
  }
}
```

---

## 10. Notifications

### 10.1 List Notifications

**Endpoint:** `GET /notifications`  
**Authentication:** Required  
**Description:** Get paginated notifications

**Query Parameters:**
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 20, max: 50)
- `unreadOnly` - Show only unread (default: false)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "notifications": [
      {
        "id": "ObjectId",
        "type": "like|comment|follow|clone|mention|anchor_update",
        "resourceType": "anchor|user|comment",
        "resourceId": "ObjectId",
        "anchorId": "ObjectId|null",
        "preview": "string",
        "isRead": false,
        "createdAt": "ISO8601",
        "actor": {
          "id": "ObjectId",
          "username": "string",
          "displayName": "string",
          "profilePicture": "string|null"
        },
        "anchor": {
          "id": "ObjectId",
          "title": "string"
        }
      }
    ],
    "pagination": { /* Pagination object */ }
  }
}
```

---

### 10.2 Get Unread Count

**Endpoint:** `GET /notifications/unread-count`  
**Authentication:** Required  
**Description:** Get count of unread notifications

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "unreadCount": 5
  }
}
```

---

### 10.3 Mark as Read

**Endpoint:** `PATCH /notifications/{id}/read`  
**Authentication:** Required  
**Description:** Mark a notification as read

**Path Parameters:**
- `id` - Notification ID (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "id": "ObjectId",
    "isRead": true
  }
}
```

---

### 10.4 Mark All as Read

**Endpoint:** `PATCH /notifications/read-all`  
**Authentication:** Required  
**Description:** Mark all notifications as read

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "markedCount": 10
  }
}
```

---

## 11. Interests

### 11.1 Get Suggested Interests

**Endpoint:** `GET /interests/suggested`  
**Authentication:** Optional  
**Description:** Get personalized or popular interest suggestions

**Query Parameters:**
- `limit` - Max suggestions (default: 10, max: 20)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "categories": [
      {
        "name": "Photography",
        "slug": "photography",
        "icon": "📷",
        "count": 245,
        "score": 245
      }
    ],
    "basedOn": "personalized|popular"
  }
}
```

**Logic:**
- **New/Unauthenticated users:** Returns popular tags across platform
- **Authenticated users:** Returns personalized tags based on:
  - User's own anchors (weighted 3x)
  - Liked anchors (weighted 2x)
  - Falls back to popular if no activity

---

### 11.2 Save User Interests

**Endpoint:** `POST /users/me/interests`  
**Authentication:** Required  
**Description:** Save interest tags for user

**Request Body:**
```json
{
  "tags": ["string"] // required, 1-10 tags, each 2-30 chars
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "Interests saved successfully"
}
```

---

## 12. Safety & Moderation

### 12.1 Create Report

**Endpoint:** `POST /reports`  
**Authentication:** Required  
**Description:** Report content or user

**Request Body:**
```json
{
  "targetId": "ObjectId (required)",
  "targetType": "anchor|user|comment (required)",
  "reason": "spam|harassment|inappropriate|other (required)"
}
```

**Response:** `201 Created`
```json
{
  "success": true,
  "message": "Report submitted"
}
```

---

### 12.2 Block User

**Endpoint:** `POST /users/{id}/block`  
**Authentication:** Required  
**Description:** Block a user

**Path Parameters:**
- `id` - User ID to block (ObjectId)

**Response:** `200 OK`
```json
{
  "success": true,
  "message": "User blocked successfully"
}
```

---

### 12.3 Get Blocked Users

**Endpoint:** `GET /users/me/blocks`  
**Authentication:** Required  
**Description:** Get list of blocked users

**Response:** `200 OK`
```json
{
  "success": true,
  "data": [
    {
      "id": "ObjectId",
      "username": "string",
      "displayName": "string",
      "profilePictureUrl": "string"
    }
  ]
}
```

---

### 12.4 Unblock User

**Endpoint:** `DELETE /users/{id}/block`
**Authentication:** Required
**Description:** Unblock a user.

**Path Parameters:**
- `id` - User ID to unblock

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "message": "User unblocked successfully"
  }
}
```

## 13. Media

### 13.1 Upload Media

**Endpoint:** `POST /media/upload`  
**Authentication:** Optional  
**Description:** Upload file to Cloudinary

**Request:** `multipart/form-data`
- `file` - File to upload (required)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "publicId": "string",
    "url": "string (Cloudinary URL)",
    "secureUrl": "string",
    "format": "string",
    "resourceType": "image|video|raw",
    "width": 1920,
    "height": 1080,
    "bytes": 123456
  }
}
```

**Supported Types:**
- **Images:** JPG, PNG, GIF, WebP (max 10MB)
- **Audio:** MP3, WAV, M4A (max 20MB)
- **Files:** PDF, DOC, etc. (max 10MB)

---

### 13.2 Get Link Preview

**Endpoint:** `GET /media/preview`  
**Authentication:** Optional  
**Description:** Get metadata for a URL

**Query Parameters:**
- `url` - URL to preview (required)

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "originalUrl": "string",
    "title": "string",
    "description": "string",
    "favicon": "string",
    "thumbnail": "string"
  }
}
```

---

## 14. Common Models

### 14.1 Item Type-Specific Data

#### URLData
```json
{
  "originalUrl": "string",
  "title": "string",
  "description": "string",
  "favicon": "string",
  "thumbnail": "string"
}
```

#### ImageData
```json
{
  "cloudinaryUrl": "string",
  "publicId": "string",
  "width": 1920,
  "height": 1080,
  "fileSize": 123456
}
```

#### AudioData
```json
{
  "cloudinaryUrl": "string",
  "publicId": "string",
  "duration": 180,
  "fileSize": 5242880
}
```

#### FileData
```json
{
  "cloudinaryUrl": "string",
  "publicId": "string",
  "filename": "document.pdf",
  "fileType": "application/pdf",
  "fileSize": 1048576
}
```

#### TextData
```json
{
  "content": "string"
}
```

---

### 14.2 Pagination Object

```json
{
  "page": 1,
  "limit": 20,
  "total": 100,
  "totalPages": 5,
  "hasMore": true
}
```

---

### 14.3 Error Response

```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message"
  }
}
```

**Common Error Codes:**
- `AUTH_FAILED` - Authentication required or invalid token
- `INVALID_JSON` - Malformed request body
- `INVALID_ID` - Invalid ObjectId format
- `NOT_FOUND` - Resource not found
- `FORBIDDEN` - Insufficient permissions
- `VALIDATION_ERROR` - Request validation failed
- `DATABASE_ERROR` - Database operation failed
- `UPLOAD_FAILED` - File upload failed

---

### 14.4 HTTP Status Codes

| Code | Meaning |
|------|---------|
| `200` | OK - Request successful |
| `201` | Created - Resource created successfully |
| `400` | Bad Request - Invalid input |
| `401` | Unauthorized - Authentication required |
| `403` | Forbidden - Insufficient permissions |
| `404` | Not Found - Resource not found |
| `409` | Conflict - Resource already exists |
| `422` | Unprocessable Entity - Validation error |
| `500` | Internal Server Error - Server error |

---

## Authentication

All authenticated endpoints require a Bearer token in the Authorization header:

```
Authorization: Bearer <JWT_TOKEN>
```

The JWT token is obtained from the `/auth/google` or `/auth/dev-login` endpoints.

**Token Expiration:** Tokens expire after the configured duration (default: 168 hours / 7 days).

---

## Rate Limiting

> **Note:** Rate limiting is not currently implemented but should be added before production deployment.

**Recommended Limits:**
- Authentication endpoints: 5 requests/minute
- Write operations: 60 requests/minute
- Read operations: 120 requests/minute

---

## Pagination Strategies

### Offset-Based Pagination
Used for most list endpoints (anchors, users, comments, etc.)

**Parameters:**
- `page` - Page number (1-indexed)
- `limit` - Items per page

**Example:** `GET /anchors?userId=123&page=2&limit=20`

### Cursor-Based Pagination
Used for feeds (following, discover, tag feeds)

**Parameters:**
- `cursor` - Opaque cursor string
- `limit` - Items per page

**Example:** `GET /feed/discover?cursor=abc123&limit=20`

**Response includes:**
- `nextCursor` - Cursor for next page (null if no more)
- `hasMore` - Boolean indicating more results

---

## Visibility & Access Control

### Anchor Visibility

| Visibility | Owner | Authenticated Users | Public |
|------------|-------|---------------------|--------|
| `private` | ✅ View, Edit, Delete | ❌ | ❌ |
| `unlisted` | ✅ View, Edit, Delete | ✅ View (with link) | ✅ View (with link) |
| `public` | ✅ View, Edit, Delete | ✅ View | ✅ View |

### Comment Permissions

- **Add:** Any authenticated user (if anchor is accessible)
- **Edit:** Comment author only
- **Delete:** Comment author OR anchor owner

---

## Notifications

### Notification Types

| Type | Trigger | Resource Type |
|------|---------|---------------|
| `like` | User likes anchor | `anchor` |
| `comment` | User comments on anchor | `anchor` |
| `mention` | User mentioned in comment | `comment` |
| `follow` | User follows you | `user` |
| `clone` | User clones your anchor | `anchor` |

**Note:** Users do not receive notifications for their own actions.

---

## Tags

- **Format:** Lowercase, alphanumeric with hyphens
- **Length:** 3-20 characters
- **Max per anchor:** 5 tags
- **Searchable:** Yes, with autocomplete support

---

## File Upload Limits

| Type | Max Size | Formats |
|------|----------|---------|
| Profile Picture | 10 MB | JPG, PNG, GIF, WebP |
| Cover Image | 10 MB | JPG, PNG, GIF, WebP |
| Anchor Item Image | 10 MB | JPG, PNG, GIF, WebP |
| Anchor Item Audio | 20 MB | MP3, WAV, M4A |
| Anchor Item File | 10 MB | PDF, DOC, DOCX, etc. |

---

## Best Practices

1. **Always validate input** on the client side before sending requests
2. **Handle pagination** properly to avoid loading too much data
3. **Cache user profiles** and anchor data when possible
4. **Use cursor-based pagination** for feeds to ensure consistency
5. **Implement retry logic** for failed uploads
6. **Show loading states** during API calls
7. **Handle 401 errors** by redirecting to login
8. **Implement optimistic updates** for likes and follows
9. **Debounce search queries** to reduce API calls
10. **Use WebSockets** for real-time notifications (future enhancement)

---

## Changelog

### Version 1.0 (Current)
- Initial API release
- Authentication via Google OAuth
- Anchor CRUD operations
- Item management
- Likes, comments, follows
- Feed (following, discover, tags)
- Search (unified, combined, type-ahead)
- Notifications
- Interests/Categories
- Safety & moderation
- Media upload

---

## Support

For API support or bug reports, please contact the development team or file an issue in the repository.

**Base URL:** `/api/v1`  
**Documentation Version:** 1.0  
**Last Updated:** 2026-01-10
