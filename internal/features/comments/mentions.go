package comments

import (
	"regexp"
	"strings"
)

var mentionRegex = regexp.MustCompile(`@([a-zA-Z0-9_]{3,30})`)

// ExtractMentions extracts unique @usernames from content
func ExtractMentions(content string) []string {
	matches := mentionRegex.FindAllStringSubmatch(content, -1)

	// Deduplicate and normalize to lowercase
	seen := make(map[string]bool)
	var usernames []string

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		username := strings.ToLower(match[1])
		if !seen[username] {
			seen[username] = true
			usernames = append(usernames, username)
		}
	}

	// Limit to 10 mentions
	if len(usernames) > 10 {
		usernames = usernames[:10]
	}

	return usernames
}
