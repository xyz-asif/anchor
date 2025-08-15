package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse represents a standard error payload returned by the API
type ErrorResponse struct {
	Error string `json:"error" example:"Invalid token"`
	Code  string `json:"code,omitempty" example:"AUTH_INVALID_TOKEN"`
}

// SuccessResponse represents a standard success payload
type SuccessResponse struct {
	Status string      `json:"status" example:"success"`
	Data   interface{} `json:"data"`
}

// PaginatedResponse represents a paginated list response
type PaginatedResponse struct {
	Status string      `json:"status" example:"success"`
	Data   interface{} `json:"data"`
	Total  int64       `json:"total" example:"25"`
	Limit  int         `json:"limit" example:"10"`
	Page   int         `json:"page,omitempty" example:"1"`
}

// Success sends a 200 OK response with data
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, SuccessResponse{
		Status: "success",
		Data:   data,
	})
}

// Created sends a 201 Created response
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, SuccessResponse{
		Status: "success",
		Data:   data,
	})
}

// Paginated sends a paginated response
func Paginated(c *gin.Context, data interface{}, total int64, limit int, page ...int) {
	pageNum := 1
	if len(page) > 0 {
		pageNum = page[0]
	}

	c.JSON(http.StatusOK, PaginatedResponse{
		Status: "success",
		Data:   data,
		Total:  total,
		Limit:  limit,
		Page:   pageNum,
	})
}

// Error sends an error response with custom status code and message
func Error(c *gin.Context, statusCode int, message string, errorCode ...string) {
	code := ""
	if len(errorCode) > 0 {
		code = errorCode[0]
	}

	c.JSON(statusCode, ErrorResponse{
		Error: message,
		Code:  code,
	})
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
