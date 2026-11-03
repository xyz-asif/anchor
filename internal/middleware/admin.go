package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

// RequireAdmin allows the request only if the authenticated user's email is listed in
// ADMIN_EMAILS. It must run after NewAuthMiddleware.
func RequireAdmin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		val, exists := c.Get("user")
		user, ok := val.(*auth.User)
		if !exists || !ok || !cfg.IsAdminEmail(user.Email) {
			response.Forbidden(c, "Admin access required", "ADMIN_REQUIRED")
			c.Abort()
			return
		}
		c.Next()
	}
}
