package notifications

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultListLimit = 20
	maxListLimit     = 50
	maxListPage      = 10000
)

// ParseNotificationListQuery reads page, limit and unreadOnly, explaining exactly what is wrong
// with bad values
func ParseNotificationListQuery(pageStr, limitStr, unreadStr string) (*NotificationListQuery, error) {
	q := &NotificationListQuery{Page: 1, Limit: defaultListLimit}
	if pageStr != "" {
		n, err := strconv.Atoi(pageStr)
		if err != nil || n < 1 {
			return nil, fmt.Errorf(`Page must be a whole number of 1 or more (got "%s")`, pageStr)
		}
		if n > maxListPage {
			return nil, fmt.Errorf("Page can't be higher than %d", maxListPage)
		}
		q.Page = n
	}
	if limitStr != "" {
		n, err := strconv.Atoi(limitStr)
		if err != nil || n < 1 {
			return nil, fmt.Errorf(`Limit must be a whole number between 1 and %d (got "%s")`, maxListLimit, limitStr)
		}
		if n > maxListLimit {
			n = maxListLimit
		}
		q.Limit = n
	}
	unread, err := parseBool(unreadStr, "unreadOnly")
	if err != nil {
		return nil, err
	}
	q.UnreadOnly = unread
	return q, nil
}

// parseBool accepts true/false/1/0 (any case); empty means false
func parseBool(value, name string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "false", "0":
		return false, nil
	case "true", "1":
		return true, nil
	}
	return false, fmt.Errorf(`%s must be true or false (got "%s")`, name, value)
}
