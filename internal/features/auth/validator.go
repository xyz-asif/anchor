package auth

import (
	"errors"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidateUpdateProfileRequest validates the profile update request
func ValidateUpdateProfileRequest(req *UpdateProfileRequest) error {
	if req.DisplayName != nil {
		if err := ValidateDisplayName(*req.DisplayName); err != nil {
			return err
		}
	}

	if req.Bio != nil {
		if err := ValidateBio(*req.Bio); err != nil {
			return err
		}
	}

	return nil
}

// ValidateDisplayName validates the display name
func ValidateDisplayName(displayName string) error {
	if len(displayName) < 2 || len(displayName) > 50 {
		return errors.New("display name must be between 2 and 50 characters")
	}
	return nil
}

// ValidateBio validates the bio
func ValidateBio(bio string) error {
	if len(bio) > 200 {
		return errors.New("bio must not exceed 200 characters")
	}
	return nil
}

// ValidateUsername validates the username format
func ValidateUsername(username string) error {
	if len(username) < 3 || len(username) > 30 {
		return errors.New("username must be between 3 and 30 characters")
	}

	// Alphanumeric and underscores only
	match, _ := regexp.MatchString("^[a-zA-Z0-9_]+$", username)
	if !match {
		return errors.New("username can only contain letters, numbers, and underscores")
	}
	return nil
}

// ValidateProfilePicture validates the profile picture file
func ValidateProfilePicture(file *multipart.FileHeader) error {
	// Check file size (max 5MB)
	if file.Size > 5*1024*1024 {
		return errors.New("profile picture must be less than 5MB")
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	validExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".webp": true,
	}

	if !validExts[ext] {
		return errors.New("invalid file type. allowed: jpg, jpeg, png, webp")
	}

	return nil
}

// ValidateCoverImage validates the cover image file
func ValidateCoverImage(file *multipart.FileHeader) error {
	// Check file size (max 10MB)
	if file.Size > 10*1024*1024 {
		return errors.New("cover image must be less than 10MB")
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	validExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".webp": true,
	}

	if !validExts[ext] {
		return errors.New("invalid file type. allowed: jpg, jpeg, png, webp")
	}

	return nil
}
