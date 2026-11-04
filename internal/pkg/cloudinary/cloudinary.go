package cloudinary

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"unicode/utf8"

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

	// The typed UploadResult has no duration field, but Cloudinary returns it for
	// audio/video uploads in the raw JSON response, which the SDK keeps in Response.
	duration := 0.0
	if raw, ok := result.Response.(map[string]interface{}); ok {
		if d, ok := raw["duration"].(float64); ok {
			duration = d
		}
	}

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
	return validate(header, "image", MaxImageSize, AllowedImageTypes)
}

func ValidateAudioFile(header *multipart.FileHeader) error {
	return validate(header, "audio file", MaxAudioSize, AllowedAudioTypes)
}

func ValidateFile(header *multipart.FileHeader) error {
	if err := validate(header, "file", MaxFileSize, AllowedFileTypes); err != nil {
		if !isAllowedExtension(getFileExtension(header.Filename), AllowedFileTypes) {
			return UnsupportedTypeError(header.Filename)
		}
		return err
	}
	return nil
}

func validate(header *multipart.FileHeader, kind string, maxSize int64, allowed []string) error {
	ext := getFileExtension(header.Filename)
	if !isAllowedExtension(ext, allowed) {
		if ext == "" {
			return fmt.Errorf("This %s has no file extension. Supported types: %s", kind, listTypes(allowed))
		}
		return fmt.Errorf("%s isn't a supported %s type. Supported types: %s", ext, kind, listTypes(allowed))
	}
	if header.Size == 0 {
		return fmt.Errorf("This %s is empty", kind)
	}
	if header.Size > maxSize {
		return fmt.Errorf("This %s is %s. The maximum is %d MB", kind, humanSize(header.Size), maxSize/(1024*1024))
	}
	return nil
}

// UnsupportedTypeError explains which kinds of files can be uploaded at all
func UnsupportedTypeError(filename string) error {
	ext := getFileExtension(filename)
	what := ext + " files"
	if ext == "" {
		what = "Files without an extension"
	}
	return fmt.Errorf("%s can't be uploaded. Supported: images (%s), audio (%s) and documents (%s)",
		what, listTypes(AllowedImageTypes), listTypes(AllowedAudioTypes), listTypes(AllowedFileTypes))
}

func listTypes(types []string) string {
	out := make([]string, len(types))
	for i, t := range types {
		out[i] = strings.TrimPrefix(t, ".")
	}
	return strings.Join(out, ", ")
}

func humanSize(n int64) string {
	const mb = 1024 * 1024
	if n >= mb {
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	}
	return fmt.Sprintf("%d KB", (n+1023)/1024)
}

// CheckDocumentContent makes sure a document really is what its extension says, by looking at
// its first bytes: a renamed executable or web page can't be uploaded as ".pdf" or ".txt" and
// then served from our storage. Images and audio are checked by Cloudinary itself.
func CheckDocumentContent(f io.ReadSeeker, filename string) error {
	ext := getFileExtension(filename)
	head := make([]byte, 8192)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return errors.New("We couldn't read this file. Please try again")
	}
	head = head[:n]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return errors.New("We couldn't read this file. Please try again")
	}

	ok := true
	switch ext {
	case ".pdf":
		ok = bytes.HasPrefix(head, []byte("%PDF-"))
	case ".docx", ".epub":
		ok = bytes.HasPrefix(head, []byte("PK\x03\x04")) // zip container
	case ".doc":
		ok = bytes.HasPrefix(head, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) // OLE2
	case ".txt":
		ok = utf8.Valid(trimPartialRune(head)) && !bytes.ContainsRune(head, 0) && !looksLikeHTML(head)
	}
	if !ok {
		return fmt.Errorf("This file's contents don't match its %s extension. Please upload the original file", ext)
	}
	return nil
}

// trimPartialRune drops a multi-byte character cut off at the end of the sniffed bytes
func trimPartialRune(b []byte) []byte {
	for i := 0; i < utf8.UTFMax && len(b) > 0; i++ {
		if utf8.Valid(b) {
			return b
		}
		b = b[:len(b)-1]
	}
	return b
}

func looksLikeHTML(b []byte) bool {
	s := strings.ToLower(strings.TrimSpace(string(b[:min(len(b), 512)])))
	for _, p := range []string{"<!doctype html", "<html", "<script", "<svg", "<?xml"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

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
