package auth

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	idToken "github.com/xyz-asif/gotodo/internal/pkg/jwt"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

// NewAuthMiddleware creates a Gin middleware for JWT authentication
func NewAuthMiddleware(repo *Repository, cfg *config.Config) gin.HandlerFunc {
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
		claims, err := idToken.ValidateToken(tokenString, cfg.JWTSecret)
		if err != nil {
			response.Unauthorized(c, "Invalid or expired token", "INVALID_TOKEN")
			c.Abort()
			return
		}
		userID := claims.UserID

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
