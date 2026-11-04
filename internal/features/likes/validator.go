package likes

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultListLimit = 20
	maxListLimit     = 50
	maxListPage      = 10000
)

// ValidateLikeActionRequest validates the like action request (any letter case is accepted)
func ValidateLikeActionRequest(req *LikeActionRequest) error {
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	switch req.Action {
	case "like", "unlike":
		return nil
	case "":
		return errors.New(`Please say what to do: send {"action": "like"} or {"action": "unlike"}`)
	}
	return fmt.Errorf(`"%s" isn't a valid action. Use "like" or "unlike"`, req.Action)
}

// ParseLikeListQuery reads ?page= and ?limit=, explaining exactly what's wrong with bad values
func ParseLikeListQuery(pageStr, limitStr string) (*LikeListQuery, error) {
	q := &LikeListQuery{Page: 1, Limit: defaultListLimit}
	if pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			return nil, fmt.Errorf(`Page must be a whole number of 1 or more (got "%s")`, pageStr)
		}
		if page > maxListPage {
			return nil, fmt.Errorf("Page can't be higher than %d", maxListPage)
		}
		q.Page = page
	}
	if limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			return nil, fmt.Errorf(`Limit must be a whole number between 1 and %d (got "%s")`, maxListLimit, limitStr)
		}
		if limit > maxListLimit {
			limit = maxListLimit
		}
		q.Limit = limit
	}
	return q, nil
}
