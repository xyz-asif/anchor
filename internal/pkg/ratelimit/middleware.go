package ratelimit

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

// enforce applies limiter to key. It writes the X-RateLimit-* headers and, when the
// limit is exceeded, a 429 response. It returns false if the request was rejected.
func enforce(c *gin.Context, limiter *RateLimiter, key string) bool {
	allowed := limiter.Allow(key)
	remaining := limiter.GetRemaining(key)
	resetTime := limiter.GetResetTime(key)

	c.Header("X-RateLimit-Limit", strconv.Itoa(limiter.limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

	if allowed {
		return true
	}

	retryAfter := int(math.Ceil(time.Until(resetTime).Seconds()))
	if retryAfter < 1 {
		retryAfter = 1
	}
	c.Header("Retry-After", strconv.Itoa(retryAfter))

	response.Respond(c, http.StatusTooManyRequests, false, "Rate limit exceeded. Try again later.", map[string]interface{}{
		"retry_after": strconv.Itoa(retryAfter) + "s",
		"reset_time":  resetTime.Format(time.RFC3339),
		"limit":       limiter.limit,
		"remaining":   remaining,
	}, "RATE_LIMIT_EXCEEDED")
	c.Abort()
	return false
}

// Middleware rate-limits every request by client IP
func Middleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enforce(c, limiter, c.ClientIP()) {
			c.Next()
		}
	}
}

// UserBasedMiddleware rate-limits by the "userID" context value, falling back to client IP
func UserBasedMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return CustomKeyMiddleware(limiter, func(c *gin.Context) string {
		return c.GetString("userID")
	})
}

// CustomKeyMiddleware rate-limits by a custom key, falling back to client IP when it is empty
func CustomKeyMiddleware(limiter *RateLimiter, keyFunc func(c *gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := keyFunc(c)
		if key == "" {
			key = c.ClientIP()
		}
		if enforce(c, limiter, key) {
			c.Next()
		}
	}
}

// DifferentLimitsMiddleware picks a limiter by route path (c.FullPath()), using the
// "default" entry when no path matches. Requests pass through if there is no limiter.
func DifferentLimitsMiddleware(limiters map[string]*RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		limiter, ok := limiters[route]
		if !ok {
			limiter = limiters["default"]
		}
		if limiter == nil {
			c.Next()
			return
		}
		if enforce(c, limiter, c.ClientIP()) {
			c.Next()
		}
	}
}

// RouteLimits applies a limiter only to matching routes. Keys are "METHOD /full/route/path"
// using Gin route patterns, e.g. "POST /api/v1/anchors/:id/items/upload". Routes sharing a
// limiter share one per-IP budget. Unmatched routes pass through untouched.
// Register it with router.Use before the routes are added.
func RouteLimits(rules map[string]*RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		limiter, ok := rules[c.Request.Method+" "+c.FullPath()]
		if !ok {
			c.Next()
			return
		}
		if enforce(c, limiter, c.ClientIP()) {
			c.Next()
		}
	}
}
