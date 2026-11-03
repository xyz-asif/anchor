package media

import (
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strings"

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
	if h.cloudinary == nil {
		response.ServiceUnavailable(c, "File uploads are not configured", "UPLOADS_UNAVAILABLE")
		return
	}

	// Reject oversized bodies before parsing (largest allowed file is 50MB)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cloudinary.MaxFileSize+(1<<20))

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			response.Error(c, http.StatusRequestEntityTooLarge, "File is too large (max 50MB)", "FILE_TOO_LARGE")
			return
		}
		response.BadRequest(c, "File is required", "MISSING_FILE")
		return
	}
	defer file.Close()

	// Choose the upload type from the file extension (the client-sent Content-Type can't be
	// trusted); each validator enforces that type's extensions and size limit.
	ext := strings.ToLower(filepath.Ext(header.Filename))
	var result *cloudinary.UploadResult
	switch {
	case contains(cloudinary.AllowedImageTypes, ext):
		if err := cloudinary.ValidateImageFile(header); err != nil {
			response.BadRequest(c, err.Error(), "INVALID_FILE")
			return
		}
		result, err = h.cloudinary.UploadImage(c.Request.Context(), file, header.Filename)
	case contains(cloudinary.AllowedAudioTypes, ext):
		if err := cloudinary.ValidateAudioFile(header); err != nil {
			response.BadRequest(c, err.Error(), "INVALID_FILE")
			return
		}
		result, err = h.cloudinary.UploadAudio(c.Request.Context(), file, header.Filename)
	default:
		if err := cloudinary.ValidateFile(header); err != nil {
			response.BadRequest(c, err.Error(), "INVALID_FILE")
			return
		}
		result, err = h.cloudinary.UploadFile(c.Request.Context(), file, header.Filename)
	}

	if err != nil {
		log.Printf("Media upload failed: %v", err)
		response.InternalServerError(c, "Failed to upload file", "UPLOAD_FAILED")
		return
	}

	response.Success(c, result)
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
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
		// Bad, unreachable or blocked (private/internal) URLs are a client problem, not a 500
		response.BadRequest(c, "Could not fetch a preview for this URL", "PREVIEW_FAILED")
		return
	}

	response.Success(c, metadata)
}
