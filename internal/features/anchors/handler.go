package anchors

import (
	"context"
	"errors"
	"log"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"github.com/xyz-asif/gotodo/internal/pkg/tags"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AnchorFollowService defines the interface for interacting with anchor follows
type AnchorFollowService interface {
	UpdateLastSeenVersion(ctx context.Context, userID, anchorID primitive.ObjectID, version int) error
}

// Handler handles HTTP requests for anchor feature
type Handler struct {
	repo                *Repository
	authRepo            *auth.Repository
	notificationService *notifications.Service
	config              *config.Config
	cloudinary          *cloudinary.Service
	likesRepo           interface{}         // Using interface to avoid cycle
	followsRepo         interface{}         // Using interface to avoid cycle
	anchorFollowService AnchorFollowService // Interface to avoid cycle
}

// NewHandler creates a new anchor handler
func NewHandler(repo *Repository, authRepo *auth.Repository, notificationService *notifications.Service, cfg *config.Config, cld *cloudinary.Service, likesRepo interface{}, followsRepo interface{}, anchorFollowService AnchorFollowService) *Handler {
	return &Handler{
		repo:                repo,
		authRepo:            authRepo,
		notificationService: notificationService,
		config:              cfg,
		cloudinary:          cld,
		likesRepo:           likesRepo,
		followsRepo:         followsRepo,
		anchorFollowService: anchorFollowService,
	}
}

// CreateAnchor handles the creation of a new anchor
// @Summary Create a new anchor
// @Description Create a new anchor for the authenticated user
// @Tags anchors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateAnchorRequest true "Anchor details"
// @Success 201 {object} response.APIResponse{data=Anchor}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /anchors [post]
func (h *Handler) CreateAnchor(c *gin.Context) {
	var req CreateAnchorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// Validate title length
	if n := utf8.RuneCountInString(req.Title); n < 3 || n > 100 {
		response.BadRequest(c, "Title must be between 3 and 100 characters", "VALIDATION_FAILED")
		return
	}

	// Default visibility if invalid
	visibility := VisibilityPublic
	if req.Visibility != nil {
		v := *req.Visibility
		if v == VisibilityPublic || v == VisibilityPrivate || v == VisibilityUnlisted {
			visibility = v
		}
	}

	var coverMediaType, coverMediaValue string
	if req.CoverMediaType != nil {
		coverMediaType = *req.CoverMediaType
	}
	if req.CoverMediaValue != nil {
		coverMediaValue = *req.CoverMediaValue
	}

	// Normalize tags
	if req.Tags != nil {
		req.Tags = normalizeTags(req.Tags)
	}

	anchor := &Anchor{
		UserID:          user.ID,
		Title:           req.Title,
		Description:     req.Description,
		CoverMediaType:  coverMediaType,
		CoverMediaValue: coverMediaValue,
		Visibility:      visibility,
		Tags:            req.Tags,
		ItemCount:       0,
		LikeCount:       0,
		CloneCount:      0,
		IsPinned:        false,
		CreatedAt:       time.Now(), // Ensure created time
		UpdatedAt:       time.Now(),
	}

	if err := h.repo.CreateAnchor(c.Request.Context(), anchor); err != nil {
		response.InternalServerError(c, "Failed to create anchor", "DATABASE_ERROR")
		return
	}

	// Increment user's anchor count
	if err := h.authRepo.IncrementAnchorCount(c.Request.Context(), user.ID, 1); err != nil {
		// Log error but don't fail request as anchor was created
		log.Printf("Failed to increment anchor count for user %s: %v", user.ID.Hex(), err)
	}

	response.Created(c, anchor)
}

// UpdateAnchor handles updating an existing anchor
// @Summary Update an anchor
// @Description Update anchor details
// @Tags anchors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param request body UpdateAnchorRequest true "Update details"
// @Success 200 {object} response.APIResponse{data=Anchor}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id} [patch]
func (h *Handler) UpdateAnchor(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	var req UpdateAnchorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// Verify ownership
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission to update this anchor")
		return
	}

	updates := make(map[string]interface{})
	if req.Title != nil {
		if n := utf8.RuneCountInString(*req.Title); n < 3 || n > 100 {
			response.BadRequest(c, "Title must be between 3 and 100 characters", "VALIDATION_FAILED")
			return
		}
		updates["title"] = *req.Title
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.CoverMediaType != nil {
		updates["coverMediaType"] = *req.CoverMediaType
	}
	if req.CoverMediaValue != nil {
		updates["coverMediaValue"] = *req.CoverMediaValue
	}
	if req.Visibility != nil {
		updates["visibility"] = *req.Visibility
	}
	if req.Tags != nil {
		updates["tags"] = normalizeTags(req.Tags)
	}

	if len(updates) == 0 {
		response.Success(c, anchor)
		return
	}

	updates["updatedAt"] = time.Now()

	if err := h.repo.UpdateAnchor(c.Request.Context(), anchorID, updates); err != nil {
		response.InternalServerError(c, "Failed to update anchor", "DATABASE_ERROR")
		return
	}

	// Fetch updated
	updatedAnchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch updated anchor", "DATABASE_ERROR")
		return
	}

	response.Success(c, updatedAnchor)
}

// normalizeTags converts tags to lowercase and trims whitespace
func normalizeTags(raw []string) []string {
	if raw == nil {
		return nil
	}
	// Same rules as interests and tag feeds, so they always match each other
	return tags.NormalizeList(raw)
}

// DeleteAnchor soft deletes an anchor
// @Summary Delete an anchor
// @Description Soft delete an anchor
// @Tags anchors
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id} [delete]
func (h *Handler) DeleteAnchor(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// Verify ownership
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission to delete this anchor")
		return
	}

	if err := h.repo.SoftDeleteAnchor(c.Request.Context(), anchorID); err != nil {
		response.InternalServerError(c, "Failed to delete anchor", "DATABASE_ERROR")
		return
	}

	// Deleting a clone lowers the original's clone count (and therefore its ranking score)
	if anchor.ClonedFromAnchorID != nil {
		if err := h.repo.IncrementCloneCount(c.Request.Context(), *anchor.ClonedFromAnchorID, -1); err != nil {
			log.Printf("Failed to decrement clone count for anchor %s: %v", anchor.ClonedFromAnchorID.Hex(), err)
		}
		if err := h.repo.UpdateEngagementScore(c.Request.Context(), *anchor.ClonedFromAnchorID); err != nil {
			log.Printf("Failed to update engagement score for anchor %s: %v", anchor.ClonedFromAnchorID.Hex(), err)
		}
	}

	// Decrement user's anchor count
	if err := h.authRepo.IncrementAnchorCount(c.Request.Context(), user.ID, -1); err != nil {
		log.Printf("Failed to decrement anchor count for user %s: %v", user.ID.Hex(), err)
	}

	response.Success(c, "Anchor deleted successfully")
}

// GetAnchor retrieves a single anchor details
// @Summary Get anchor details
// @Description Get anchor details and items
// @Tags anchors
// @Produce json
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse{data=AnchorWithItemsResponse}
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id} [get]
func (h *Handler) GetAnchor(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	// Access control: deleted -> 404, someone else's private anchor -> 401/403
	if !ensureAccess(c, anchor) {
		return
	}

	// Get items
	items, err := h.repo.GetAnchorItems(c.Request.Context(), anchorID)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch anchor items", "DATABASE_ERROR")
		return
	}

	// Build response
	anchorResponse := AnchorWithItemsResponse{
		Anchor: *anchor,
		Items:  items,
	}

	// Update last seen version for authenticated user (async)
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			go func(uid, aid primitive.ObjectID, ver int) {
				if h.anchorFollowService != nil {
					_ = h.anchorFollowService.UpdateLastSeenVersion(context.Background(), uid, aid, ver)
				}
			}(user.ID, anchorID, anchor.Version)
		}
	}

	response.Success(c, anchorResponse)
}

// ListUserAnchors lists anchors for a user
// @Summary List user anchors
// @Description List anchors for a user (public/unlisted)
// @Tags anchors
// @Produce json
// @Param userId query string true "User ID"
// @Param page query int false "Page number"
// @Param limit query int false "Limit per page"
// @Success 200 {object} response.APIResponse{data=PaginatedResponse}
// @Router /anchors [get]
func (h *Handler) ListUserAnchors(c *gin.Context) {
	userIDStr := c.Query("userId")
	if userIDStr == "" {
		response.BadRequest(c, "userId is required", "MISSING_PARAM")
		return
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}

	page, limit := pageParams(c, 10, 50)

	var anchors []Anchor
	var total int64

	// Determine if viewer is owner
	isOwner := false
	if val, exists := c.Get("user"); exists {
		if currentUser, ok := val.(*auth.User); ok && currentUser.ID == userID {
			isOwner = true
		}
	}

	if isOwner {
		anchors, total, err = h.repo.GetUserAnchors(c.Request.Context(), userID, page, limit)
	} else {
		anchors, total, err = h.repo.GetPublicUserAnchors(c.Request.Context(), userID, page, limit)
	}

	if err != nil {
		response.InternalServerError(c, "Failed to fetch anchors", "DATABASE_ERROR")
		return
	}

	response.Success(c, PaginatedResponse{
		Data: anchors,
		Pagination: struct { // This assumes PaginatedResponse structure matches model
			Page       int   `json:"page"`
			Limit      int   `json:"limit"`
			Total      int64 `json:"total"`
			TotalPages int   `json:"totalPages"`
			HasMore    bool  `json:"hasMore"`
		}{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: int((total + int64(limit) - 1) / int64(limit)),
			HasMore:    int64(page*limit) < total,
		},
	})
}

// ListAnchorItems lists items for an anchor with pagination
func (h *Handler) ListAnchorItems(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	page, limit := pageParams(c, 20, 100)

	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	// Access control: deleted -> 404, someone else's private anchor -> 401/403
	if !ensureAccess(c, anchor) {
		return
	}

	items, total, err := h.repo.GetAnchorItemsPaginated(c.Request.Context(), anchorID, page, limit)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch items", "DATABASE_ERROR")
		return
	}

	response.Success(c, gin.H{
		"items": items,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// AddItem adds a new item to an anchor
func (h *Handler) AddItem(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	var req AddItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	// Media items must go through the upload endpoint
	if req.Type == ItemTypeImage || req.Type == ItemTypeAudio || req.Type == ItemTypeFile {
		response.BadRequest(c, "Use POST /anchors/{id}/items/upload for image, audio and file items", "USE_UPLOAD_ENDPOINT")
		return
	}
	if err := ValidateAddItemRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// Verify ownership
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission")
		return
	}

	// Get current item count to set position
	count, err := h.repo.CountAnchorItems(c.Request.Context(), anchorID)
	if err != nil {
		response.InternalServerError(c, "Database error", "DATABASE_ERROR")
		return
	}

	// Map content based on type
	var textData *TextData
	var urlData *URLData

	if req.Type == ItemTypeText && req.Content != nil {
		textData = &TextData{Content: strings.TrimSpace(*req.Content)}
	} else if req.Type == ItemTypeURL && req.URL != nil {
		targetURL := strings.TrimSpace(*req.URL)
		urlData = &URLData{OriginalURL: targetURL}

		// Best effort: fill title/description/favicon/thumbnail from the page.
		// Failures (timeouts, blocked addresses, non-HTML) keep just the URL.
		fetchCtx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		if meta, err := FetchURLMetadata(fetchCtx, targetURL); err == nil && meta != nil {
			meta.OriginalURL = targetURL
			urlData = meta
		} else if err != nil {
			log.Printf("URL metadata fetch failed for %s: %v", targetURL, err)
		}
		cancel()
	}

	item := &Item{
		AnchorID:  anchorID,
		Type:      req.Type,
		Position:  int(count),
		TextData:  textData,
		URLData:   urlData,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.repo.CreateItem(c.Request.Context(), item); err != nil {
		response.InternalServerError(c, "Failed to create item", "DATABASE_ERROR")
		return
	}

	h.afterItemAdded(c.Request.Context(), anchor, item, user.ID)

	response.Created(c, item)
}

// afterItemAdded updates anchor bookkeeping (lastItemAddedAt, itemCount, version)
// and notifies anchor followers asynchronously.
func (h *Handler) afterItemAdded(ctx context.Context, anchor *Anchor, item *Item, actorID primitive.ObjectID) {
	if err := h.repo.UpdateAnchor(ctx, anchor.ID, map[string]interface{}{
		"$set": map[string]interface{}{"lastItemAddedAt": item.CreatedAt},
		"$inc": map[string]interface{}{"itemCount": 1},
	}); err != nil {
		log.Printf("Failed to update anchor %s after adding item: %v", anchor.ID.Hex(), err)
	}

	if err := h.repo.IncrementVersion(ctx, anchor.ID); err != nil {
		log.Printf("Failed to increment version for anchor %s: %v", anchor.ID.Hex(), err)
	}

	// Followers of an anchor that has since gone private must not learn about its updates
	if anchor.Visibility == VisibilityPrivate {
		return
	}

	notificationService := h.notificationService
	if notificationService == nil {
		notificationService = notifications.GetService(h.repo.db)
	}
	go func(aid primitive.ObjectID, title string) {
		if err := notificationService.CreateAnchorUpdateNotifications(context.Background(), aid, title, actorID); err != nil {
			log.Printf("Failed to create anchor update notifications: %v", err)
		}
	}(anchor.ID, anchor.Title)
}

// UploadItem uploads an image, audio or document file as an anchor item.
// The item type is chosen from the file extension:
//   - image (.jpg .jpeg .png .gif .webp, max 10MB)
//   - audio (.mp3 .wav .aac .m4a .ogg, max 25MB)
//   - file  (.pdf .docx .doc .epub .txt, max 50MB)
//
// @Summary Upload an item file
// @Tags anchors
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param file formData file true "File to upload"
// @Success 201 {object} response.APIResponse{data=Item}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/items/upload [post]
func (h *Handler) UploadItem(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	if h.cloudinary == nil {
		response.ServiceUnavailable(c, "File uploads are not configured", "UPLOADS_UNAVAILABLE")
		return
	}

	// Verify ownership first
	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission")
		return
	}

	// Cap the request body at the largest allowed file size (+1MB for multipart overhead)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cloudinary.MaxFileSize+(1<<20))

	file, err := c.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			response.Error(c, http.StatusRequestEntityTooLarge, "File is too large (max 50MB)", "FILE_TOO_LARGE")
			return
		}
		response.BadRequest(c, "File is required (multipart field 'file', max 50MB)", "MISSING_FILE")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	itemType := ItemTypeFile
	var validationErr error
	switch {
	case containsString(cloudinary.AllowedImageTypes, ext):
		itemType = ItemTypeImage
		validationErr = cloudinary.ValidateImageFile(file)
	case containsString(cloudinary.AllowedAudioTypes, ext):
		itemType = ItemTypeAudio
		validationErr = cloudinary.ValidateAudioFile(file)
	default:
		validationErr = cloudinary.ValidateFile(file)
	}
	if validationErr != nil {
		response.BadRequest(c, validationErr.Error(), "INVALID_FILE")
		return
	}

	fileContent, err := file.Open()
	if err != nil {
		response.InternalServerError(c, "Failed to open file", "FILE_ERROR")
		return
	}
	defer fileContent.Close()

	ctx := c.Request.Context()
	now := time.Now()
	item := &Item{
		AnchorID:  anchorID,
		Type:      itemType,
		CreatedAt: now,
		UpdatedAt: now,
	}

	switch itemType {
	case ItemTypeImage:
		result, err := h.cloudinary.UploadImage(ctx, fileContent, file.Filename)
		if err != nil {
			log.Printf("Image upload failed: %v", err)
			response.InternalServerError(c, "Failed to upload image", "UPLOAD_FAILED")
			return
		}
		item.ImageData = &ImageData{
			CloudinaryURL: result.URL,
			PublicID:      result.PublicID,
			Width:         result.Width,
			Height:        result.Height,
			FileSize:      result.FileSize,
		}
	case ItemTypeAudio:
		result, err := h.cloudinary.UploadAudio(ctx, fileContent, file.Filename)
		if err != nil {
			log.Printf("Audio upload failed: %v", err)
			response.InternalServerError(c, "Failed to upload audio", "UPLOAD_FAILED")
			return
		}
		item.AudioData = &AudioData{
			CloudinaryURL: result.URL,
			PublicID:      result.PublicID,
			Duration:      int(math.Round(result.Duration)),
			FileSize:      result.FileSize,
		}
	default:
		result, err := h.cloudinary.UploadFile(ctx, fileContent, file.Filename)
		if err != nil {
			log.Printf("File upload failed: %v", err)
			response.InternalServerError(c, "Failed to upload file", "UPLOAD_FAILED")
			return
		}
		item.FileData = &FileData{
			CloudinaryURL: result.URL,
			PublicID:      result.PublicID,
			Filename:      file.Filename,
			FileType:      strings.TrimPrefix(ext, "."),
			FileSize:      result.FileSize,
		}
	}

	count, err := h.repo.CountAnchorItems(ctx, anchorID)
	if err != nil {
		response.InternalServerError(c, "Database error", "DATABASE_ERROR")
		return
	}
	item.Position = int(count)

	if err := h.repo.CreateItem(ctx, item); err != nil {
		// Don't leave an orphaned asset behind
		h.deleteItemAsset(ctx, item)
		response.InternalServerError(c, "Failed to create item", "DATABASE_ERROR")
		return
	}

	h.afterItemAdded(ctx, anchor, item, user.ID)

	response.Created(c, item)
}

// deleteItemAsset removes an item's uploaded asset from Cloudinary (best effort)
func (h *Handler) deleteItemAsset(ctx context.Context, item *Item) {
	if h.cloudinary == nil {
		return
	}
	var publicID, resourceType string
	switch {
	case item.ImageData != nil:
		publicID, resourceType = item.ImageData.PublicID, "image"
	case item.AudioData != nil:
		publicID, resourceType = item.AudioData.PublicID, "video" // Cloudinary stores audio as "video"
	case item.FileData != nil:
		publicID, resourceType = item.FileData.PublicID, "raw"
	}
	if publicID == "" {
		return
	}
	var url string
	switch {
	case item.ImageData != nil:
		url = item.ImageData.CloudinaryURL
	case item.AudioData != nil:
		url = item.AudioData.CloudinaryURL
	case item.FileData != nil:
		url = item.FileData.CloudinaryURL
	}
	if h.repo.IsAssetShared(ctx, item.AnchorID, url) {
		return
	}
	if err := h.cloudinary.Delete(ctx, publicID, resourceType); err != nil {
		log.Printf("Failed to delete Cloudinary asset %s: %v", publicID, err)
	}
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// UpdateItem edits a text or URL item
// @Summary Edit an item
// @Description Text items: send {"content"}. URL items: send {"url"}; the link preview is fetched again.
// @Description Image, audio and file items can't be edited — delete and upload again instead.
// @Tags anchors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param itemId path string true "Item ID"
// @Param request body UpdateItemRequest true "New content"
// @Success 200 {object} response.APIResponse{data=Item}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/items/{itemId} [patch]
func (h *Handler) UpdateItem(c *gin.Context) {
	anchorID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}
	itemID, err := primitive.ObjectIDFromHex(c.Param("itemId"))
	if err != nil {
		response.BadRequest(c, "Invalid item ID", "INVALID_ID")
		return
	}

	var req UpdateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	ctx := c.Request.Context()

	anchor, err := h.repo.GetAnchorByID(ctx, anchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}
	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission")
		return
	}

	item, err := h.repo.GetItemByID(ctx, itemID)
	if err != nil || item.AnchorID != anchorID {
		response.NotFound(c, "Item not found", "ITEM_NOT_FOUND")
		return
	}

	set := bson.M{}
	switch item.Type {
	case ItemTypeText:
		if req.Content == nil {
			response.BadRequest(c, "content is required for text items", "VALIDATION_FAILED")
			return
		}
		if err := ValidateTextContent(*req.Content); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		set["textData"] = TextData{Content: strings.TrimSpace(*req.Content)}
	case ItemTypeURL:
		if req.URL == nil {
			response.BadRequest(c, "url is required for URL items", "VALIDATION_FAILED")
			return
		}
		targetURL := strings.TrimSpace(*req.URL)
		if err := ValidateURL(targetURL); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		urlData := &URLData{OriginalURL: targetURL}
		fetchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		if meta, err := FetchURLMetadata(fetchCtx, targetURL); err == nil && meta != nil {
			meta.OriginalURL = targetURL
			urlData = meta
		} else if err != nil {
			log.Printf("URL metadata fetch failed for %s: %v", targetURL, err)
		}
		cancel()
		set["urlData"] = urlData
	default:
		response.BadRequest(c, "Only text and URL items can be edited; delete and re-upload media items", "ITEM_NOT_EDITABLE")
		return
	}

	updated, err := h.repo.UpdateItem(ctx, itemID, set)
	if err != nil {
		response.InternalServerError(c, "Failed to update item", "DATABASE_ERROR")
		return
	}

	if err := h.repo.UpdateAnchor(ctx, anchorID, map[string]interface{}{"updatedAt": time.Now()}); err != nil {
		log.Printf("Failed to touch anchor %s after item edit: %v", anchorID.Hex(), err)
	}

	response.Success(c, updated)
}

// DeleteItem deletes an item
func (h *Handler) DeleteItem(c *gin.Context) {
	anchorIDStr := c.Param("id")
	itemIDStr := c.Param("itemId")

	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}
	itemID, err := primitive.ObjectIDFromHex(itemIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid item ID", "INVALID_ID")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// Verify ownership of anchor
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission")
		return
	}

	// Verify item belongs to anchor
	item, err := h.repo.GetItemByID(c.Request.Context(), itemID)
	if err != nil {
		response.NotFound(c, "Item not found", "ITEM_NOT_FOUND")
		return
	}
	if item.AnchorID != anchorID {
		response.BadRequest(c, "Item does not belong to this anchor", "INVALID_RELATION")
		return
	}

	if err := h.repo.DeleteItem(c.Request.Context(), itemID); err != nil {
		response.InternalServerError(c, "Failed to delete item", "DATABASE_ERROR")
		return
	}

	// Remove the uploaded file too (unless a clone still points at it)
	h.deleteItemAsset(c.Request.Context(), item)

	// Close the gap in positions and set itemCount to the real number of items
	if err := h.repo.RenumberItems(c.Request.Context(), anchorID); err != nil {
		log.Printf("Failed to renumber items of anchor %s: %v", anchorID.Hex(), err)
	}

	response.Success(c, "Item deleted")
}

// ReorderItems updates the order of items
func (h *Handler) ReorderItems(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	var req struct {
		ItemIDs []string `json:"itemIds" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request", "INVALID_JSON")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission")
		return
	}

	// Must list every item of the anchor exactly once
	count, err := h.repo.CountAnchorItems(c.Request.Context(), anchorID)
	if err != nil {
		response.InternalServerError(c, "Database error", "DATABASE_ERROR")
		return
	}
	if err := ValidateReorderItemsRequest(&ReorderItemsRequest{ItemIDs: req.ItemIDs}, int(count)); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	if err := h.repo.ReorderItems(c.Request.Context(), anchorIDStr, req.ItemIDs); err != nil {
		response.InternalServerError(c, "Failed to reorder items", "DATABASE_ERROR")
		return
	}

	response.Success(c, "Items reordered")
}

// CloneAnchor creates a copy of another user's anchor (and all its items) for the current user
// @Summary Clone an anchor
// @Description Copy a public or unlisted anchor, including its items, into your own anchors.
// @Description Body is optional; title defaults to the original title and visibility to public.
// @Tags anchors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param request body CloneAnchorRequest false "Optional overrides"
// @Success 201 {object} response.APIResponse{data=Anchor}
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Failure 409 {object} response.APIResponse
// @Router /anchors/{id}/clone [post]
func (h *Handler) CloneAnchor(c *gin.Context) {
	anchorID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// Body is optional
	var req CloneAnchorRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid request format", "INVALID_JSON")
			return
		}
	}

	ctx := c.Request.Context()

	original, err := h.repo.GetAnchorByID(ctx, anchorID)
	// Private anchors are reported as not found so their existence isn't revealed
	if err != nil || original.DeletedAt != nil ||
		(original.Visibility == VisibilityPrivate && original.UserID != user.ID) {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if original.UserID == user.ID {
		response.BadRequest(c, "You cannot clone your own anchor", "CANNOT_CLONE_OWN")
		return
	}

	// Blocked users (either direction) can't clone each other's anchors
	if blocked, err := h.authRepo.IsBlockedBetween(ctx, user, original.UserID); err != nil {
		response.InternalServerError(c, "Database error", "DATABASE_ERROR")
		return
	} else if blocked {
		response.Forbidden(c, "You can't interact with this user", "USER_BLOCKED")
		return
	}

	existing, err := h.repo.FindUserCloneOf(ctx, user.ID, anchorID)
	if err != nil {
		response.InternalServerError(c, "Database error", "DATABASE_ERROR")
		return
	}
	if existing != nil {
		response.Respond(c, http.StatusConflict, false, "You have already cloned this anchor",
			gin.H{"anchorId": existing.ID}, "ALREADY_CLONED")
		return
	}

	title := original.Title
	if req.Title != nil {
		title = strings.TrimSpace(*req.Title)
		if err := ValidateAnchorTitle(title); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
	}
	// A clone can't be more visible than its original: copies of link-only (unlisted)
	// anchors must not end up in public feeds.
	visibility := VisibilityPublic
	if original.Visibility == VisibilityUnlisted {
		visibility = VisibilityUnlisted
	}
	if req.Visibility != nil {
		visibility = *req.Visibility
	}
	if original.Visibility == VisibilityUnlisted && visibility == VisibilityPublic {
		response.BadRequest(c, "Clones of unlisted anchors can't be public", "VISIBILITY_NOT_ALLOWED")
		return
	}

	items, err := h.repo.GetAnchorItems(ctx, anchorID)
	if err != nil {
		response.InternalServerError(c, "Failed to read anchor items", "DATABASE_ERROR")
		return
	}

	originalOwner := original.UserID.Hex()
	clone := &Anchor{
		UserID:             user.ID,
		Title:              title,
		Description:        original.Description,
		CoverMediaType:     original.CoverMediaType,
		CoverMediaValue:    original.CoverMediaValue,
		Visibility:         visibility,
		Tags:               append([]string{}, original.Tags...),
		ClonedFromAnchorID: &original.ID,
		ClonedFromUserID:   &originalOwner,
		ItemCount:          len(items),
		Version:            1,
	}
	if clone.Tags == nil {
		clone.Tags = []string{}
	}

	if err := h.repo.CreateAnchor(ctx, clone); err != nil {
		response.InternalServerError(c, "Failed to clone anchor", "DATABASE_ERROR")
		return
	}

	// Guard against double-submits: if two requests raced past the "already cloned" check,
	// keep only the oldest clone and roll this one back.
	cloneIDs, err := h.repo.ActiveCloneIDs(ctx, user.ID, original.ID)
	if err == nil && len(cloneIDs) > 1 && cloneIDs[0] != clone.ID {
		if delErr := h.repo.DeleteAnchor(ctx, clone.ID); delErr != nil {
			log.Printf("Failed to roll back duplicate clone %s: %v", clone.ID.Hex(), delErr)
		}
		response.Respond(c, http.StatusConflict, false, "You have already cloned this anchor",
			gin.H{"anchorId": cloneIDs[0]}, "ALREADY_CLONED")
		return
	}

	if len(items) > 0 {
		now := time.Now()
		docs := make([]interface{}, 0, len(items))
		for i, it := range items {
			copied := it
			copied.ID = primitive.NewObjectID()
			copied.AnchorID = clone.ID
			copied.Position = i
			copied.CreatedAt = now
			copied.UpdatedAt = now
			// Media files stay owned by the original anchor. The clone points at the same
			// URL but has no PublicID, so deleting the clone never deletes the original's asset.
			if it.ImageData != nil {
				d := *it.ImageData
				d.PublicID = ""
				copied.ImageData = &d
			}
			if it.AudioData != nil {
				d := *it.AudioData
				d.PublicID = ""
				copied.AudioData = &d
			}
			if it.FileData != nil {
				d := *it.FileData
				d.PublicID = ""
				copied.FileData = &d
			}
			docs = append(docs, copied)
		}
		if err := h.repo.CreateItems(ctx, docs); err != nil {
			// Roll back the half-created clone
			if delErr := h.repo.DeleteAnchor(ctx, clone.ID); delErr != nil {
				log.Printf("Failed to roll back clone %s: %v", clone.ID.Hex(), delErr)
			}
			response.InternalServerError(c, "Failed to copy anchor items", "DATABASE_ERROR")
			return
		}
	}

	if err := h.repo.IncrementCloneCount(ctx, original.ID, 1); err != nil {
		log.Printf("Failed to increment clone count for anchor %s: %v", original.ID.Hex(), err)
	}
	if err := h.repo.UpdateEngagementScore(ctx, original.ID); err != nil {
		log.Printf("Failed to update engagement score for anchor %s: %v", original.ID.Hex(), err)
	}
	if err := h.authRepo.IncrementAnchorCount(ctx, user.ID, 1); err != nil {
		log.Printf("Failed to increment anchor count for user %s: %v", user.ID.Hex(), err)
	}

	notificationService := h.notificationService
	if notificationService == nil {
		notificationService = notifications.GetService(h.repo.db)
	}
	go func(cloneID, originalID primitive.ObjectID, title string, actorID, ownerID primitive.ObjectID) {
		if err := notificationService.CreateCloneNotification(context.Background(), cloneID, originalID, title, actorID, ownerID); err != nil {
			log.Printf("Failed to create clone notification: %v", err)
		}
	}(clone.ID, original.ID, original.Title, user.ID, original.UserID)

	response.Created(c, clone, "Anchor cloned successfully")
}

// GetAnchorClones godoc
// @Summary Get clones of an anchor
// @Description Get paginated list of clones for an anchor
// @Tags anchors
// @Produce json
// @Param id path string true "Anchor ID"
// @Param page query int false "Page number"
// @Param limit query int false "Limit per page"
// @Success 200 {object} response.APIResponse{data=AnchorClonesResponse}
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/clones [get]
func (h *Handler) GetAnchorClones(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	page, limit := pageParams(c, 20, 50)

	// Check anchor exists
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if !ensureAccess(c, anchor) {
		return
	}

	clones, total, err := h.repo.GetAnchorClones(c.Request.Context(), anchorID, page, limit)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch clones", "DATABASE_ERROR")
		return
	}

	// For clones, we might want to show who cloned it (UserID)
	// Collect user IDs
	userIDs := make([]primitive.ObjectID, len(clones))
	for i, clone := range clones {
		userIDs[i] = clone.UserID
	}

	usersList, _ := h.authRepo.GetUsersByIDs(c.Request.Context(), userIDs)
	usersMap := make(map[primitive.ObjectID]*auth.User)
	for i := range usersList {
		usersMap[usersList[i].ID] = &usersList[i]
	}

	items := make([]gin.H, 0)
	for _, clone := range clones {
		var clonerInfo gin.H
		if user, ok := usersMap[clone.UserID]; ok {
			clonerInfo = gin.H{
				"id":                user.ID,
				"username":          user.Username,
				"displayName":       user.DisplayName,
				"profilePictureUrl": user.ProfilePictureURL,
			}
		}

		items = append(items, gin.H{
			"id":        clone.ID,
			"title":     clone.Title,
			"createdAt": clone.CreatedAt,
			"cloner":    clonerInfo,
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

// TogglePin toggles the pinned status of an anchor
// @Summary Toggle pin status
// @Params id path string true "Anchor ID"
// @Router /anchors/{id}/pin [patch]
func (h *Handler) TogglePin(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission")
		return
	}

	// Check limit
	if !anchor.IsPinned {
		count, err := h.repo.CountPinnedAnchors(c.Request.Context(), user.ID)
		if err != nil {
			response.InternalServerError(c, "Database error", "DATABASE_ERROR")
			return
		}
		if count >= 3 {
			response.BadRequest(c, "You can only pin up to 3 anchors", "LIMIT_REACHED")
			return
		}
	}

	newStatus := !anchor.IsPinned

	err = h.repo.UpdateAnchor(c.Request.Context(), anchorID, map[string]interface{}{
		"$set": map[string]interface{}{"isPinned": newStatus},
	})
	if err != nil {
		response.InternalServerError(c, "Failed to update pin status", "DATABASE_ERROR")
		return
	}

	response.Success(c, gin.H{"isPinned": newStatus})
}
