package follows

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultListLimit = 20
	maxListLimit     = 50
	maxListPage      = 10000 // keeps (page-1)*limit far from overflow and skips cheap
)

// ValidateFollowActionRequest validates the follow action request
func ValidateFollowActionRequest(req *FollowActionRequest) error {
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	switch req.Action {
	case "follow", "unfollow":
		return nil
	case "":
		return errors.New(`Please say what to do: send {"action": "follow"} or {"action": "unfollow"}`)
	}
	return fmt.Errorf(`"%s" isn't a valid action. Use "follow" or "unfollow"`, req.Action)
}

// ParseFollowListQuery reads ?type=&page=&limit= and explains exactly what is wrong when a
// value is invalid (instead of the validator library's "Key: 'FollowListQuery.Type'..." text)
func ParseFollowListQuery(listType, pageStr, limitStr string) (*FollowListQuery, error) {
	q := &FollowListQuery{Type: strings.ToLower(strings.TrimSpace(listType)), Page: 1, Limit: defaultListLimit}

	switch q.Type {
	case "followers", "following":
	case "":
		return nil, errors.New(`Please choose which list to show: add ?type=followers or ?type=following`)
	default:
		return nil, fmt.Errorf(`"%s" isn't a valid list type. Use type=followers or type=following`, listType)
	}

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
			limit = maxListLimit // asking for too many is harmless: just return the maximum
		}
		q.Limit = limit
	}

	return q, nil
}
