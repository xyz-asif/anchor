package search

import (
	"errors"
	"strings"
)

func ValidateUnifiedSearchQuery(query *UnifiedSearchQuery) error {
	query.Q = strings.TrimSpace(query.Q)

	if len(query.Q) < 2 {
		return errors.New("query must be at least 2 characters")
	}

	if len(query.Q) > 100 {
		return errors.New("query must be 100 characters or less")
	}

	if query.Type == "" {
		query.Type = TypeAll
	}

	if query.Type != TypeAll && query.Type != TypeAnchors && query.Type != TypeUsers {
		return errors.New("type must be: all, anchors, or users")
	}

	if query.Limit < 1 {
		query.Limit = 10
	}
	if query.Limit > 20 {
		query.Limit = 20
	}

	return nil
}

func ValidateAnchorSearchQuery(query *AnchorSearchQuery) error {
	query.Q = strings.TrimSpace(query.Q)
	query.Tag = strings.TrimSpace(query.Tag)

	if len(query.Q) < 2 {
		return errors.New("query must be at least 2 characters")
	}

	if len(query.Q) > 100 {
		return errors.New("query must be 100 characters or less")
	}

	if query.Sort == "" {
		query.Sort = SortRelevant
	}

	if query.Sort != SortRelevant && query.Sort != SortRecent && query.Sort != SortPopular {
		return errors.New("sort must be: relevant, recent, or popular")
	}

	if query.Page < 1 {
		query.Page = 1
	}

	if query.Limit < 1 {
		query.Limit = 20
	}
	if query.Limit > 50 {
		query.Limit = 50
	}

	return nil
}

func ValidateUserSearchQuery(query *UserSearchQuery) error {
	query.Q = strings.TrimSpace(query.Q)

	if len(query.Q) < 2 {
		return errors.New("query must be at least 2 characters")
	}

	if len(query.Q) > 100 {
		return errors.New("query must be 100 characters or less")
	}

	if query.Page < 1 {
		query.Page = 1
	}

	if query.Limit < 1 {
		query.Limit = 20
	}
	if query.Limit > 50 {
		query.Limit = 50
	}

	return nil
}

func ValidateTagSearchQuery(query *TagSearchQuery) error {
	query.Q = strings.TrimSpace(query.Q)

	if len(query.Q) < 1 {
		return errors.New("query must be at least 1 character")
	}

	if len(query.Q) > 50 {
		return errors.New("query must be 50 characters or less")
	}

	if query.Limit < 1 {
		query.Limit = 10
	}
	if query.Limit > 20 {
		query.Limit = 20
	}

	return nil
}
