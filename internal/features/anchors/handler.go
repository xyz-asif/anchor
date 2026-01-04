package anchors

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Handler handles anchor-related HTTP requests
type Handler struct {
	repo     *Repository
	authRepo *auth.Repository
	config   *config.Config
}

// NewHandler creates a new anchor handler
func NewHandler(repo *Repository, authRepo *auth.Repository, cfg *config.Config) *Handler {
	return &Handler{
		repo:     repo,
		authRepo: authRepo,
		config:   cfg,
	}
}

// CreateAnchor godoc
// @Summary Create a new anchor
// @Description Create a new anchor collection
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
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := user.(*auth.User)

	// Bind request
	var req CreateAnchorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", err.Error())
		return
	}

	// Validate request
	if err := ValidateCreateAnchorRequest(&req); err != nil {
		response.BadRequest(c, "VALIDATION_FAILED", err.Error())
		return
	}

	// Normalize tags
	normalizedTags := NormalizeTags(req.Tags)

	// Set defaults
	coverMediaType := "emoji"
	if req.CoverMediaType != nil {
		coverMediaType = *req.CoverMediaType
	}

	coverMediaValue := "⚓"
	if req.CoverMediaValue != nil {
		coverMediaValue = *req.CoverMediaValue
	}

	visibility := VisibilityPrivate
	if req.Visibility != nil {
		visibility = *req.Visibility
	}

	// Create anchor
	now := time.Now()
	anchor := &Anchor{
		ID:              primitive.NewObjectID(),
		UserID:          currentUser.ID,
		Title:           req.Title,
		Description:     req.Description,
		Tags:            normalizedTags,
		Visibility:      visibility,
		CoverMediaType:  coverMediaType,
		CoverMediaValue: coverMediaValue,
		IsPinned:        false,
		ItemCount:       0,
		LikeCount:       0,
		CloneCount:      0,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Save to database
	if err := h.repo.CreateAnchor(c.Request.Context(), anchor); err != nil {
		response.InternalServerError(c, "CREATE_FAILED", "Failed to create anchor")
		return
	}

	// Increment user's anchor count
	if err := h.authRepo.IncrementAnchorCount(c.Request.Context(), currentUser.ID, 1); err != nil {
		// Log error but don't fail the request
		// The anchor was created successfully
		c.Error(err)
	}

	response.Created(c, anchor)
}

// GetAnchor godoc
// @Summary Get anchor by ID
// @Description Get a specific anchor with its items
// @Tags anchors
// @Produce json
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse{data=AnchorWithItemsResponse}
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id} [get]
func (h *Handler) GetAnchor(c *gin.Context) {
	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	// Get anchor from database
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check if anchor is deleted
	if anchor.DeletedAt != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check access permissions
	var viewerID primitive.ObjectID
	user, exists := c.Get("user")
	if exists {
		currentUser := user.(*auth.User)
		viewerID = currentUser.ID
	}

	// If user is not authenticated, only allow public/unlisted anchors
	if !exists {
		if anchor.Visibility != VisibilityPublic && anchor.Visibility != VisibilityUnlisted {
			response.Forbidden(c, "ACCESS_DENIED", "Access denied")
			return
		}
	} else {
		// If user is authenticated, use CanBeViewed method
		if !anchor.CanBeViewed(viewerID) {
			response.Forbidden(c, "ACCESS_DENIED", "Access denied")
			return
		}
	}

	// Get anchor items
	items, err := h.repo.GetAnchorItems(c.Request.Context(), anchorID)
	if err != nil {
		response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch anchor items")
		return
	}

	// Build response
	anchorResponse := AnchorWithItemsResponse{
		Anchor: *anchor,
		Items:  items,
	}

	response.Success(c, anchorResponse)
}

// ListUserAnchors godoc
// @Summary List anchors
// @Description List anchors for a user
// @Tags anchors
// @Produce json
// @Param userId query string false "User ID (defaults to authenticated user)"
// @Success 200 {object} response.APIResponse{data=[]Anchor}
// @Failure 400 {object} response.APIResponse
// @Router /anchors [get]
func (h *Handler) ListUserAnchors(c *gin.Context) {
	// Get userId from query params
	userIDStr := c.Query("userId")

	var targetUserID primitive.ObjectID
	var viewerID primitive.ObjectID
	var isViewingOwnAnchors bool

	// Get authenticated user if exists
	user, isAuthenticated := c.Get("user")
	if isAuthenticated {
		currentUser := user.(*auth.User)
		viewerID = currentUser.ID
	}

	// Determine target user
	if userIDStr == "" {
		// No userId provided, must be authenticated
		if !isAuthenticated {
			response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
			return
		}
		targetUserID = viewerID
		isViewingOwnAnchors = true
	} else {
		// userId provided
		var err error
		targetUserID, err = primitive.ObjectIDFromHex(userIDStr)
		if err != nil {
			response.BadRequest(c, "INVALID_USER_ID", "Invalid user ID")
			return
		}

		// Check if viewing own anchors
		if isAuthenticated && targetUserID == viewerID {
			isViewingOwnAnchors = true
		}
	}

	// Fetch anchors based on permissions
	var anchors []Anchor
	var err error

	if isViewingOwnAnchors {
		// Show all anchors (including private)
		anchors, err = h.repo.GetUserAnchors(c.Request.Context(), targetUserID)
	} else {
		// Show only public and unlisted anchors
		anchors, err = h.repo.GetPublicUserAnchors(c.Request.Context(), targetUserID)
	}

	if err != nil {
		response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch anchors")
		return
	}

	response.Success(c, anchors)
}

// UpdateAnchor godoc
// @Summary Update anchor
// @Description Update anchor details
// @Tags anchors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param request body UpdateAnchorRequest true "Update fields"
// @Success 200 {object} response.APIResponse{data=Anchor}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id} [patch]
func (h *Handler) UpdateAnchor(c *gin.Context) {
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := user.(*auth.User)

	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	// Bind request
	var req UpdateAnchorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", err.Error())
		return
	}

	// Get anchor from database
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check ownership
	if !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "NOT_OWNER", "You don't own this anchor")
		return
	}

	// Validate updated fields
	if err := ValidateUpdateAnchorRequest(&req); err != nil {
		response.BadRequest(c, "VALIDATION_FAILED", err.Error())
		return
	}

	// Build updates map
	updates := bson.M{
		"updatedAt": time.Now(),
	}

	if req.Title != nil {
		updates["title"] = *req.Title
	}

	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if req.Tags != nil {
		normalizedTags := NormalizeTags(req.Tags)
		updates["tags"] = normalizedTags
	}

	if req.Visibility != nil {
		updates["visibility"] = *req.Visibility
	}

	if req.CoverMediaType != nil {
		updates["coverMediaType"] = *req.CoverMediaType
	}

	if req.CoverMediaValue != nil {
		updates["coverMediaValue"] = *req.CoverMediaValue
	}

	// Update anchor in database
	if err := h.repo.UpdateAnchor(c.Request.Context(), anchorID, updates); err != nil {
		response.InternalServerError(c, "UPDATE_FAILED", "Failed to update anchor")
		return
	}

	// Fetch updated anchor
	updatedAnchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch updated anchor")
		return
	}

	response.Success(c, updatedAnchor)
}

// DeleteAnchor godoc
// @Summary Delete anchor
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
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := user.(*auth.User)

	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	// Get anchor from database
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check ownership
	if !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "NOT_OWNER", "You don't own this anchor")
		return
	}

	// Soft delete anchor
	if err := h.repo.SoftDeleteAnchor(c.Request.Context(), anchorID); err != nil {
		response.InternalServerError(c, "DELETE_FAILED", "Failed to delete anchor")
		return
	}

	// Decrement user's anchor count
	if err := h.authRepo.IncrementAnchorCount(c.Request.Context(), currentUser.ID, -1); err != nil {
		// Log error but don't fail the request
		c.Error(err)
	}

	response.Success(c, gin.H{
		"message": "Anchor deleted successfully",
	})
}

// TogglePin godoc
// @Summary Toggle pin status
// @Description Pin or unpin an anchor
// @Tags anchors
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse{data=Anchor}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Router /anchors/{id}/pin [patch]
func (h *Handler) TogglePin(c *gin.Context) {
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := user.(*auth.User)

	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	// Get anchor from database
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check ownership
	if !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "NOT_OWNER", "You don't own this anchor")
		return
	}

	var newPinStatus bool

	if anchor.IsPinned {
		// Unpin the anchor
		newPinStatus = false
	} else {
		// Pin the anchor - need to check constraints
		// Check if user already has 3 pinned anchors
		pinnedCount, err := h.repo.CountPinnedAnchors(c.Request.Context(), currentUser.ID)
		if err != nil {
			response.InternalServerError(c, "CHECK_FAILED", "Failed to check pinned anchors")
			return
		}

		if pinnedCount >= 3 {
			response.BadRequest(c, "MAX_PINS_REACHED", "You can only pin up to 3 anchors")
			return
		}

		// Check if anchor is private
		if anchor.Visibility == VisibilityPrivate {
			response.BadRequest(c, "CANNOT_PIN_PRIVATE", "Cannot pin private anchors")
			return
		}

		newPinStatus = true
	}

	// Update pin status
	updates := bson.M{
		"isPinned":  newPinStatus,
		"updatedAt": time.Now(),
	}

	if err := h.repo.UpdateAnchor(c.Request.Context(), anchorID, updates); err != nil {
		response.InternalServerError(c, "UPDATE_FAILED", "Failed to update pin status")
		return
	}

	// Fetch updated anchor
	updatedAnchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch updated anchor")
		return
	}

	response.Success(c, updatedAnchor)
}

// AddItem godoc
// @Summary Add item to anchor
// @Description Add a new item (URL or text) to an anchor
// @Tags anchors
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param request body AddItemRequest true "Item details"
// @Success 201 {object} response.APIResponse{data=Item}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Router /anchors/{id}/items [post]
func (h *Handler) AddItem(c *gin.Context) {
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := user.(*auth.User)

	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	// Bind request
	var req AddItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", err.Error())
		return
	}

	// Get anchor from database
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check ownership
	if !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "NOT_OWNER", "You don't own this anchor")
		return
	}

	// Check if anchor has reached maximum items
	if anchor.ItemCount >= 100 {
		response.BadRequest(c, "MAX_ITEMS_REACHED", "Anchor has reached maximum of 100 items")
		return
	}

	// Validate item type
	if err := ValidateAddItemRequest(&req); err != nil {
		response.BadRequest(c, "VALIDATION_FAILED", err.Error())
		return
	}

	// Create item based on type
	now := time.Now()
	item := &Item{
		ID:        primitive.NewObjectID(),
		AnchorID:  anchorID,
		Type:      req.Type,
		Position:  anchor.ItemCount, // Next position
		CreatedAt: now,
		UpdatedAt: now,
	}

	switch req.Type {
	case ItemTypeURL:
		if req.URL == nil {
			response.BadRequest(c, "MISSING_URL", "URL is required for URL type items")
			return
		}
		item.URLData = &URLData{
			OriginalURL: *req.URL,
			// Metadata will be fetched later
		}

	case ItemTypeText:
		if req.Content == nil {
			response.BadRequest(c, "MISSING_CONTENT", "Content is required for text type items")
			return
		}
		item.TextData = &TextData{
			Content: *req.Content,
		}

	case ItemTypeImage, ItemTypeAudio, ItemTypeFile:
		response.BadRequest(c, "NOT_IMPLEMENTED", "File upload not yet implemented")
		return

	default:
		response.BadRequest(c, "INVALID_TYPE", "Invalid item type")
		return
	}

	// Save item to database
	if err := h.repo.CreateItem(c.Request.Context(), item); err != nil {
		response.InternalServerError(c, "CREATE_FAILED", "Failed to create item")
		return
	}

	// Update anchor's itemCount and lastItemAddedAt
	updates := bson.M{
		"$inc": bson.M{"itemCount": 1},
		"$set": bson.M{
			"lastItemAddedAt": now,
			"updatedAt":       now,
		},
	}

	if err := h.repo.UpdateAnchor(c.Request.Context(), anchorID, updates); err != nil {
		// Log error but don't fail the request
		// The item was created successfully
		c.Error(err)
	}

	response.Created(c, item)
}

// DeleteItem godoc
// @Summary Delete item from anchor
// @Description Remove an item from an anchor
// @Tags anchors
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param itemId path string true "Item ID"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Router /anchors/{id}/items/{itemId} [delete]
func (h *Handler) DeleteItem(c *gin.Context) {
	// Extract authenticated user
	user, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := user.(*auth.User)

	// Get anchor ID from path
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	// Get item ID from path
	itemIDStr := c.Param("itemId")
	itemID, err := primitive.ObjectIDFromHex(itemIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ITEM_ID", "Invalid item ID")
		return
	}

	// Get anchor from database
	anchor, err := h.repo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check ownership
	if !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "NOT_OWNER", "You don't own this anchor")
		return
	}

	// Get item from database
	item, err := h.repo.GetItemByID(c.Request.Context(), itemID)
	if err != nil {
		response.NotFound(c, "ITEM_NOT_FOUND", "Item not found")
		return
	}

	// Check if item belongs to this anchor
	if item.AnchorID != anchorID {
		response.BadRequest(c, "ITEM_MISMATCH", "Item does not belong to this anchor")
		return
	}

	// Delete item from database
	if err := h.repo.DeleteItem(c.Request.Context(), itemID); err != nil {
		response.InternalServerError(c, "DELETE_FAILED", "Failed to delete item")
		return
	}

	// Update anchor's itemCount
	updates := bson.M{
		"$inc": bson.M{"itemCount": -1},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	if err := h.repo.UpdateAnchor(c.Request.Context(), anchorID, updates); err != nil {
		// Log error but don't fail the request
		c.Error(err)
	}

	response.Success(c, gin.H{
		"message": "Item deleted successfully",
	})
}
