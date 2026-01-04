# Anchors Feature Specification

## Overview
Anchors are collections where users organize content (URLs, images, audio, files, text).

## Anchor Model Fields
- id (ObjectID)
- userId (string, indexed) - owner
- title (string, 3-100 chars)
- description (string, 0-500 chars, optional)
- coverMediaType (enum: icon, emoji, image)
- coverMediaValue (string) - emoji/icon name or image URL
- visibility (enum: private, unlisted, public, indexed)
- isPinned (boolean, max 3 per user)
- tags ([]string, max 5 tags, indexed)
- clonedFromAnchorId (ObjectID, nullable)
- clonedFromUserId (string, nullable)
- likeCount (int, default 0)
- cloneCount (int, default 0)
- commentCount (int, default 0)
- viewCount (int, default 0)
- itemCount (int, default 0) - cached count of items
- createdAt (timestamp)
- updatedAt (timestamp)
- lastItemAddedAt (timestamp) - for feed sorting
- deletedAt (timestamp, nullable) - soft delete

## Item Model Fields
- id (ObjectID)
- anchorId (ObjectID, indexed)
- type (enum: url, image, audio, file, text)
- position (int) - for ordering (0, 1, 2, 3...)
- createdAt (timestamp)

### Type-Specific Data (stored as embedded documents):
- urlData: {originalUrl, title, description, favicon, thumbnail}
- imageData: {cloudinaryUrl, publicId, width, height, fileSize}
- audioData: {cloudinaryUrl, publicId, duration, fileSize}
- fileData: {cloudinaryUrl, publicId, filename, fileType, fileSize}
- textData: {content (markdown)}

## API Endpoints

### Anchor CRUD
- POST /anchors - Create anchor
- GET /anchors/:id - Get anchor by ID
- GET /anchors - List user's anchors (with filters)
- PATCH /anchors/:id - Update anchor
- DELETE /anchors/:id - Soft delete anchor
- PATCH /anchors/:id/pin - Toggle pin status

### Item CRUD
- POST /anchors/:id/items - Add item to anchor
- GET /anchors/:id/items - List anchor's items
- DELETE /anchors/:id/items/:itemId - Remove item
- PATCH /anchors/:id/items/reorder - Reorder items

## Validation Rules
- Anchor title: 3-100 characters
- Description: 0-500 characters
- Tags: 0-5 tags, each 3-20 chars, lowercase
- Max 3 pinned anchors per user
- Max 100 items per anchor
- Image max: 10MB
- Audio max: 25MB, 10 minutes
- File max: 50MB

## Business Rules
- Only owner can edit/delete anchor
- Private anchors not visible to others
- Unlisted anchors accessible via link only
- Public anchors visible in feeds and search
- Pinned anchors must be public or unlisted
- Cannot pin private anchors
- Deleting anchor soft-deletes (30 days recovery)
```

**Save and exit** (Ctrl+X, Y, Enter)

---

## 🎯 **STEP 1: Anchor & Item Models**

### **Prompt for Antigravity:**
```
CONTEXT:
Phase 2 of Anchor app - Building the Anchors feature (core content system).

Reference: Read ANCHORS_SPEC.md for full requirements.
Architecture: Continue following ARCHITECTURE.md and STANDARDS.md.

TASK: Create Anchor and Item Models

Create: internal/features/anchors/model.go

Requirements:

1. Anchor struct with these fields (camelCase bson/json):
   - id (primitive.ObjectID)
   - userId (string, indexed)
   - title (string)
   - description (string)
   - coverMediaType (string) - "icon", "emoji", or "image"
   - coverMediaValue (string)
   - visibility (string) - "private", "unlisted", "public"
   - isPinned (bool)
   - tags ([]string)
   - clonedFromAnchorId (*primitive.ObjectID, nullable pointer)
   - clonedFromUserId (*string, nullable pointer)
   - likeCount (int)
   - cloneCount (int)
   - commentCount (int)
   - viewCount (int)
   - itemCount (int)
   - createdAt (time.Time)
   - updatedAt (time.Time)
   - lastItemAddedAt (time.Time)
   - deletedAt (*time.Time, nullable pointer)

2. Item struct with these fields:
   - id (primitive.ObjectID)
   - anchorId (primitive.ObjectID, indexed)
   - type (string) - "url", "image", "audio", "file", "text"
   - position (int)
   - createdAt (time.Time)
   - updatedAt (time.Time)

3. Embedded type-specific data structs:
   - URLData: {originalUrl, title, description, favicon, thumbnail}
   - ImageData: {cloudinaryUrl, publicId, width, height, fileSize}
   - AudioData: {cloudinaryUrl, publicId, duration, fileSize}
   - FileData: {cloudinaryUrl, publicId, filename, fileType, fileSize}
   - TextData: {content}

4. Item struct should have fields for each type (use pointers, only one will be set):
   - urlData (*URLData)
   - imageData (*ImageData)
   - audioData (*AudioData)
   - fileData (*FileData)
   - textData (*TextData)

5. Request DTOs:
   - CreateAnchorRequest:
     * title (required, min=3, max=100)
     * description (optional, max=500)
     * coverMediaType (optional, default="emoji")
     * coverMediaValue (optional, default="⚓")
     * visibility (optional, default="private")
     * tags (optional, array of strings)
   
   - UpdateAnchorRequest:
     * title (optional, min=3, max=100)
     * description (optional, max=500)
     * coverMediaType (optional)
     * coverMediaValue (optional)
     * visibility (optional)
     * tags (optional)
   
   - AddItemRequest:
     * type (required, enum: url, image, audio, file, text)
     * For type="url": url (required)
     * For type="text": content (required)
     * For type="image/audio/file": these will be handled via multipart upload separately
   
   - ReorderItemsRequest:
     * itemIds ([]string, array of item IDs in new order)

6. Response DTOs:
   - AnchorResponse: Anchor with populated item count
   - AnchorWithItemsResponse: Anchor + array of Items
   - ItemResponse: Item struct as-is

7. Helper methods:
   - (a *Anchor) ToPublicAnchor() map[string]interface{} - returns anchor without sensitive fields
   - (a *Anchor) CanBeViewed(viewerUserId string) bool - checks if viewer can access anchor
   - (a *Anchor) IsOwnedBy(userId string) bool - checks ownership

Follow exact naming conventions from ARCHITECTURE.md and STANDARDS.md.
Use proper validation tags on all request structs.
Add JSON omitempty tags where appropriate.

Show me model.go and STOP. Wait for my approval before continuing.
