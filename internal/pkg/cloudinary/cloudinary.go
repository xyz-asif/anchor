package cloudinary

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

// Service handles Cloudinary upload operations
type Service struct {
	cld          *cloudinary.Cloudinary
	uploadFolder string
}

// UploadResult contains the result of a successful upload
type UploadResult struct {
	URL      string
	PublicID string
	Width    int
	Height   int
	Duration float64 // for audio/video, in seconds
	FileSize int64
	Format   string
}

// File validation constants
var (
	AllowedImageTypes = []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
	AllowedAudioTypes = []string{".mp3", ".wav", ".aac", ".m4a", ".ogg"}
	AllowedFileTypes  = []string{".pdf", ".docx", ".doc", ".epub", ".txt"}

	MaxImageSize = int64(10 * 1024 * 1024) // 10MB
	MaxAudioSize = int64(25 * 1024 * 1024) // 25MB
	MaxFileSize  = int64(50 * 1024 * 1024) // 50MB
)

// NewService creates a new Cloudinary service instance
func NewService(cloudName, apiKey, apiSecret, uploadFolder string) (*Service, error) {
	if cloudName == "" || apiKey == "" || apiSecret == "" {
		return nil, errors.New("cloudinary credentials are required")
	}

	// Build Cloudinary URL
	cloudinaryURL := fmt.Sprintf("cloudinary://%s:%s@%s", apiKey, apiSecret, cloudName)

	// Initialize Cloudinary client
	cld, err := cloudinary.NewFromURL(cloudinaryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cloudinary client: %w", err)
	}

	if uploadFolder == "" {
		uploadFolder = "anchor"
	}

	return &Service{
		cld:          cld,
		uploadFolder: uploadFolder,
	}, nil
}

// UploadImage uploads an image file to Cloudinary
func (s *Service) UploadImage(ctx context.Context, file multipart.File, filename string) (*UploadResult, error) {
	folder := s.uploadFolder + "/images"

	uploadParams := uploader.UploadParams{
		Folder:       folder,
		ResourceType: "image",
	}

	result, err := s.cld.Upload.Upload(ctx, file, uploadParams)
	if err != nil {
		return nil, fmt.Errorf("failed to upload image: %w", err)
	}

	return &UploadResult{
		URL:      result.SecureURL,
		PublicID: result.PublicID,
		Width:    result.Width,
		Height:   result.Height,
		FileSize: int64(result.Bytes),
		Format:   result.Format,
	}, nil
}

// UploadAudio uploads an audio file to Cloudinary
func (s *Service) UploadAudio(ctx context.Context, file multipart.File, filename string) (*UploadResult, error) {
	folder := s.uploadFolder + "/audio"

	uploadParams := uploader.UploadParams{
		Folder:       folder,
		ResourceType: "video", // Cloudinary uses "video" resource type for audio
	}

	result, err := s.cld.Upload.Upload(ctx, file, uploadParams)
	if err != nil {
		return nil, fmt.Errorf("failed to upload audio: %w", err)
	}

	// TODO: Extract duration via Admin API
	// The upload response doesn't include duration in the current SDK version
	// To get duration, we need to call: s.cld.Admin.Asset(ctx, admin.AssetParams{PublicID: result.PublicID})
	// For now, duration will be 0 and should be extracted client-side or via webhook
	duration := 0.0

	return &UploadResult{
		URL:      result.SecureURL,
		PublicID: result.PublicID,
		Duration: duration,
		FileSize: int64(result.Bytes),
		Format:   result.Format,
	}, nil
}

// UploadFile uploads a generic file to Cloudinary
func (s *Service) UploadFile(ctx context.Context, file multipart.File, filename string) (*UploadResult, error) {
	folder := s.uploadFolder + "/files"

	uploadParams := uploader.UploadParams{
		Folder:       folder,
		ResourceType: "raw",
	}

	result, err := s.cld.Upload.Upload(ctx, file, uploadParams)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	return &UploadResult{
		URL:      result.SecureURL,
		PublicID: result.PublicID,
		FileSize: int64(result.Bytes),
		Format:   result.Format,
	}, nil
}

// Delete removes an asset from Cloudinary
func (s *Service) Delete(ctx context.Context, publicID string, resourceType string) error {
	if publicID == "" {
		return errors.New("publicID is required")
	}

	if resourceType == "" {
		resourceType = "image"
	}

	destroyParams := uploader.DestroyParams{
		PublicID:     publicID,
		ResourceType: resourceType,
	}

	_, err := s.cld.Upload.Destroy(ctx, destroyParams)
	if err != nil {
		return fmt.Errorf("failed to delete asset: %w", err)
	}

	return nil
}

// ValidateImageFile validates an image file upload
func ValidateImageFile(header *multipart.FileHeader) error {
	// Check file size
	if header.Size > MaxImageSize {
		return fmt.Errorf("image file size exceeds maximum allowed size of %d MB", MaxImageSize/(1024*1024))
	}

	// Check file extension
	ext := getFileExtension(header.Filename)
	if !isAllowedExtension(ext, AllowedImageTypes) {
		return fmt.Errorf("invalid image file type: %s. Allowed types: %s", ext, strings.Join(AllowedImageTypes, ", "))
	}

	return nil
}

// ValidateAudioFile validates an audio file upload
func ValidateAudioFile(header *multipart.FileHeader) error {
	// Check file size
	if header.Size > MaxAudioSize {
		return fmt.Errorf("audio file size exceeds maximum allowed size of %d MB", MaxAudioSize/(1024*1024))
	}

	// Check file extension
	ext := getFileExtension(header.Filename)
	if !isAllowedExtension(ext, AllowedAudioTypes) {
		return fmt.Errorf("invalid audio file type: %s. Allowed types: %s", ext, strings.Join(AllowedAudioTypes, ", "))
	}

	return nil
}

// ValidateFile validates a generic file upload
func ValidateFile(header *multipart.FileHeader) error {
	// Check file size
	if header.Size > MaxFileSize {
		return fmt.Errorf("file size exceeds maximum allowed size of %d MB", MaxFileSize/(1024*1024))
	}

	// Check file extension
	ext := getFileExtension(header.Filename)
	if !isAllowedExtension(ext, AllowedFileTypes) {
		return fmt.Errorf("invalid file type: %s. Allowed types: %s", ext, strings.Join(AllowedFileTypes, ", "))
	}

	return nil
}

// getFileExtension returns the lowercase file extension including the dot
func getFileExtension(filename string) string {
	ext := filepath.Ext(filename)
	return strings.ToLower(ext)
}

// isAllowedExtension checks if the extension is in the allowed list
func isAllowedExtension(ext string, allowedTypes []string) bool {
	for _, allowed := range allowedTypes {
		if ext == allowed {
			return true
		}
	}
	return false
}
