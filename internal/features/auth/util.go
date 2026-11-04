package auth

import (
	"crypto/rand"
	"math/big"
	"regexp"
	"strings"
	"unicode"
)

const (
	usernameMinLen = 3
	usernameMaxLen = 20
)

var nonUsernameChars = regexp.MustCompile("[^a-z0-9_]+")

// GenerateUniqueUsername generates a base username from a display name
// (the caller still has to make it unique, see Handler.uniqueUsername)
func GenerateUniqueUsername(displayName string) string {
	return usernameBase(strings.ReplaceAll(strings.ToLower(displayName), " ", "_"))
}

// usernameBase turns any string into a valid username candidate: allowed characters only,
// at most 20 characters, at least 3, and never a reserved name.
func usernameBase(s string) string {
	username := nonUsernameChars.ReplaceAllString(strings.ToLower(s), "")
	if len(username) > usernameMaxLen {
		username = username[:usernameMaxLen]
	}
	if len(username) < usernameMinLen {
		username = "user_" + username
	}
	if reservedUsernames[username] {
		username = withUsernameSuffix(username, "_1")
	}
	return username
}

// withUsernameSuffix appends suffix, shortening base so the result stays within 20 characters
func withUsernameSuffix(base, suffix string) string {
	if keep := usernameMaxLen - len(suffix); len(base) > keep {
		base = base[:keep]
	}
	return base + suffix
}

// randomDigits returns n cryptographically random decimal digits
func randomDigits(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		d, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			d = big.NewInt(int64(i % 10))
		}
		b.WriteByte(byte('0' + d.Int64()))
	}
	return b.String()
}

// stripControlChars removes control and invisible format characters (newlines, zero-width
// spaces, bidi overrides) that would let a name look empty or impersonate someone else
func stripControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if isInvisibleChar(r) {
			return -1
		}
		return r
	}, s)
}

// isInvisibleChar reports control characters and invisible format characters. The zero-width
// joiner is kept because compound emoji (e.g. family emoji) are built with it.
func isInvisibleChar(r rune) bool {
	return r != '\u200d' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r))
}

// stripControlCharsKeepNewlines is stripControlChars for multi-line text such as a bio
func stripControlCharsKeepNewlines(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if isInvisibleChar(r) {
			return -1
		}
		return r
	}, s)
}
