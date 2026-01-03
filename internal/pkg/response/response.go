package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIResponse is the unified response envelope used across the API
type APIResponse struct {
	Success    bool        `json:"success" example:"true"`
	StatusCode int         `json:"statusCode" example:"200"`
	Message    string      `json:"message,omitempty" example:"OK"`
	Data       interface{} `json:"data,omitempty"`
	Code       string      `json:"code,omitempty" example:"AUTH_INVALID_TOKEN"`
}

// paginatedData holds the paginated items and metadata
type paginatedData struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total" example:"25"`
	Limit int         `json:"limit" example:"10"`
	Page  int         `json:"page,omitempty" example:"1"`
}

// Respond sends a JSON response using the unified envelope
func Respond(c *gin.Context, statusCode int, success bool, message string, data interface{}, code ...string) {
	c.JSON(statusCode, APIResponse{
		Success:    success,
		StatusCode: statusCode,
		Message:    message,
		Data:       data,
		Code: func() string {
			if len(code) > 0 {
				return code[0]
			}
			return ""
		}(),
	})
}

// Success sends a 200 OK response with the unified envelope
func Success(c *gin.Context, data interface{}, message ...string) {
	msg := "success"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	Respond(c, http.StatusOK, true, msg, data)
}

// Created sends a 201 Created response
func Created(c *gin.Context, data interface{}, message ...string) {
	msg := "created"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}
	Respond(c, http.StatusCreated, true, msg, data)
}

// Paginated sends a paginated response using the unified envelope
func Paginated(c *gin.Context, items interface{}, total int64, limit int, page ...int) {
	pageNum := 1
	if len(page) > 0 {
		pageNum = page[0]
	}

	pd := paginatedData{
		Items: items,
		Total: total,
		Limit: limit,
		Page:  pageNum,
	}

	Success(c, pd, "success")
}

// Error sends an error response with custom status code and message
// It keeps the same signature as before and supports an optional error code
func Error(c *gin.Context, statusCode int, message string, errorCode ...string) {
	code := ""
	if len(errorCode) > 0 {
		code = errorCode[0]
	}

	Respond(c, statusCode, false, message, nil, code)
}

// BadRequest sends a 400 Bad Request error
func BadRequest(c *gin.Context, message string, errorCode ...string) {
	Error(c, http.StatusBadRequest, message, errorCode...)
}

// Unauthorized sends a 401 Unauthorized error
func Unauthorized(c *gin.Context, message string, errorCode ...string) {
	Error(c, http.StatusUnauthorized, message, errorCode...)
}

// Forbidden sends a 403 Forbidden error
func Forbidden(c *gin.Context, message string, errorCode ...string) {
	Error(c, http.StatusForbidden, message, errorCode...)
}

// NotFound sends a 404 Not Found error
func NotFound(c *gin.Context, message string, errorCode ...string) {
	Error(c, http.StatusNotFound, message, errorCode...)
}

// Conflict sends a 409 Conflict error
func Conflict(c *gin.Context, message string, errorCode ...string) {
	Error(c, http.StatusConflict, message, errorCode...)
}

// ValidationError sends a 422 Unprocessable Entity error
func ValidationError(c *gin.Context, message string, errorCode ...string) {
	Error(c, http.StatusUnprocessableEntity, message, errorCode...)
}

// InternalServerError sends a 500 Internal Server Error
func InternalServerError(c *gin.Context, message string, errorCode ...string) {
	Error(c, http.StatusInternalServerError, message, errorCode...)
}

// ServiceUnavailable sends a 503 Service Unavailable error
func ServiceUnavailable(c *gin.Context, message string, errorCode ...string) {
	Error(c, http.StatusServiceUnavailable, message, errorCode...)
}

// BindJSONError handles JSON decode errors in request body
func BindJSONError(c *gin.Context, err error) {
	BadRequest(c, "Invalid request format", "INVALID_JSON")
}

// ValidationFailed handles validation errors
func ValidationFailed(c *gin.Context, message string) {
	ValidationError(c, message, "VALIDATION_FAILED")
}

// DatabaseError handles database operation errors
func DatabaseError(c *gin.Context, message string) {
	InternalServerError(c, message, "DATABASE_ERROR")
}

// AuthenticationError handles authentication failures
func AuthenticationError(c *gin.Context, message string) {
	Unauthorized(c, message, "AUTH_FAILED")
}

// AuthorizationError handles authorization failures
func AuthorizationError(c *gin.Context, message string) {
	Forbidden(c, message, "FORBIDDEN")
}
