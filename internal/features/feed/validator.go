package feed

import (
	"errors"
	"strings"
)

// ValidateFeedQuery validates the query parameters
func ValidateFeedQuery(query *FeedQuery) error {
	if query.Limit < 1 {
		query.Limit = 20
	}
	if query.Limit > 50 {
		return errors.New("limit must be between 1 and 50")
	}

	// Default IncludeOwn to true if nil
	if query.IncludeOwn == nil {
		val := true
		query.IncludeOwn = &val
	}

	return nil
}

// ValidateCursor validates and decodes the cursor
func ValidateCursor(cursor string) (*FeedCursor, error) {
	return DecodeCursor(cursor)
}

// ValidateDiscoverQuery validates discovery query parameters
func ValidateDiscoverQuery(query *DiscoverQuery) error {
	// Default limit
	if query.Limit < 1 {
		query.Limit = 20
	}
	if query.Limit > 50 {
		return errors.New("limit must be between 1 and 50")
	}

	// Default and validate category
	if query.Category == "" {
		query.Category = CategoryTrending
	}
	if query.Category != CategoryTrending &&
		query.Category != CategoryPopular &&
		query.Category != CategoryRecent {
		return errors.New("category must be: trending, popular, or recent")
	}

	// Normalize tag to lowercase
	if query.Tag != "" {
		query.Tag = strings.ToLower(strings.TrimSpace(query.Tag))
	}

	return nil
}
