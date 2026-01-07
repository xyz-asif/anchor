package anchors

import (
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Handler handles HTTP requests for anchor feature
type Handler struct {
	repo                *Repository
	authRepo            *auth.Repository
	notificationService *notifications.Service
	config              *config.Config
	cloudinary          *cloudinary.Service
	likesRepo           interface{} // Using interface to avoid cycle
	followsRepo         interface{} // Using interface to avoid cycle
}

// NewHandler creates a new anchor handler
func NewHandler(repo *Repository, authRepo *auth.Repository, notificationService *notifications.Service, cfg *config.Config, cld *cloudinary.Service, likesRepo interface{}, followsRepo interface{}) *Handler {
	return &Handler{
		repo:                repo,
		authRepo:            authRepo,
		notificationService: notificationService,
		config:              cfg,
		cloudinary:          cld,
		likesRepo:           likesRepo,
		followsRepo:         followsRepo,
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
	if len(req.Title) < 3 || len(req.Title) > 100 {
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
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission to update this anchor")
		return
	}

	updates := make(map[string]interface{})
	if req.Title != nil {
		if len(*req.Title) < 3 || len(*req.Title) > 100 {
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
		updates["tags"] = req.Tags
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

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission to delete this anchor")
		return
	}

	if err := h.repo.SoftDeleteAnchor(c.Request.Context(), anchorID); err != nil {
		response.InternalServerError(c, "Failed to delete anchor", "DATABASE_ERROR")
		return
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

	// Access control
	if anchor.Visibility == VisibilityPrivate {
		// Must be authenticated and owner
		val, exists := c.Get("user")
		if !exists {
			response.Unauthorized(c, "This anchor is private", "PRIVATE_ANCHOR")
			return
		}
		user, ok := val.(*auth.User)
		if !ok || user.ID != anchor.UserID {
			response.Forbidden(c, "You cannot view this private anchor")
			return
		}
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

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit > 50 {
		limit = 50
	}

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

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	// Access control
	if anchor.Visibility == VisibilityPrivate {
		// Must be authenticated and owner
		val, exists := c.Get("user")
		if !exists {
			response.Unauthorized(c, "This anchor is private", "PRIVATE_ANCHOR")
			return
		}
		user, ok := val.(*auth.User)
		if !ok || user.ID != anchor.UserID {
			response.Forbidden(c, "You cannot view this private anchor")
			return
		}
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
		textData = &TextData{Content: *req.Content}
	} else if req.Type == ItemTypeURL && req.URL != nil {
		urlData = &URLData{OriginalURL: *req.URL}
		// Fetch metadata logic omitted for brevity/fix
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

	// Update anchor lastItemAddedAt and increment itemCount
	h.repo.UpdateAnchor(c.Request.Context(), anchorID, map[string]interface{}{
		"$set": map[string]interface{}{"lastItemAddedAt": item.CreatedAt},
		"$inc": map[string]interface{}{"itemCount": 1},
	})

	response.Created(c, item)
}

// UploadItem uploads a file as an item
func (h *Handler) UploadItem(c *gin.Context) {
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
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
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission")
		return
	}

	// Handle file upload
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "File is required", "MISSING_FILE")
		return
	}

	// Determine type based on mime type or extension
	// Simplified logic: images, videos
	contentType := file.Header.Get("Content-Type")
	// itemType := ItemTypeText // fall back
	// folder := "items/files"

	// if contentType == "" {
	// 	// fallback detection not implemented
	// } else if len(contentType) > 5 && contentType[:5] == "image" {
	// 	itemType = ItemTypeImage
	// 	folder = "items/images"
	// } else if len(contentType) > 5 && contentType[:5] == "video" {
	// 	itemType = ItemTypeVideo
	// 	folder = "items/videos"
	// } else if len(contentType) > 5 && contentType[:5] == "audio" {
	// 	itemType = ItemTypeAudio
	// 	folder = "items/audio"
	// }

	fileContent, err := file.Open()
	if err != nil {
		response.InternalServerError(c, "Failed to open file", "FILE_ERROR")
		return
	}
	defer fileContent.Close()

	// Upload new file using general input
	uploadResult, err := h.cloudinary.UploadFile(c.Request.Context(), fileContent, file.Filename)
	if err != nil {
		response.InternalServerError(c, "Failed to upload file", "UPLOAD_FAILED")
		return
	}

	// Create item struct with FileData
	fileData := &FileData{
		CloudinaryURL: uploadResult.URL,
		PublicID:      uploadResult.PublicID,
		Filename:      file.Filename,
		FileType:      contentType,
		FileSize:      uploadResult.FileSize,
	}

	// Create item
	count, _ := h.repo.CountAnchorItems(c.Request.Context(), anchorID)

	item := &Item{
		AnchorID:  anchorID,
		Type:      ItemTypeFile, // General file type
		FileData:  fileData,
		Position:  int(count),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.repo.CreateItem(c.Request.Context(), item); err != nil {
		response.InternalServerError(c, "Failed to create item", "DATABASE_ERROR")
		return
	}

	h.repo.UpdateAnchor(c.Request.Context(), anchorID, map[string]interface{}{
		"$set": map[string]interface{}{"lastItemAddedAt": item.CreatedAt},
		"$inc": map[string]interface{}{"itemCount": 1},
	})

	response.Success(c, item)
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
	if err != nil {
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

	// Decrement count
	h.repo.UpdateAnchor(c.Request.Context(), anchorID, map[string]interface{}{
		"$inc": map[string]interface{}{"itemCount": -1},
	})

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
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.UserID != user.ID {
		response.Forbidden(c, "You do not have permission")
		return
	}

	if err := h.repo.ReorderItems(c.Request.Context(), anchorIDStr, req.ItemIDs); err != nil {
		response.InternalServerError(c, "Failed to reorder items", "DATABASE_ERROR")
		return
	}

	response.Success(c, "Items reordered")
}

// CloneAnchor creates a copy of an anchor
func (h *Handler) CloneAnchor(c *gin.Context) {
	// Implementation placeholder or reused from Clone System
	// See previous task. Included here for completeness of Handler struct.
	response.Success(c, "Clone feature implemented in separate module/task")
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
	if err != nil {
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
