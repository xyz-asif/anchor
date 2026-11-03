// Package tags holds the one canonical way tags are normalised, shared by anchor tags,
// user interests and tag-feed lookups so that they always match each other.
package tags

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MinLen and MaxLen bound a normalised tag's length in characters
const (
	MinLen = 2
	MaxLen = 30
)

// Normalize returns the canonical form of a tag, or "" if nothing valid remains:
// lowercase, leading '#' removed, spaces/underscores turned into '-', only letters
// (any language, with their combining marks), digits and '-' kept, repeated/edge hyphens trimmed.
func Normalize(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimLeft(s, "#")

	var b strings.Builder
	lastHyphen := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.M, r): // M = combining marks (e.g. Devanagari vowel signs)
			b.WriteRune(r)
			lastHyphen = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastHyphen && b.Len() > 0 {
				b.WriteRune('-')
				lastHyphen = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if n := utf8.RuneCountInString(out); n < MinLen || n > MaxLen {
		return ""
	}
	return out
}

// NormalizeList normalises tags, dropping invalid ones and duplicates (order kept)
func NormalizeList(raw []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range raw {
		if n := Normalize(t); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}
