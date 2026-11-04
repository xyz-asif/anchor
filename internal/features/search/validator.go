package search

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xyz-asif/gotodo/internal/pkg/tags"
)

const (
	minQueryLength = 2
	maxQueryLength = 100
	maxTagPrefix   = tags.MaxLen
	maxPage        = 10000
)

// cleanQuery removes invisible characters (control characters, zero-width spaces, bidi
// overrides) and collapses runs of whitespace, so "  web​  design " searches "web design"
func cleanQuery(raw string) string {
	raw = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, raw)
	return strings.Join(strings.Fields(raw), " ")
}

// parseSearchText cleans and checks the search text; lengths count characters, not bytes
func parseSearchText(raw string) (string, error) {
	q := cleanQuery(raw)
	n := utf8.RuneCountInString(q)
	if n == 0 {
		return "", errors.New("Please type something to search for")
	}
	if n < minQueryLength {
		return "", fmt.Errorf("Please type at least %d characters to search", minQueryLength)
	}
	if n > maxQueryLength {
		return "", fmt.Errorf("Searches can be at most %d characters long (this one has %d)", maxQueryLength, n)
	}
	return q, nil
}

// parseLimit reads ?limit=; values above max are capped like every other list
func parseLimit(raw string, def, max int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf(`Limit must be a whole number between 1 and %d (got "%s")`, max, raw)
	}
	if n > max {
		n = max
	}
	return n, nil
}

func parsePage(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf(`Page must be a whole number of 1 or more (got "%s")`, raw)
	}
	if n > maxPage {
		return 0, fmt.Errorf("Page can't be higher than %d", maxPage)
	}
	return n, nil
}

// tagPrefix turns what the user typed into the start of a stored tag: tags are saved lowercase
// with '-' between words, so "#Web Des" becomes "web-des"
func tagPrefix(raw string) string {
	var b strings.Builder
	lastHyphen := true // no leading hyphen
	for _, r := range strings.ToLower(strings.TrimLeft(strings.TrimSpace(raw), "#")) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.M, r):
			b.WriteRune(r)
			lastHyphen = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastHyphen {
				b.WriteRune('-')
				lastHyphen = true
			}
		}
	}
	return b.String()
}

// ParseUnifiedQuery reads q, type and limit for GET /search
func ParseUnifiedQuery(qStr, typeStr, limitStr string) (*UnifiedSearchQuery, error) {
	q, err := parseSearchText(qStr)
	if err != nil {
		return nil, err
	}
	searchType := strings.ToLower(strings.TrimSpace(typeStr))
	switch searchType {
	case "":
		searchType = TypeAll
	case TypeAll, TypeAnchors, TypeUsers:
	default:
		return nil, fmt.Errorf(`"%s" isn't a search type. Use all, anchors or users`, typeStr)
	}
	limit, err := parseLimit(limitStr, 10, 20)
	if err != nil {
		return nil, err
	}
	return &UnifiedSearchQuery{Q: q, Type: searchType, Limit: limit}, nil
}

// ParseCombinedQuery reads q and limit for the type-ahead GET /search/combined
func ParseCombinedQuery(qStr, limitStr string) (string, int, error) {
	q, err := parseSearchText(qStr)
	if err != nil {
		return "", 0, err
	}
	limit, err := parseLimit(limitStr, 3, 10)
	if err != nil {
		return "", 0, err
	}
	return q, limit, nil
}

// ParseAnchorSearchQuery reads q, tag, sort, page and limit for GET /search/anchors
func ParseAnchorSearchQuery(qStr, tagStr, sortStr, pageStr, limitStr string) (*AnchorSearchQuery, error) {
	q, err := parseSearchText(qStr)
	if err != nil {
		return nil, err
	}
	var tag string
	if strings.TrimSpace(tagStr) != "" {
		// Normalised like stored tags, so "#Web Design" filters on "web-design"
		if tag = tags.Normalize(tagStr); tag == "" {
			return nil, fmt.Errorf(`"%s" isn't a valid tag. Tags are %d to %d letters or numbers`, tagStr, tags.MinLen, tags.MaxLen)
		}
	}
	sort := strings.ToLower(strings.TrimSpace(sortStr))
	switch sort {
	case "":
		sort = SortRelevant
	case SortRelevant, SortRecent, SortPopular:
	default:
		return nil, fmt.Errorf(`"%s" isn't a valid sort. Use relevant, recent or popular`, sortStr)
	}
	page, err := parsePage(pageStr)
	if err != nil {
		return nil, err
	}
	limit, err := parseLimit(limitStr, 20, 50)
	if err != nil {
		return nil, err
	}
	return &AnchorSearchQuery{Q: q, Tag: tag, Sort: sort, Page: page, Limit: limit}, nil
}

// ParseUserSearchQuery reads q, page and limit for GET /search/users
func ParseUserSearchQuery(qStr, pageStr, limitStr string) (*UserSearchQuery, error) {
	q, err := parseSearchText(qStr)
	if err != nil {
		return nil, err
	}
	page, err := parsePage(pageStr)
	if err != nil {
		return nil, err
	}
	limit, err := parseLimit(limitStr, 20, 50)
	if err != nil {
		return nil, err
	}
	return &UserSearchQuery{Q: q, Page: page, Limit: limit}, nil
}

// ParseTagSearchQuery reads q (a tag prefix, 1 character is enough) and limit for GET /search/tags
func ParseTagSearchQuery(qStr, limitStr string) (*TagSearchQuery, error) {
	if cleanQuery(qStr) == "" {
		return nil, errors.New("Please type the start of a tag to search for")
	}
	prefix := tagPrefix(cleanQuery(qStr))
	if prefix == "" {
		return nil, fmt.Errorf(`"%s" has no letters or numbers to search tags for`, qStr)
	}
	if n := utf8.RuneCountInString(prefix); n > maxTagPrefix {
		return nil, fmt.Errorf("Tags are at most %d characters long, so no tag starts with this (it has %d)", maxTagPrefix, n)
	}
	limit, err := parseLimit(limitStr, 10, 20)
	if err != nil {
		return nil, err
	}
	return &TagSearchQuery{Q: prefix, Limit: limit}, nil
}
