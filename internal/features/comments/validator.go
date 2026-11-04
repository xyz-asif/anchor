package comments

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxCommentLength = 1000
	defaultListLimit = 20
	maxListLimit     = 50
	maxListPage      = 10000
)

// cleanCommentContent trims the comment and removes invisible characters (control characters
// other than line breaks/tabs, zero-width spaces, bidi overrides). The zero-width joiner stays,
// since compound emoji need it.
func cleanCommentContent(content string) string {
	content = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '‍' {
			return r
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, content)
	return strings.TrimSpace(content)
}

// validateContent checks cleaned comment text; the length is counted in characters, so every
// language gets the same 1,000 characters
func validateContent(content string) error {
	if content == "" {
		return errors.New("A comment can't be empty. Please write something first")
	}
	if n := utf8.RuneCountInString(content); n > maxCommentLength {
		return fmt.Errorf("Comments can be at most %d characters long (this one has %d)", maxCommentLength, n)
	}
	return nil
}

func ValidateCreateCommentRequest(req *CreateCommentRequest) error {
	req.Content = cleanCommentContent(req.Content)
	return validateContent(req.Content)
}

func ValidateUpdateCommentRequest(req *UpdateCommentRequest) error {
	req.Content = cleanCommentContent(req.Content)
	return validateContent(req.Content)
}

func ValidateCommentLikeActionRequest(req *CommentLikeActionRequest) error {
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	switch req.Action {
	case "like", "unlike":
		return nil
	case "":
		return errors.New(`Please say what to do: send {"action": "like"} or {"action": "unlike"}`)
	}
	return fmt.Errorf(`"%s" isn't a valid action. Use "like" or "unlike"`, req.Action)
}

// parsePaging reads ?page= and ?limit=, explaining exactly what is wrong with bad values
func parsePaging(pageStr, limitStr string) (page, limit int, err error) {
	page, limit = 1, defaultListLimit
	if pageStr != "" {
		n, convErr := strconv.Atoi(pageStr)
		if convErr != nil || n < 1 {
			return 0, 0, fmt.Errorf(`Page must be a whole number of 1 or more (got "%s")`, pageStr)
		}
		if n > maxListPage {
			return 0, 0, fmt.Errorf("Page can't be higher than %d", maxListPage)
		}
		page = n
	}
	if limitStr != "" {
		n, convErr := strconv.Atoi(limitStr)
		if convErr != nil || n < 1 {
			return 0, 0, fmt.Errorf(`Limit must be a whole number between 1 and %d (got "%s")`, maxListLimit, limitStr)
		}
		if n > maxListLimit {
			n = maxListLimit
		}
		limit = n
	}
	return page, limit, nil
}

// ParseCommentListQuery reads page, limit and sort for the comment list
func ParseCommentListQuery(pageStr, limitStr, sortStr string) (*CommentListQuery, error) {
	page, limit, err := parsePaging(pageStr, limitStr)
	if err != nil {
		return nil, err
	}
	sort := strings.ToLower(strings.TrimSpace(sortStr))
	switch sort {
	case "":
		sort = SortNewest
	case SortNewest, SortOldest, SortTop:
	default:
		return nil, fmt.Errorf(`"%s" isn't a valid sort. Use newest, oldest or top`, sortStr)
	}
	return &CommentListQuery{Page: page, Limit: limit, Sort: sort}, nil
}
