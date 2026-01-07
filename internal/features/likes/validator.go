package likes

import (
	"errors"
)

// ValidateLikeActionRequest validates the like action request
func ValidateLikeActionRequest(req *LikeActionRequest) error {
	if req.Action != "like" && req.Action != "unlike" {
		return errors.New("action must be 'like' or 'unlike'")
	}
	return nil
}

// ValidateLikeListQuery validates the like list query parameters
func ValidateLikeListQuery(query *LikeListQuery) error {
	if query.Page < 1 {
		query.Page = 1
	}

	if query.Limit < 1 {
		query.Limit = 20
	} else if query.Limit > 50 {
		query.Limit = 50
	}

	return nil
}
