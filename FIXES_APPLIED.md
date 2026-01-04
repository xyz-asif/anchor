# Step 4: Handler Implementation - All Fixes Applied ✅

## Summary of All Changes

All 9 issues have been successfully fixed across 4 files:

---

## 1. ✅ model.go - UPDATED

### Changes Made:
1. **Added Constants** (lines 9-24):
   ```go
   // Visibility constants
   const (
       VisibilityPrivate  = "private"
       VisibilityUnlisted = "unlisted"
       VisibilityPublic   = "public"
   )

   // Item type constants
   const (
       ItemTypeURL   = "url"
       ItemTypeImage = "image"
       ItemTypeAudio = "audio"
       ItemTypeFile  = "file"
       ItemTypeText  = "text"
   )
   ```

2. **Fixed UserID Type** (line 28):
   ```go
   // Changed from:
   UserID string `bson:"userId" json:"userId"`
   
   // To:
   UserID primitive.ObjectID `bson:"userId" json:"userId"`
   ```

3. **Updated CreateAnchorRequest** (lines 104-111):
   ```go
   type CreateAnchorRequest struct {
       Title           string   `json:"title" binding:"required,min=3,max=100"`
       Description     string   `json:"description" binding:"omitempty,max=500"`
       CoverMediaType  *string  `json:"coverMediaType" binding:"omitempty,oneof=icon emoji image"`  // Changed to pointer
       CoverMediaValue *string  `json:"coverMediaValue" binding:"omitempty"`                        // Changed to pointer
       Visibility      *string  `json:"visibility" binding:"omitempty,oneof=private unlisted public"` // Changed to pointer
       Tags            []string `json:"tags" binding:"omitempty,max=5,dive,min=3,max=20"`
   }
   ```

4. **Updated AddItemRequest** (lines 121-125):
   ```go
   type AddItemRequest struct {
       Type    string  `json:"type" binding:"required,oneof=url image audio file text"`
       URL     *string `json:"url" binding:"omitempty"`      // Changed to pointer
       Content *string `json:"content" binding:"omitempty,max=10000"` // Changed to pointer
   }
   ```

5. **Fixed AnchorWithItemsResponse** (lines 141-144):
   ```go
   type AnchorWithItemsResponse struct {
       Anchor Anchor `json:"anchor"`  // Changed from *Anchor to Anchor
       Items  []Item `json:"items"`
   }
   ```

6. **Updated Helper Methods** (lines 169-193):
   ```go
   // Changed from string to primitive.ObjectID
   func (a *Anchor) CanBeViewed(viewerUserID primitive.ObjectID) bool {
       if a.UserID == viewerUserID {
           return true
       }
       if a.DeletedAt != nil {
           return false
       }
       if a.Visibility == VisibilityPublic || a.Visibility == VisibilityUnlisted {
           return true
       }
       return false
   }

   func (a *Anchor) IsOwnedBy(userID primitive.ObjectID) bool {
       return a.UserID == userID
   }
   ```

---

## 2. ✅ repository.go - UPDATED

### Changes Made:
1. **Updated All Method Signatures** - Changed from `string` to `primitive.ObjectID`:
   - `GetAnchorByID(ctx, anchorID primitive.ObjectID)`
   - `UpdateAnchor(ctx, anchorID primitive.ObjectID, updates bson.M)`
   - `SoftDeleteAnchor(ctx, anchorID primitive.ObjectID)`
   - `CountPinnedAnchors(ctx, userID primitive.ObjectID)`
   - `GetAnchorItems(ctx, anchorID primitive.ObjectID)`
   - `GetItemByID(ctx, itemID primitive.ObjectID)`
   - `DeleteItem(ctx, itemID primitive.ObjectID)`
   - `CountAnchorItems(ctx, anchorID primitive.ObjectID)`

2. **Renamed Methods**:
   - `GetAnchorsByUserID` → `GetUserAnchors`
   - `AddItem` → `CreateItem`
   - `GetItemsByAnchorID` → `GetAnchorItems`
   - `CountUserPinnedAnchors` → `CountPinnedAnchors`

3. **Added New Method** - `GetPublicUserAnchors`:
   ```go
   func (r *Repository) GetPublicUserAnchors(ctx context.Context, userID primitive.ObjectID) ([]Anchor, error) {
       filter := bson.M{
           "userId":     userID,
           "deletedAt":  nil,
           "visibility": bson.M{"$in": []string{VisibilityPublic, VisibilityUnlisted}},
       }
       // ... rest of implementation
   }
   ```

4. **Updated UpdateAnchor** - Now handles both `$inc` and `$set` operations:
   ```go
   func (r *Repository) UpdateAnchor(ctx context.Context, anchorID primitive.ObjectID, updates bson.M) error {
       filter := bson.M{"_id": anchorID}
       
       var update bson.M
       if _, hasInc := updates["$inc"]; hasInc {
           update = updates
       } else {
           update = bson.M{"$set": updates}
       }
       // ... rest
   }
   ```

5. **Removed GetPinnedAnchors** - Not needed for current implementation

---

## 3. ✅ validator.go - UPDATED

### Added Three Validation Functions:

1. **ValidateCreateAnchorRequest** (lines 172-197):
   ```go
   func ValidateCreateAnchorRequest(req *CreateAnchorRequest) error {
       if err := ValidateAnchorTitle(req.Title); err != nil {
           return err
       }
       if err := ValidateDescription(req.Description); err != nil {
           return err
       }
       if len(req.Tags) > 0 {
           if err := ValidateTags(req.Tags); err != nil {
               return err
           }
       }
       if req.Visibility != nil {
           if err := ValidateVisibility(*req.Visibility); err != nil {
               return err
           }
       }
       if req.CoverMediaType != nil {
           if err := ValidateCoverMediaType(*req.CoverMediaType); err != nil {
               return err
           }
       }
       return nil
   }
   ```

2. **ValidateUpdateAnchorRequest** (lines 200-225):
   ```go
   func ValidateUpdateAnchorRequest(req *UpdateAnchorRequest) error {
       if req.Title != nil {
           if err := ValidateAnchorTitle(*req.Title); err != nil {
               return err
           }
       }
       // ... validates all non-nil fields
       return nil
   }
   ```

3. **ValidateAddItemRequest** (lines 228-251):
   ```go
   func ValidateAddItemRequest(req *AddItemRequest) error {
       if err := ValidateItemType(req.Type); err != nil {
           return err
       }
       
       switch req.Type {
       case ItemTypeURL:
           if req.URL == nil || *req.URL == "" {
               return errors.New("URL is required for URL type items")
           }
           if err := ValidateURL(*req.URL); err != nil {
               return err
           }
       case ItemTypeText:
           if req.Content == nil || *req.Content == "" {
               return errors.New("content is required for text type items")
           }
           if err := ValidateTextContent(*req.Content); err != nil {
               return err
           }
       }
       
       return nil
   }
   ```

---

## 4. ✅ auth/repository.go - UPDATED

### Added IncrementAnchorCount Method (lines 151-171):
```go
// IncrementAnchorCount increments or decrements the user's anchor count
func (r *Repository) IncrementAnchorCount(ctx context.Context, userID primitive.ObjectID, delta int) error {
    filter := bson.M{"_id": userID}
    update := bson.M{
        "$inc": bson.M{"anchorCount": delta},
        "$set": bson.M{"updatedAt": time.Now()},
    }
    
    result, err := r.collection.UpdateOne(ctx, filter, update)
    if err != nil {
        return err
    }
    
    if result.MatchedCount == 0 {
        return errors.New("user not found")
    }
    
    return nil
}
```

---

## 5. ✅ handler.go - UPDATED

### Fixed Import Path (line 9):
```go
// Changed from:
"github.com/xyz-asif/gotodo/pkg/response"

// To:
"github.com/xyz-asif/gotodo/internal/pkg/response"
```

### Fixed Tags Handling (line 327):
```go
// Changed from:
normalizedTags := NormalizeTags(*req.Tags)

// To:
normalizedTags := NormalizeTags(req.Tags)
```

---

## ✅ All Issues Resolved

| Issue | File | Status |
|-------|------|--------|
| 1. Import path | handler.go | ✅ Fixed |
| 2. Add constants | model.go | ✅ Added |
| 3. Fix CreateAnchorRequest | model.go | ✅ Fixed |
| 4. Fix AddItemRequest | model.go | ✅ Fixed |
| 5. Fix helper methods | model.go | ✅ Fixed |
| 6. Add GetPublicUserAnchors | repository.go | ✅ Added |
| 7. Fix repository signatures | repository.go | ✅ Fixed |
| 8. Add validation functions | validator.go | ✅ Added |
| 9. Add IncrementAnchorCount | auth/repository.go | ✅ Added |

---

## 🎯 Handler is Now Complete

All 8 endpoints are fully implemented and all dependencies are resolved:

1. ✅ **CreateAnchor** - POST /anchors
2. ✅ **GetAnchor** - GET /anchors/:id
3. ✅ **ListUserAnchors** - GET /anchors
4. ✅ **UpdateAnchor** - PATCH /anchors/:id
5. ✅ **DeleteAnchor** - DELETE /anchors/:id
6. ✅ **TogglePin** - PATCH /anchors/:id/pin
7. ✅ **AddItem** - POST /anchors/:id/items
8. ✅ **DeleteItem** - DELETE /anchors/:id/items/:itemId

**Ready for your approval to proceed to STEP 5!**
