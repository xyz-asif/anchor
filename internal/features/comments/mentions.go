package comments

import (
	"strings"
)

const (
	maxMentions       = 10
	mentionMinLength  = 3
	mentionMaxLength  = 20 // same limits as usernames
	usernameCharTable = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
)

func isUsernameChar(b byte) bool {
	return strings.IndexByte(usernameCharTable, b) >= 0
}

// ExtractMentions returns the unique, lowercased @usernames in content (at most 10).
// An @ only starts a mention at the beginning of the text or after a character that can't be
// part of a username or address, so "me@example.com" mentions nobody, and the whole name must
// fit the username rules: "@" followed by 25 letters is not a mention of its first 20.
func ExtractMentions(content string) []string {
	seen := make(map[string]bool)
	var usernames []string

	for i := 0; i < len(content); i++ {
		if content[i] != '@' {
			continue
		}
		if i > 0 {
			prev := content[i-1]
			if isUsernameChar(prev) || prev == '.' || prev == '@' || prev == '-' || prev == '+' {
				continue
			}
		}
		end := i + 1
		for end < len(content) && isUsernameChar(content[end]) {
			end++
		}
		name := content[i+1 : end]
		i = end - 1
		if len(name) < mentionMinLength || len(name) > mentionMaxLength {
			continue
		}
		name = strings.ToLower(name)
		if !seen[name] {
			seen[name] = true
			usernames = append(usernames, name)
			if len(usernames) == maxMentions {
				break
			}
		}
	}

	return usernames
}
