package comments

import (
	"errors"
	"strings"
)

func ValidateCreateCommentRequest(req *CreateCommentRequest) error {
	req.Content = strings.TrimSpace(req.Content)

	if req.Content == "" {
		return errors.New("content is required")
	}

	if len(req.Content) > 1000 {
		return errors.New("content must be 1000 characters or less")
	}

	return nil
}

func ValidateUpdateCommentRequest(req *UpdateCommentRequest) error {
	req.Content = strings.TrimSpace(req.Content)

	if req.Content == "" {
		return errors.New("content is required")
	}

	if len(req.Content) > 1000 {
		return errors.New("content must be 1000 characters or less")
	}

	return nil
}

func ValidateCommentLikeActionRequest(req *CommentLikeActionRequest) error {
	if req.Action != "like" && req.Action != "unlike" {
		return errors.New("action must be 'like' or 'unlike'")
	}
	return nil
}

func ValidateCommentListQuery(query *CommentListQuery) error {
	if query.Page < 1 {
		query.Page = 1
	}

	if query.Limit < 1 {
		query.Limit = 20
	}
	if query.Limit > 50 {
		query.Limit = 50
	}

	if query.Sort == "" {
		query.Sort = SortNewest
	}

	if query.Sort != SortNewest && query.Sort != SortOldest && query.Sort != SortTop {
		return errors.New("sort must be: newest, oldest, or top")
	}

	return nil
}
