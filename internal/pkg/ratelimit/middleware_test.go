package ratelimit

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMiddleware_RateLimitExceeded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	lim := New(0, time.Minute) // limit 0 -> always deny
	r := gin.New()
	r.Use(Middleware(lim))
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, 429, w.Code)
	var body map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	require.Equal(t, false, body["success"])
	require.Equal(t, float64(429), body["statusCode"])
	require.Equal(t, "Rate limit exceeded. Try again later.", body["message"])
	// make sure our extra fields are present in data
	data := body["data"].(map[string]any)
	require.Contains(t, data, "retry_after")
	require.Contains(t, data, "reset_time")
}
