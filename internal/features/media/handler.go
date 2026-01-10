package media

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/features/anchors" // Imported for Scraper
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

type Handler struct {
	cloudinary *cloudinary.Service
}

func NewHandler(cld *cloudinary.Service) *Handler {
	return &Handler{
		cloudinary: cld,
	}
}

// @Summary Upload media
// @Description Upload a file to Cloudinary (image, audio, or raw)
// @Tags media
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "File to upload"
// @Success 200 {object} response.APIResponse{data=cloudinary.UploadResult}
// @Router /media/upload [post]
func (h *Handler) UploadMedia(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "File is required", "MISSING_FILE")
		return
	}
	defer file.Close()

	// Detect content type
	contentType := header.Header.Get("Content-Type")
	var result *cloudinary.UploadResult

	// Simple logic: if audio -> audio, if image -> image, else -> file
	// Using header.Header.Get("Content-Type") might be empty, relying on extension validation in Service
	// But Service Upload methods take `multipart.File`.
	// I need to decide which method to call.

	// Check extension logic or mime type
	// For simplicity, checking Content-Type start
	if len(contentType) >= 5 && contentType[:5] == "image" {
		if err := cloudinary.ValidateImageFile(header); err != nil {
			response.BadRequest(c, err.Error(), "INVALID_FILE")
			return
		}
		result, err = h.cloudinary.UploadImage(c.Request.Context(), file, header.Filename)
	} else if len(contentType) >= 5 && contentType[:5] == "audio" {
		if err := cloudinary.ValidateAudioFile(header); err != nil {
			response.BadRequest(c, err.Error(), "INVALID_FILE")
			return
		}
		result, err = h.cloudinary.UploadAudio(c.Request.Context(), file, header.Filename)
	} else {
		// Default to generic file
		if err := cloudinary.ValidateFile(header); err != nil {
			response.BadRequest(c, err.Error(), "INVALID_FILE")
			return
		}
		result, err = h.cloudinary.UploadFile(c.Request.Context(), file, header.Filename)
	}

	if err != nil {
		response.InternalServerError(c, "Failed to upload file", "UPLOAD_FAILED")
		return
	}

	response.Success(c, result)
}

// @Summary Preview link
// @Description Get metadata for a URL
// @Tags media
// @Produce json
// @Param url query string true "URL to preview"
// @Success 200 {object} response.APIResponse{data=anchors.URLData}
// @Router /media/preview [get]
func (h *Handler) GetLinkPreview(c *gin.Context) {
	targetURL := c.Query("url")
	if targetURL == "" {
		response.BadRequest(c, "URL is required", "MISSING_PARAM")
		return
	}

	// Use anchors scraper
	metadata, err := anchors.FetchURLMetadata(c.Request.Context(), targetURL)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch metadata", "SCRAPE_FAILED")
		return
	}

	response.Success(c, metadata)
}
