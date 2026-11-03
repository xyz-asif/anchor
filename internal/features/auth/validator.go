package auth

import (
	"errors"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
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

// ValidateDisplayName validates the display name (length in characters, not bytes)
func ValidateDisplayName(displayName string) error {
	if n := utf8.RuneCountInString(strings.TrimSpace(displayName)); n < 2 || n > 50 {
		return errors.New("display name must be between 2 and 50 characters")
	}
	return nil
}

// ValidateBio validates the bio
func ValidateBio(bio string) error {
	if utf8.RuneCountInString(bio) > 200 {
		return errors.New("bio must not exceed 200 characters")
	}
	return nil
}

// reservedUsernames can't be taken by users (they collide with routes or imply authority)
var reservedUsernames = map[string]bool{
	"admin": true, "administrator": true, "api": true, "www": true, "app": true, "help": true,
	"support": true, "anchor": true, "anchors": true, "user": true, "users": true, "settings": true,
	"login": true, "logout": true, "signup": true, "register": true, "me": true, "feed": true,
	"search": true, "notifications": true, "moderator": true, "staff": true, "official": true,
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// ValidateUsername validates a (lowercased) username. This one rule is used by both the
// availability check and the username change, so they can never disagree.
func ValidateUsername(username string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if len(username) < 3 || len(username) > 20 {
		return errors.New("username must be between 3 and 20 characters")
	}
	if !usernamePattern.MatchString(username) {
		return errors.New("username can only contain letters, numbers, and underscores")
	}
	if reservedUsernames[username] {
		return errors.New("this username is reserved")
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
