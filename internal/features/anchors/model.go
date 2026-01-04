package anchors

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

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

// Anchor represents a collection where users organize content
type Anchor struct {
	ID                 primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID             primitive.ObjectID  `bson:"userId" json:"userId"`
	Title              string              `bson:"title" json:"title"`
	Description        string              `bson:"description" json:"description"`
	CoverMediaType     string              `bson:"coverMediaType" json:"coverMediaType"` // "icon", "emoji", "image"
	CoverMediaValue    string              `bson:"coverMediaValue" json:"coverMediaValue"`
	Visibility         string              `bson:"visibility" json:"visibility"` // "private", "unlisted", "public"
	IsPinned           bool                `bson:"isPinned" json:"isPinned"`
	Tags               []string            `bson:"tags" json:"tags"`
	ClonedFromAnchorID *primitive.ObjectID `bson:"clonedFromAnchorId,omitempty" json:"clonedFromAnchorId,omitempty"`
	ClonedFromUserID   *string             `bson:"clonedFromUserId,omitempty" json:"clonedFromUserId,omitempty"`
	LikeCount          int                 `bson:"likeCount" json:"likeCount"`
	CloneCount         int                 `bson:"cloneCount" json:"cloneCount"`
	CommentCount       int                 `bson:"commentCount" json:"commentCount"`
	ViewCount          int                 `bson:"viewCount" json:"viewCount"`
	ItemCount          int                 `bson:"itemCount" json:"itemCount"`
	CreatedAt          time.Time           `bson:"createdAt" json:"createdAt"`
	UpdatedAt          time.Time           `bson:"updatedAt" json:"updatedAt"`
	LastItemAddedAt    time.Time           `bson:"lastItemAddedAt" json:"lastItemAddedAt"`
	DeletedAt          *time.Time          `bson:"deletedAt,omitempty" json:"deletedAt,omitempty"`
}

// Item represents a single content item within an anchor
type Item struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	AnchorID  primitive.ObjectID `bson:"anchorId" json:"anchorId"`
	Type      string             `bson:"type" json:"type"` // "url", "image", "audio", "file", "text"
	Position  int                `bson:"position" json:"position"`
	URLData   *URLData           `bson:"urlData,omitempty" json:"urlData,omitempty"`
	ImageData *ImageData         `bson:"imageData,omitempty" json:"imageData,omitempty"`
	AudioData *AudioData         `bson:"audioData,omitempty" json:"audioData,omitempty"`
	FileData  *FileData          `bson:"fileData,omitempty" json:"fileData,omitempty"`
	TextData  *TextData          `bson:"textData,omitempty" json:"textData,omitempty"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// URLData contains metadata for URL items
type URLData struct {
	OriginalURL string `bson:"originalUrl" json:"originalUrl"`
	Title       string `bson:"title" json:"title"`
	Description string `bson:"description" json:"description"`
	Favicon     string `bson:"favicon" json:"favicon"`
	Thumbnail   string `bson:"thumbnail" json:"thumbnail"`
}

// ImageData contains metadata for image items
type ImageData struct {
	CloudinaryURL string `bson:"cloudinaryUrl" json:"cloudinaryUrl"`
	PublicID      string `bson:"publicId" json:"publicId"`
	Width         int    `bson:"width" json:"width"`
	Height        int    `bson:"height" json:"height"`
	FileSize      int64  `bson:"fileSize" json:"fileSize"`
}

// AudioData contains metadata for audio items
type AudioData struct {
	CloudinaryURL string `bson:"cloudinaryUrl" json:"cloudinaryUrl"`
	PublicID      string `bson:"publicId" json:"publicId"`
	Duration      int    `bson:"duration" json:"duration"` // in seconds
	FileSize      int64  `bson:"fileSize" json:"fileSize"`
}

// FileData contains metadata for file items
type FileData struct {
	CloudinaryURL string `bson:"cloudinaryUrl" json:"cloudinaryUrl"`
	PublicID      string `bson:"publicId" json:"publicId"`
	Filename      string `bson:"filename" json:"filename"`
	FileType      string `bson:"fileType" json:"fileType"`
	FileSize      int64  `bson:"fileSize" json:"fileSize"`
}

// TextData contains content for text items
type TextData struct {
	Content string `bson:"content" json:"content"`
}

// CreateAnchorRequest represents the payload for creating a new anchor
type CreateAnchorRequest struct {
	Title           string   `json:"title" binding:"required,min=3,max=100"`
	Description     string   `json:"description" binding:"omitempty,max=500"`
	CoverMediaType  *string  `json:"coverMediaType" binding:"omitempty,oneof=icon emoji image"`
	CoverMediaValue *string  `json:"coverMediaValue" binding:"omitempty"`
	Visibility      *string  `json:"visibility" binding:"omitempty,oneof=private unlisted public"`
	Tags            []string `json:"tags" binding:"omitempty,max=5,dive,min=3,max=20"`
}

// UpdateAnchorRequest represents the payload for updating an anchor
type UpdateAnchorRequest struct {
	Title           *string  `json:"title" binding:"omitempty,min=3,max=100"`
	Description     *string  `json:"description" binding:"omitempty,max=500"`
	CoverMediaType  *string  `json:"coverMediaType" binding:"omitempty,oneof=icon emoji image"`
	CoverMediaValue *string  `json:"coverMediaValue" binding:"omitempty"`
	Visibility      *string  `json:"visibility" binding:"omitempty,oneof=private unlisted public"`
	Tags            []string `json:"tags" binding:"omitempty,max=5,dive,min=3,max=20"`
}

// AddItemRequest represents the payload for adding an item to an anchor
type AddItemRequest struct {
	Type    string  `json:"type" binding:"required,oneof=url image audio file text"`
	URL     *string `json:"url" binding:"omitempty"`
	Content *string `json:"content" binding:"omitempty,max=10000"`
}

// ReorderItemsRequest represents the payload for reordering items
type ReorderItemsRequest struct {
	ItemIDs []string `json:"itemIds" binding:"required,min=1"`
}

// AnchorResponse represents the response for a single anchor
type AnchorResponse struct {
	*Anchor
}

// AnchorWithItemsResponse represents an anchor with its items
type AnchorWithItemsResponse struct {
	Anchor Anchor `json:"anchor"`
	Items  []Item `json:"items"`
}

// ItemResponse represents the response for a single item
type ItemResponse struct {
	*Item
}

// ToPublicAnchor returns anchor fields safe for public display
func (a *Anchor) ToPublicAnchor() map[string]interface{} {
	return map[string]interface{}{
		"id":              a.ID,
		"userId":          a.UserID,
		"title":           a.Title,
		"description":     a.Description,
		"coverMediaType":  a.CoverMediaType,
		"coverMediaValue": a.CoverMediaValue,
		"visibility":      a.Visibility,
		"isPinned":        a.IsPinned,
		"tags":            a.Tags,
		"likeCount":       a.LikeCount,
		"cloneCount":      a.CloneCount,
		"commentCount":    a.CommentCount,
		"viewCount":       a.ViewCount,
		"itemCount":       a.ItemCount,
		"createdAt":       a.CreatedAt,
		"updatedAt":       a.UpdatedAt,
		"lastItemAddedAt": a.LastItemAddedAt,
	}
}

// CanBeViewed checks if a viewer can access this anchor
func (a *Anchor) CanBeViewed(viewerUserID primitive.ObjectID) bool {
	// Owner can always view
	if a.UserID == viewerUserID {
		return true
	}

	// Deleted anchors cannot be viewed by non-owners
	if a.DeletedAt != nil {
		return false
	}

	// Public and unlisted anchors can be viewed by anyone
	if a.Visibility == VisibilityPublic || a.Visibility == VisibilityUnlisted {
		return true
	}

	// Private anchors can only be viewed by owner
	return false
}

// IsOwnedBy checks if the anchor is owned by the given user
func (a *Anchor) IsOwnedBy(userID primitive.ObjectID) bool {
	return a.UserID == userID
}
