package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"

	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

// NewAuthMiddleware creates a Gin middleware for JWT authentication
func NewAuthMiddleware(repo *auth.Repository, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, "Authorization header required", "AUTH_REQUIRED")
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.Unauthorized(c, "Invalid authorization format", "INVALID_AUTH_FORMAT")
			c.Abort()
			return
		}

		tokenString := parts[1]
		userID, err := auth.ValidateJWT(tokenString, cfg)
		if err != nil {
			response.Unauthorized(c, "Invalid or expired token", "INVALID_TOKEN")
			c.Abort()
			return
		}

		user, err := repo.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			// differentiate between db error and not found if possible, but 401 is safest for auth
			response.Unauthorized(c, "User not found", "USER_NOT_FOUND")
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Next()
	}
}

// OptionalAuthMiddleware attempts to authenticate but doesn't require it
// If valid token present: sets "user" in context
// If no token or invalid token: continues without setting user (no abort)
func OptionalAuthMiddleware(repo *auth.Repository, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.Next()
			return
		}

		tokenString := parts[1]
		userID, err := auth.ValidateJWT(tokenString, cfg)
		if err != nil {
			// Invalid token - continue without auth (don't abort)
			c.Next()
			return
		}

		user, err := repo.GetUserByID(c.Request.Context(), userID)
		if err != nil {
			c.Next()
			return
		}

		c.Set("user", user)
		c.Next()
	}
}
