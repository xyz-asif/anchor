// ================== internal/middleware/auth.go ==================
package middleware

import (
	"net/http"
	"strings"

	"github.com/xyz-asif/gotodo/internal/pkg/token"

	"github.com/gin-gonic/gin"
)

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Support both "Bearer <token>" (case-insensitive) and raw token in header
		fields := strings.Fields(authHeader)
		var tokenString string
		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			tokenString = fields[1]
		} else {
			// Treat the entire header value as the token
			tokenString = authHeader
		}

		claims, err := token.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
}
