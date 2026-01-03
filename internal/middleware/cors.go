// ================== internal/middleware/cors.go ==================
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func CORS(allowedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Decide which origin to allow
		allowOrigin := ""
		if allowedOrigin == "*" && origin != "" {
			// With credentials, wildcard is not allowed. Echo the request origin.
			allowOrigin = origin
		} else if origin == allowedOrigin {
			allowOrigin = origin
		}

		if allowOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowOrigin)
		}

		// Always vary on origin and requested headers/method
		c.Header("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		// Reflect requested headers if present, otherwise set a sane default
		reqHeaders := c.Request.Header.Get("Access-Control-Request-Headers")
		if strings.TrimSpace(reqHeaders) == "" {
			reqHeaders = "Content-Type, Authorization"
		}
		c.Header("Access-Control-Allow-Headers", reqHeaders)

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
