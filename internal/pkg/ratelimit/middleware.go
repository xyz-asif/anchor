package ratelimit

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Middleware creates a rate limiting middleware for Gin
func Middleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use IP address as the key (or user ID if authenticated)
		key := c.ClientIP()

		if !limiter.Allow(key) {
			remaining := limiter.GetRemaining(key)
			resetTime := limiter.GetResetTime(key)

			c.Header("X-RateLimit-Limit", "100")
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", resetTime.Format(time.RFC3339))
			c.Header("Retry-After", "60")

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded. Try again later.",
				"retry_after": "60s",
				"reset_time":  resetTime.Format(time.RFC3339),
				"limit":       100,
				"remaining":   remaining,
			})
			c.Abort()
			return
		}

		// Add rate limit headers
		remaining := limiter.GetRemaining(key)
		resetTime := limiter.GetResetTime(key)

		c.Header("X-RateLimit-Limit", "100")
		c.Header("X-RateLimit-Remaining", string(rune(remaining)))
		c.Header("X-RateLimit-Reset", resetTime.Format(time.RFC3339))

		c.Next()
	}
}

// UserBasedMiddleware creates a rate limiting middleware based on user ID
func UserBasedMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to get user ID from context (set by auth middleware)
		userID := c.GetString("userID")
		if userID == "" {
			// Fallback to IP if no user ID
			userID = c.ClientIP()
		}

		if !limiter.Allow(userID) {
			remaining := limiter.GetRemaining(userID)
			resetTime := limiter.GetResetTime(userID)

			c.Header("X-RateLimit-Limit", "100")
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", resetTime.Format(time.RFC3339))
			c.Header("Retry-After", "60")

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded. Try again later.",
				"retry_after": "60s",
				"reset_time":  resetTime.Format(time.RFC3339),
				"limit":       100,
				"remaining":   remaining,
			})
			c.Abort()
			return
		}

		// Add rate limit headers
		remaining := limiter.GetRemaining(userID)
		resetTime := limiter.GetResetTime(userID)

		c.Header("X-RateLimit-Limit", "100")
		c.Header("X-RateLimit-Remaining", string(rune(remaining)))
		c.Header("X-RateLimit-Reset", resetTime.Format(time.RFC3339))

		c.Next()
	}
}

// CustomKeyMiddleware creates a rate limiting middleware with custom key function
func CustomKeyMiddleware(limiter *RateLimiter, keyFunc func(c *gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFunc(c)
		if key == "" {
			key = c.ClientIP() // Fallback to IP
		}

		if !limiter.Allow(key) {
			remaining := limiter.GetRemaining(key)
			resetTime := limiter.GetResetTime(key)

			c.Header("X-RateLimit-Limit", "100")
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", resetTime.Format(time.RFC3339))
			c.Header("Retry-After", "60")

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded. Try again later.",
				"retry_after": "60s",
				"reset_time":  resetTime.Format(time.RFC3339),
				"limit":       100,
				"remaining":   remaining,
			})
			c.Abort()
			return
		}

		// Add rate limit headers
		remaining := limiter.GetRemaining(key)
		resetTime := limiter.GetResetTime(key)

		c.Header("X-RateLimit-Limit", "100")
		c.Header("X-RateLimit-Remaining", string(rune(remaining)))
		c.Header("X-RateLimit-Reset", resetTime.Format(time.RFC3339))

		c.Next()
	}
}

// DifferentLimitsMiddleware creates middleware with different limits for different endpoints
func DifferentLimitsMiddleware(limiters map[string]*RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Determine which limiter to use based on route
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}

		var limiter *RateLimiter
		for pattern, l := range limiters {
			if route == pattern {
				limiter = l
				break
			}
		}

		// Use default limiter if no specific one found
		if limiter == nil {
			limiter = limiters["default"]
		}

		if limiter == nil {
			c.Next()
			return
		}

		key := c.ClientIP()
		if !limiter.Allow(key) {
			remaining := limiter.GetRemaining(key)
			resetTime := limiter.GetResetTime(key)

			c.Header("X-RateLimit-Limit", "100")
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", resetTime.Format(time.RFC3339))
			c.Header("Retry-After", "60")

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded. Try again later.",
				"retry_after": "60s",
				"reset_time":  resetTime.Format(time.RFC3339),
				"limit":       100,
				"remaining":   remaining,
			})
			c.Abort()
			return
		}

		// Add rate limit headers
		remaining := limiter.GetRemaining(key)
		resetTime := limiter.GetResetTime(key)

		c.Header("X-RateLimit-Limit", "100")
		c.Header("X-RateLimit-Remaining", string(rune(remaining)))
		c.Header("X-RateLimit-Reset", resetTime.Format(time.RFC3339))

		c.Next()
	}
}
