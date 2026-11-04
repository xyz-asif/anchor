package anchors

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/pkg/bind"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
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

// maxPinnedAnchors is how many anchors a user can pin to their profile
const maxPinnedAnchors = 3

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
	user, ok := requireUser(c, "create an anchor")
	if !ok {
		return
	}
	var req CreateAnchorRequest
	if !bind.JSON(c, &req) {
		return
	}

	title := strings.TrimSpace(req.Title)
	if err := ValidateAnchorTitle(title); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}
	description := strings.TrimSpace(req.Description)
	if err := ValidateDescription(description); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	visibility := VisibilityPublic
	if req.Visibility != nil {
		visibility = *req.Visibility // checked by the binding's oneof rule
	}

	var coverType, coverValue string
	if req.CoverMediaType != nil {
		coverType = *req.CoverMediaType
	}
	if req.CoverMediaValue != nil {
		coverValue = strings.TrimSpace(*req.CoverMediaValue)
	}
	if err := ValidateCover(coverType, coverValue); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	tagList, err := NormalizeAndValidateTags(req.Tags)
	if err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	anchor := &Anchor{
		UserID:          user.ID,
		Title:           title,
		Description:     description,
		CoverMediaType:  coverType,
		CoverMediaValue: coverValue,
		Visibility:      visibility,
		Tags:            tagList,
	}

	if err := h.repo.CreateAnchor(c.Request.Context(), anchor); err != nil {
		log.Printf("anchors: create failed: %v", err)
		response.InternalServerError(c, "We couldn't create your anchor right now. Please try again", "DATABASE_ERROR")
		return
	}

	if err := h.authRepo.IncrementAnchorCount(c.Request.Context(), user.ID, 1); err != nil {
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
	user, ok := requireUser(c, "edit an anchor")
	if !ok {
		return
	}
	anchor, ok := h.loadOwnedAnchor(c, user)
	if !ok {
		return
	}
	var req UpdateAnchorRequest
	if !bind.JSON(c, &req) {
		return
	}

	updates := bson.M{}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if err := ValidateAnchorTitle(title); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		updates["title"] = title
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if err := ValidateDescription(description); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		updates["description"] = description
	}
	if req.CoverMediaType != nil || req.CoverMediaValue != nil {
		// The cover's type and value must still fit together after the change
		coverType, coverValue := anchor.CoverMediaType, anchor.CoverMediaValue
		if req.CoverMediaType != nil {
			coverType = *req.CoverMediaType
		}
		if req.CoverMediaValue != nil {
			coverValue = strings.TrimSpace(*req.CoverMediaValue)
		}
		if coverType == "" {
			coverValue = "" // removing the cover type removes the cover
		}
		if err := ValidateCover(coverType, coverValue); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		updates["coverMediaType"] = coverType
		updates["coverMediaValue"] = coverValue
	}
	if req.Visibility != nil {
		updates["visibility"] = *req.Visibility
	}
	if req.Tags != nil {
		tagList, err := NormalizeAndValidateTags(req.Tags)
		if err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		updates["tags"] = tagList
	}

	if len(updates) == 0 {
		response.Success(c, anchor)
		return
	}
	updates["updatedAt"] = time.Now()

	ctx := c.Request.Context()
	if err := h.repo.UpdateAnchor(ctx, anchor.ID, updates); err != nil {
		log.Printf("anchors: update %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't save your changes right now. Please try again", "DATABASE_ERROR")
		return
	}

	updatedAnchor, err := h.repo.GetAnchorByID(ctx, anchor.ID)
	if err != nil {
		log.Printf("anchors: re-reading %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "Your changes were saved, but we couldn't load the updated anchor. Please refresh", "DATABASE_ERROR")
		return
	}

	response.Success(c, updatedAnchor)
}

// normalizeTags converts tags to the canonical form shared with interests and tag feeds
func normalizeTags(raw []string) []string {
	if raw == nil {
		return nil
	}
	tagList, _ := NormalizeAndValidateTags(raw)
	return tagList
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
	user, ok := requireUser(c, "delete an anchor")
	if !ok {
		return
	}
	anchor, ok := h.loadOwnedAnchor(c, user)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Atomic: of two concurrent deletes only one succeeds, so counters change once
	if err := h.repo.SoftDeleteAnchor(ctx, anchor.ID); err != nil {
		if errors.Is(err, ErrAnchorNotFound) {
			response.NotFound(c, "This anchor has already been deleted", "ANCHOR_NOT_FOUND")
			return
		}
		log.Printf("anchors: delete %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't delete this anchor right now. Please try again", "DATABASE_ERROR")
		return
	}

	// Deleting a clone lowers the original's clone count (and therefore its ranking score)
	if anchor.ClonedFromAnchorID != nil {
		if err := h.repo.IncrementCloneCount(ctx, *anchor.ClonedFromAnchorID, -1); err != nil {
			log.Printf("Failed to decrement clone count for anchor %s: %v", anchor.ClonedFromAnchorID.Hex(), err)
		}
		if err := h.repo.UpdateEngagementScore(ctx, *anchor.ClonedFromAnchorID); err != nil {
			log.Printf("Failed to update engagement score for anchor %s: %v", anchor.ClonedFromAnchorID.Hex(), err)
		}
	}

	if err := h.authRepo.IncrementAnchorCount(ctx, user.ID, -1); err != nil {
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
	anchor, ok := h.loadViewableAnchor(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	viewer := currentUser(c)

	items, err := h.repo.GetAnchorItems(ctx, anchor.ID)
	if err != nil {
		log.Printf("anchors: items of %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't load this anchor's items right now. Please try again", "DATABASE_ERROR")
		return
	}
	if items == nil {
		items = []Item{}
	}

	if err := h.hideCloneSources(ctx, viewer, []*Anchor{anchor}); err != nil {
		log.Printf("anchors: clone source check failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return
	}

	// Update last seen version for authenticated user (async)
	if viewer != nil && h.anchorFollowService != nil {
		go func(uid, aid primitive.ObjectID, ver int) {
			_ = h.anchorFollowService.UpdateLastSeenVersion(context.Background(), uid, aid, ver)
		}(viewer.ID, anchor.ID, anchor.Version)
	}

	response.Success(c, AnchorWithItemsResponse{
		Anchor: *anchor,
		Items:  items,
	})
}

// ListUserAnchors lists anchors for a user
// @Summary List user anchors
// @Description List a user's anchors: all of them for the owner, public ones for everyone else
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
		response.BadRequest(c, "Please say whose anchors to list: add ?userId=<user ID>", "MISSING_PARAM")
		return
	}
	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		response.BadRequest(c, `"`+userIDStr+`" isn't a valid user ID`, "INVALID_ID")
		return
	}

	page, limit := pageParams(c, 10, 50)
	ctx := c.Request.Context()
	viewer := currentUser(c)
	isOwner := viewer != nil && viewer.ID == userID

	// Someone who blocked the viewer looks like they don't exist, here as on their profile
	if viewer != nil && !isOwner {
		blocked, err := h.authRepo.UserHasBlocked(ctx, userID, viewer.ID)
		if err != nil {
			log.Printf("anchors: block check failed: %v", err)
			response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
			return
		}
		if blocked {
			response.NotFound(c, "This user doesn't exist or has deleted their account", "USER_NOT_FOUND")
			return
		}
	}

	var anchors []Anchor
	var total int64
	if isOwner {
		anchors, total, err = h.repo.GetUserAnchors(ctx, userID, page, limit)
	} else {
		anchors, total, err = h.repo.GetPublicUserAnchors(ctx, userID, page, limit)
	}
	if err != nil {
		log.Printf("anchors: listing anchors of %s failed: %v", userID.Hex(), err)
		response.InternalServerError(c, "We couldn't load these anchors right now. Please try again", "DATABASE_ERROR")
		return
	}
	if anchors == nil {
		anchors = []Anchor{}
	}

	ptrs := make([]*Anchor, len(anchors))
	for i := range anchors {
		ptrs[i] = &anchors[i]
	}
	if err := h.hideCloneSources(ctx, viewer, ptrs); err != nil {
		log.Printf("anchors: clone source check failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return
	}

	resp := PaginatedResponse{Data: anchors}
	resp.Pagination.Page = page
	resp.Pagination.Limit = limit
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = int((total + int64(limit) - 1) / int64(limit))
	resp.Pagination.HasMore = int64(page)*int64(limit) < total
	response.Success(c, resp)
}

// ListAnchorItems lists items for an anchor with pagination
func (h *Handler) ListAnchorItems(c *gin.Context) {
	anchor, ok := h.loadViewableAnchor(c)
	if !ok {
		return
	}
	page, limit := pageParams(c, 20, 100)

	items, total, err := h.repo.GetAnchorItemsPaginated(c.Request.Context(), anchor.ID, page, limit)
	if err != nil {
		log.Printf("anchors: items page of %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't load this anchor's items right now. Please try again", "DATABASE_ERROR")
		return
	}
	if items == nil {
		items = []Item{}
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
	user, ok := requireUser(c, "add items")
	if !ok {
		return
	}
	var req AddItemRequest
	if !bind.JSON(c, &req) {
		return
	}

	// Media items must go through the upload endpoint
	if req.Type == ItemTypeImage || req.Type == ItemTypeAudio || req.Type == ItemTypeFile {
		response.BadRequest(c, "Images, audio and files are added by uploading them to POST /anchors/{id}/items/upload", "USE_UPLOAD_ENDPOINT")
		return
	}
	if err := ValidateAddItemRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	anchor, ok := h.loadOwnedAnchor(c, user)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	count, err := h.repo.CountAnchorItems(ctx, anchor.ID)
	if err != nil {
		log.Printf("anchors: counting items of %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't add this item right now. Please try again", "DATABASE_ERROR")
		return
	}

	var textData *TextData
	var urlData *URLData
	if req.Type == ItemTypeText {
		textData = &TextData{Content: strings.TrimSpace(*req.Content)}
	} else {
		urlData = fetchLinkPreview(ctx, strings.TrimSpace(*req.URL))
	}

	item := &Item{
		AnchorID: anchor.ID,
		Type:     req.Type,
		Position: int(count),
		TextData: textData,
		URLData:  urlData,
	}

	if err := h.repo.CreateItem(ctx, item); err != nil {
		log.Printf("anchors: creating item in %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't add this item right now. Please try again", "DATABASE_ERROR")
		return
	}

	h.afterItemAdded(ctx, anchor, item, user.ID)

	response.Created(c, item)
}

// fetchLinkPreview fills title/description/favicon/thumbnail from the page (best effort:
// timeouts, blocked addresses or non-HTML pages keep just the URL)
func fetchLinkPreview(ctx context.Context, targetURL string) *URLData {
	fetchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	meta, err := FetchURLMetadata(fetchCtx, targetURL)
	if err != nil || meta == nil {
		if err != nil {
			log.Printf("URL metadata fetch failed for %s: %v", targetURL, err)
		}
		return &URLData{OriginalURL: targetURL}
	}
	meta.OriginalURL = targetURL
	return meta
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

// maxFilenameLength bounds stored original filenames (characters)
const maxFilenameLength = 200

// cleanFilename keeps only the base name of an uploaded file (clients may send full paths
// with / or \), drops control/invisible characters and caps the length.
func cleanFilename(name, ext string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > maxFilenameLength {
		// keep the extension visible when shortening
		keep := maxFilenameLength - utf8.RuneCountInString(ext)
		if keep < 1 {
			keep = maxFilenameLength
		}
		name = string(r[:keep]) + ext
	}
	if name == "" || name == "." || name == ".." {
		name = "file" + ext
	}
	return name
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
	user, ok := requireUser(c, "upload files")
	if !ok {
		return
	}
	if h.cloudinary == nil {
		response.ServiceUnavailable(c, "File uploads aren't available right now", "UPLOADS_UNAVAILABLE")
		return
	}

	// Check ownership before reading a possibly large body
	anchor, ok := h.loadOwnedAnchor(c, user)
	if !ok {
		return
	}

	// Cap the request body at the largest allowed file size (+1MB for multipart overhead)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cloudinary.MaxFileSize+(1<<20))

	file, err := c.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			response.Error(c, http.StatusRequestEntityTooLarge, "This file is too large. The maximum is 10MB for images, 25MB for audio and 50MB for documents", "FILE_TOO_LARGE")
			return
		}
		response.BadRequest(c, `Please attach the file as multipart form field "file"`, "MISSING_FILE")
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
		log.Printf("anchors: opening upload failed: %v", err)
		response.InternalServerError(c, "We couldn't read the uploaded file. Please try again", "FILE_ERROR")
		return
	}
	defer fileContent.Close()

	// Documents are stored as-is, so check they really are what the extension says
	if itemType == ItemTypeFile {
		if err := cloudinary.CheckDocumentContent(fileContent, file.Filename); err != nil {
			response.BadRequest(c, err.Error(), "INVALID_FILE")
			return
		}
	}

	ctx := c.Request.Context()
	filename := cleanFilename(file.Filename, ext)
	item := &Item{AnchorID: anchor.ID, Type: itemType}

	switch itemType {
	case ItemTypeImage:
		result, err := h.cloudinary.UploadImage(ctx, fileContent, filename)
		if err != nil {
			log.Printf("Image upload failed: %v", err)
			response.InternalServerError(c, "We couldn't upload this image. Please check it's a valid image file and try again", "UPLOAD_FAILED")
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
		result, err := h.cloudinary.UploadAudio(ctx, fileContent, filename)
		if err != nil {
			log.Printf("Audio upload failed: %v", err)
			response.InternalServerError(c, "We couldn't upload this audio. Please check it's a valid audio file and try again", "UPLOAD_FAILED")
			return
		}
		item.AudioData = &AudioData{
			CloudinaryURL: result.URL,
			PublicID:      result.PublicID,
			Duration:      int(math.Round(result.Duration)),
			FileSize:      result.FileSize,
		}
	default:
		result, err := h.cloudinary.UploadFile(ctx, fileContent, filename)
		if err != nil {
			log.Printf("File upload failed: %v", err)
			response.InternalServerError(c, "We couldn't upload this file. Please try again", "UPLOAD_FAILED")
			return
		}
		item.FileData = &FileData{
			CloudinaryURL: result.URL,
			PublicID:      result.PublicID,
			Filename:      filename,
			FileType:      strings.TrimPrefix(ext, "."),
			FileSize:      result.FileSize,
		}
	}

	count, err := h.repo.CountAnchorItems(ctx, anchor.ID)
	if err != nil {
		h.deleteItemAsset(ctx, item)
		log.Printf("anchors: counting items of %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't add this item right now. Please try again", "DATABASE_ERROR")
		return
	}
	item.Position = int(count)

	if err := h.repo.CreateItem(ctx, item); err != nil {
		// Don't leave an orphaned asset behind
		h.deleteItemAsset(ctx, item)
		log.Printf("anchors: creating item in %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't add this item right now. Please try again", "DATABASE_ERROR")
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
	var publicID, resourceType, url string
	switch {
	case item.ImageData != nil:
		publicID, resourceType, url = item.ImageData.PublicID, "image", item.ImageData.CloudinaryURL
	case item.AudioData != nil:
		publicID, resourceType, url = item.AudioData.PublicID, "video", item.AudioData.CloudinaryURL // Cloudinary stores audio as "video"
	case item.FileData != nil:
		publicID, resourceType, url = item.FileData.PublicID, "raw", item.FileData.CloudinaryURL
	}
	if publicID == "" {
		return
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
	user, ok := requireUser(c, "edit items")
	if !ok {
		return
	}
	anchor, ok := h.loadOwnedAnchor(c, user)
	if !ok {
		return
	}
	item, ok := h.loadItemOf(c, anchor.ID)
	if !ok {
		return
	}
	var req UpdateItemRequest
	if !bind.JSON(c, &req) {
		return
	}
	ctx := c.Request.Context()

	set := bson.M{}
	switch item.Type {
	case ItemTypeText:
		if req.Content == nil {
			response.BadRequest(c, `Please send the new text as "content"`, "VALIDATION_FAILED")
			return
		}
		if err := ValidateTextContent(*req.Content); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		set["textData"] = TextData{Content: strings.TrimSpace(*req.Content)}
	case ItemTypeURL:
		if req.URL == nil {
			response.BadRequest(c, `Please send the new link as "url"`, "VALIDATION_FAILED")
			return
		}
		targetURL := strings.TrimSpace(*req.URL)
		if err := ValidateURL(targetURL); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		set["urlData"] = fetchLinkPreview(ctx, targetURL)
	default:
		response.BadRequest(c, "Images, audio and files can't be edited. Delete this item and upload the new file instead", "ITEM_NOT_EDITABLE")
		return
	}

	updated, err := h.repo.UpdateItem(ctx, item.ID, set)
	if err != nil {
		log.Printf("anchors: updating item %s failed: %v", item.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't save this item right now. Please try again", "DATABASE_ERROR")
		return
	}

	if err := h.repo.UpdateAnchor(ctx, anchor.ID, map[string]interface{}{"updatedAt": time.Now()}); err != nil {
		log.Printf("Failed to touch anchor %s after item edit: %v", anchor.ID.Hex(), err)
	}

	response.Success(c, updated)
}

// DeleteItem deletes an item
func (h *Handler) DeleteItem(c *gin.Context) {
	user, ok := requireUser(c, "delete items")
	if !ok {
		return
	}
	anchor, ok := h.loadOwnedAnchor(c, user)
	if !ok {
		return
	}
	item, ok := h.loadItemOf(c, anchor.ID)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	if err := h.repo.DeleteItem(ctx, item.ID); err != nil {
		if errors.Is(err, ErrItemNotFound) {
			response.NotFound(c, "This item has already been deleted", "ITEM_NOT_FOUND")
			return
		}
		log.Printf("anchors: deleting item %s failed: %v", item.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't delete this item right now. Please try again", "DATABASE_ERROR")
		return
	}

	// Remove the uploaded file too (unless a clone still points at it)
	h.deleteItemAsset(ctx, item)

	// Close the gap in positions and set itemCount to the real number of items
	if err := h.repo.RenumberItems(ctx, anchor.ID); err != nil {
		log.Printf("Failed to renumber items of anchor %s: %v", anchor.ID.Hex(), err)
	}

	response.Success(c, "Item deleted")
}

// ReorderItems updates the order of items
func (h *Handler) ReorderItems(c *gin.Context) {
	user, ok := requireUser(c, "reorder items")
	if !ok {
		return
	}
	var req ReorderItemsRequest
	if !bind.JSON(c, &req) {
		return
	}
	anchor, ok := h.loadOwnedAnchor(c, user)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Check the whole list before writing anything: a bad ID halfway through must not leave
	// the anchor half reordered
	existing, err := h.repo.ItemIDs(ctx, anchor.ID)
	if err != nil {
		log.Printf("anchors: loading item IDs of %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't reorder the items right now. Please try again", "DATABASE_ERROR")
		return
	}
	if err := ValidateReorderItemsRequest(&req, len(existing)); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}
	for _, raw := range req.ItemIDs {
		id, err := primitive.ObjectIDFromHex(raw)
		if err != nil {
			response.BadRequest(c, fmt.Sprintf(`"%s" isn't a valid item ID`, raw), "VALIDATION_FAILED")
			return
		}
		if !existing[id] {
			response.BadRequest(c, fmt.Sprintf(`Item "%s" isn't in this anchor`, raw), "VALIDATION_FAILED")
			return
		}
	}

	if err := h.repo.ReorderItems(ctx, anchor.ID.Hex(), req.ItemIDs); err != nil {
		log.Printf("anchors: reordering %s failed: %v", anchor.ID.Hex(), err)
		response.Error(c, http.StatusConflict, "The items changed while you were reordering them. Please reload and try again", "ITEMS_CHANGED")
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
	user, ok := requireUser(c, "clone anchors")
	if !ok {
		return
	}

	// Body is optional
	var req CloneAnchorRequest
	if c.Request.ContentLength > 0 {
		if !bind.JSON(c, &req) {
			return
		}
	}

	original, ok := h.loadAnchor(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Private anchors are reported as not found so their existence isn't revealed
	if original.DeletedAt != nil || (original.Visibility == VisibilityPrivate && original.UserID != user.ID) {
		response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		return
	}
	if original.UserID == user.ID {
		response.BadRequest(c, "This is your own anchor, so there's nothing to clone. You can edit it directly", "CANNOT_CLONE_OWN")
		return
	}

	// Blocked users (either direction) can't clone each other's anchors
	if blocked, err := h.authRepo.IsBlockedBetween(ctx, user, original.UserID); err != nil {
		log.Printf("anchors: block check failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return
	} else if blocked {
		if user.HasBlocked(original.UserID) {
			response.Forbidden(c, "You've blocked the owner of this anchor. Unblock them first if you want to clone it", "USER_BLOCKED")
		} else {
			response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		}
		return
	}

	existing, err := h.repo.FindUserCloneOf(ctx, user.ID, original.ID)
	if err != nil {
		log.Printf("anchors: clone lookup failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return
	}
	if existing != nil {
		response.Respond(c, http.StatusConflict, false, "You've already cloned this anchor. Open your copy instead",
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
		response.BadRequest(c, "This anchor is shared by link only, so your copy can be unlisted or private but not public", "VISIBILITY_NOT_ALLOWED")
		return
	}

	items, err := h.repo.GetAnchorItems(ctx, original.ID)
	if err != nil {
		log.Printf("anchors: reading items of %s failed: %v", original.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't clone this anchor right now. Please try again", "DATABASE_ERROR")
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

	if err := h.repo.CreateAnchor(ctx, clone); err != nil {
		log.Printf("anchors: creating clone failed: %v", err)
		response.InternalServerError(c, "We couldn't clone this anchor right now. Please try again", "DATABASE_ERROR")
		return
	}

	// Guard against double-submits: if two requests raced past the "already cloned" check,
	// keep only the oldest clone and roll this one back.
	cloneIDs, err := h.repo.ActiveCloneIDs(ctx, user.ID, original.ID)
	if err == nil && len(cloneIDs) > 1 && cloneIDs[0] != clone.ID {
		if delErr := h.repo.DeleteAnchor(ctx, clone.ID); delErr != nil {
			log.Printf("Failed to roll back duplicate clone %s: %v", clone.ID.Hex(), delErr)
		}
		response.Respond(c, http.StatusConflict, false, "You've already cloned this anchor. Open your copy instead",
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
			log.Printf("anchors: copying items into clone failed: %v", err)
			response.InternalServerError(c, "We couldn't copy this anchor's items. Please try again", "DATABASE_ERROR")
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
	anchor, ok := h.loadViewableAnchor(c)
	if !ok {
		return
	}
	page, limit := pageParams(c, 20, 50)
	ctx := c.Request.Context()

	// Clones by people in a block relationship with the viewer are left out
	hidden, err := h.authRepo.HiddenUserIDs(ctx, currentUser(c))
	if err != nil {
		log.Printf("anchors: loading blocks failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return
	}

	clones, total, err := h.repo.GetAnchorClones(ctx, anchor.ID, hidden, page, limit)
	if err != nil {
		log.Printf("anchors: listing clones of %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't load the copies of this anchor right now. Please try again", "DATABASE_ERROR")
		return
	}

	userIDs := make([]primitive.ObjectID, len(clones))
	for i, clone := range clones {
		userIDs[i] = clone.UserID
	}
	usersList, err := h.authRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		log.Printf("anchors: loading cloners failed: %v", err)
		response.InternalServerError(c, "We couldn't load the copies of this anchor right now. Please try again", "DATABASE_ERROR")
		return
	}
	usersMap := make(map[primitive.ObjectID]*auth.User)
	for i := range usersList {
		usersMap[usersList[i].ID] = &usersList[i]
	}

	items := make([]gin.H, 0, len(clones))
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
	user, ok := requireUser(c, "pin anchors")
	if !ok {
		return
	}
	anchor, ok := h.loadOwnedAnchor(c, user)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	pinFailed := "We couldn't change the pin right now. Please try again"

	newStatus := !anchor.IsPinned
	// Atomic flip from the state we read: a double tap can't toggle twice
	changed, err := h.repo.SetPinned(ctx, anchor.ID, anchor.IsPinned, newStatus)
	if err != nil {
		log.Printf("anchors: pin %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, pinFailed, "DATABASE_ERROR")
		return
	}
	if !changed {
		// A concurrent request already changed it; report the current state
		response.Success(c, gin.H{"isPinned": newStatus})
		return
	}

	if newStatus {
		// Count *after* pinning, so concurrent pins of different anchors can't exceed the limit
		count, err := h.repo.CountPinnedAnchors(ctx, user.ID)
		if err != nil || count > maxPinnedAnchors {
			if _, revertErr := h.repo.SetPinned(ctx, anchor.ID, true, false); revertErr != nil {
				log.Printf("anchors: reverting pin of %s failed: %v", anchor.ID.Hex(), revertErr)
			}
			if err != nil {
				log.Printf("anchors: counting pins failed: %v", err)
				response.InternalServerError(c, pinFailed, "DATABASE_ERROR")
			} else {
				response.BadRequest(c, fmt.Sprintf("You can pin up to %d anchors. Unpin one first to pin this one", maxPinnedAnchors), "LIMIT_REACHED")
			}
			return
		}
	}

	response.Success(c, gin.H{"isPinned": newStatus})
}
