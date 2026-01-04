package auth

import (
	"errors"
	"regexp"
	"strings"
)

var (
	usernameRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// ValidateUsername checks if the username format is valid
func ValidateUsername(username string) error {
	username = strings.TrimSpace(username)

	// Convert to lowercase for consistent validation
	username = strings.ToLower(username)

	if len(username) < 3 || len(username) > 20 {
		return errors.New("username must be between 3 and 20 characters")
	}

	if !usernameRegex.MatchString(username) {
		return errors.New("username must start with a letter and contain only letters, numbers, underscores, or hyphens")
	}

	return nil
}

// ValidateDisplayName checks if the display name is valid
func ValidateDisplayName(name string) error {
	name = strings.TrimSpace(name)

	if len(name) < 3 || len(name) > 50 {
		return errors.New("display name must be between 3 and 50 characters")
	}

	return nil
}

// ValidateBio checks if the bio length is valid
func ValidateBio(bio string) error {
	bio = strings.TrimSpace(bio)

	if len(bio) > 160 {
		return errors.New("bio cannot exceed 160 characters")
	}

	return nil
}

// GenerateUniqueUsername creates a base username from a name string
// Note: Uniqueness is not guaranteed by this function alone, it just formats the string
func GenerateUniqueUsername(name string) string {
	// Remove spaces and special characters, keep only alphanumeric
	reg := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	username := reg.ReplaceAllString(name, "")

	username = strings.ToLower(username)

	// Ensure it's not empty and doesn't start with a number
	if username == "" || (len(username) > 0 && username[0] >= '0' && username[0] <= '9') {
		username = "user" + username
	}

	// Truncate if too long (leave room for suffix numbers if needed)
	if len(username) > 15 {
		username = username[:15]
	}

	// Ensure minimum length
	if len(username) < 3 {
		username = username + "dev"
	}

	return username
}
