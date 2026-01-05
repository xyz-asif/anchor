package follows

import (
	"errors"
)

// ValidateFollowActionRequest validates the follow action request
func ValidateFollowActionRequest(req *FollowActionRequest) error {
	if req.Action != "follow" && req.Action != "unfollow" {
		return errors.New("action must be 'follow' or 'unfollow'")
	}
	return nil
}

// ValidateFollowListQuery validates the follow list query parameters
func ValidateFollowListQuery(query *FollowListQuery) error {
	if query.Type != "followers" && query.Type != "following" {
		return errors.New("type must be 'followers' or 'following'")
	}

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
