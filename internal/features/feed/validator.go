package feed

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xyz-asif/gotodo/internal/pkg/tags"
)

const (
	defaultFeedLimit = 20
	maxFeedLimit     = 50
)

// parseLimit reads ?limit=; values above the maximum are capped, like every other list
func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultFeedLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf(`Limit must be a whole number between 1 and %d (got "%s")`, maxFeedLimit, raw)
	}
	if n > maxFeedLimit {
		n = maxFeedLimit
	}
	return n, nil
}

// parseBool accepts true/false/1/0 (any case); empty means def
func parseBool(raw, name string, def bool) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return def, nil
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	}
	return false, fmt.Errorf(`%s must be true or false (got "%s")`, name, raw)
}

// ParseFeedQuery reads limit, cursor and includeOwn for the home feed
func ParseFeedQuery(limitStr, cursorStr, includeOwnStr string) (*FeedQuery, error) {
	limit, err := parseLimit(limitStr)
	if err != nil {
		return nil, err
	}
	includeOwn, err := parseBool(includeOwnStr, "includeOwn", true)
	if err != nil {
		return nil, err
	}
	cursor, err := DecodeCursor(strings.TrimSpace(cursorStr))
	if err != nil {
		return nil, err
	}
	return &FeedQuery{Limit: limit, Cursor: cursor, IncludeOwn: includeOwn}, nil
}

// ParseDiscoverQuery reads limit, cursor, category and tag for the discover and tag feeds.
// defaultCategory is used when no category is given.
func ParseDiscoverQuery(limitStr, cursorStr, categoryStr, tagStr, defaultCategory string) (*DiscoverQuery, error) {
	limit, err := parseLimit(limitStr)
	if err != nil {
		return nil, err
	}

	category := strings.ToLower(strings.TrimSpace(categoryStr))
	switch category {
	case "":
		category = defaultCategory
	case CategoryTrending, CategoryPopular, CategoryRecent, CategoryForYou:
	case "foryou", "for-you":
		category = CategoryForYou
	default:
		return nil, fmt.Errorf(`"%s" isn't a feed category. Use trending, popular, recent or for_you`, categoryStr)
	}

	var tag string
	if strings.TrimSpace(tagStr) != "" {
		// Normalised exactly like anchor tags are stored, so "#Web Design" finds "web-design"
		if tag = tags.Normalize(tagStr); tag == "" {
			return nil, fmt.Errorf(`"%s" isn't a valid tag. Tags are %d to %d letters or numbers`, tagStr, tags.MinLen, tags.MaxLen)
		}
	}

	cursor, err := DecodeDiscoverCursor(strings.TrimSpace(cursorStr))
	if err != nil {
		return nil, err
	}
	// Score-sorted categories page by score; a cursor without one (e.g. from "recent") would
	// silently return page 1 again forever
	if cursor != nil && category != CategoryRecent && cursor.Score == nil {
		return nil, errBadCursor
	}

	return &DiscoverQuery{Limit: limit, Cursor: cursor, Category: category, Tag: tag}, nil
}
