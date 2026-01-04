// ================== pkg/errors/errors.go =================
package errors

import "errors"

var (
	ErrNotFound     = errors.New("resource not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrBadRequest   = errors.New("bad request")
	ErrInternal     = errors.New("internal server error")
	ErrDuplicate    = errors.New("resource already exists")
	ErrValidation   = errors.New("validation failed")
)
