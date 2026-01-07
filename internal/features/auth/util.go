package auth

import (
	"regexp"
	"strings"
)

// GenerateUniqueUsername generates a base username from a display name
func GenerateUniqueUsername(displayName string) string {
	// Convert to lowercase
	username := strings.ToLower(displayName)

	// Replace spaces with underscores
	username = strings.ReplaceAll(username, " ", "_")

	// Remove non-alphanumeric characters (except underscores)
	reg, _ := regexp.Compile("[^a-z0-9_]+")
	username = reg.ReplaceAllString(username, "")

	// Truncate to max length (e.g., 20)
	if len(username) > 20 {
		username = username[:20]
	}

	// Ensure minimum length
	if len(username) < 3 {
		username = "user_" + username
	}

	// If still too short (was empty)
	if len(username) < 3 {
		username = "user_default"
	}

	return username
}
