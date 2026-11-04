package media

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
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
		response.ServiceUnavailable(c, "File uploads aren't available right now. Please try again later", "UPLOADS_UNAVAILABLE")
		return
	}

	// Reject oversized bodies before parsing (largest allowed file is 50MB)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cloudinary.MaxFileSize+(1<<20))

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			response.Error(c, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("This file is too large. The maximum is %d MB", cloudinary.MaxFileSize/(1024*1024)), "FILE_TOO_LARGE")
			return
		}
		response.BadRequest(c, `Please attach the file as a multipart form field named "file"`, "MISSING_FILE")
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
		// Documents are stored as-is, so check they really are what the extension says
		if err := cloudinary.CheckDocumentContent(file, header.Filename); err != nil {
			response.BadRequest(c, err.Error(), "INVALID_FILE")
			return
		}
		result, err = h.cloudinary.UploadFile(c.Request.Context(), file, header.Filename)
	}

	if err != nil {
		log.Printf("Media upload failed: %v", err)
		response.InternalServerError(c, "We couldn't upload this file. If it's an image or audio file, check that it isn't damaged, then try again", "UPLOAD_FAILED")
		return
	}

	response.Success(c, result)
}

// hasScheme matches "scheme:" at the start (no dots, so "example.com:8080" is a host, not a scheme)
var hasScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+\-]*:`)

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
	targetURL := strings.TrimSpace(c.Query("url"))
	if targetURL == "" {
		response.BadRequest(c, "Please add the link to preview, e.g. ?url=https://example.com", "MISSING_PARAM")
		return
	}
	// "example.com/page" is what people paste; treat it as https. Text that already starts with
	// a scheme ("javascript:", "ftp:") is left alone, so it gets a clear "must be http(s)" message.
	if !hasScheme.MatchString(targetURL) {
		targetURL = "https://" + targetURL
	}
	if err := anchors.ValidateURL(targetURL); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_URL")
		return
	}

	metadata, err := anchors.FetchURLMetadata(c.Request.Context(), targetURL)
	if err != nil {
		// Bad, unreachable or blocked (private/internal) URLs are a client problem, not a 500
		response.BadRequest(c, "We couldn't load a preview for this link. Check that the page exists and is publicly reachable", "PREVIEW_FAILED")
		return
	}

	response.Success(c, metadata)
}
