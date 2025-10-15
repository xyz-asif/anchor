package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorGray   = "\033[90m"
)

// Logger configuration
type LoggerConfig struct {
	EnableColors     bool
	LogRequestBody   bool
	LogResponseBody  bool
	MaxBodySize      int64 // Max body size to log (in bytes)
	SkipPaths        []string
	SensitiveHeaders []string
}

// Default configuration - more conservative
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		EnableColors:     true,
		LogRequestBody:   true,
		LogResponseBody:  false, // Only for errors
		MaxBodySize:      2048,  // 2KB limit
		SkipPaths:        []string{"/health", "/metrics", "/ping"},
		SensitiveHeaders: []string{"authorization", "cookie", "x-api-key"},
	}
}

func Logger() gin.HandlerFunc {
	return LoggerWithConfig(DefaultLoggerConfig())
}

func LoggerWithConfig(config LoggerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		// Skip logging for certain paths
		for _, skipPath := range config.SkipPaths {
			if path == skipPath {
				c.Next()
				return
			}
		}

		// Capture request info
		method := c.Request.Method
		ip := c.ClientIP()
		userAgent := c.GetHeader("User-Agent")
		contentType := c.GetHeader("Content-Type")
		queryParams := c.Request.URL.RawQuery

		// Read and restore request body with size limits
		var requestBody string
		if config.LogRequestBody && c.Request.Body != nil && c.Request.ContentLength > 0 {
			// Pre-check content length
			if c.Request.ContentLength > config.MaxBodySize {
				requestBody = "[Request body too large to log]"
			} else {
				bodyBytes, err := io.ReadAll(io.LimitReader(c.Request.Body, config.MaxBodySize))
				if err == nil {
					c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
					requestBody = sanitizeBody(string(bodyBytes), contentType)
				}
			}
		}

		// Log incoming request
		logIncomingRequest(config, method, path, ip, queryParams, contentType, requestBody, userAgent)

		// Use size-limited response writer
		writer := &limitedResponseWriter{
			ResponseWriter: c.Writer,
			maxSize:        config.MaxBodySize,
		}
		c.Writer = writer

		c.Next()

		// Calculate response metrics
		latency := time.Since(start)
		status := writer.Status()
		responseSize := writer.size
		userID := c.GetString("userID")
		email := c.GetString("email")

		// Get response body if needed (smart: only for errors or if explicitly enabled)
		var responseBody string
		if writer.body.Len() > 0 {
			if config.LogResponseBody || status >= 400 {
				responseBody = sanitizeResponseBody(writer.body.String())
			}
		}

		// Log outgoing response
		logOutgoingResponse(config, method, path, status, latency, int(responseSize), userID, email, responseBody)
	}
}

// Size-limited response writer - prevents memory issues
type limitedResponseWriter struct {
	gin.ResponseWriter
	body    bytes.Buffer
	size    int64
	maxSize int64
}

func (w *limitedResponseWriter) Write(b []byte) (int, error) {
	// Always write to actual response
	n, err := w.ResponseWriter.Write(b)

	// Only capture for logging if under size limit
	if w.size+int64(len(b)) <= w.maxSize {
		w.body.Write(b[:n])
	}
	w.size += int64(n)

	return n, err
}

func logIncomingRequest(config LoggerConfig, method, path, ip, query, contentType, body, userAgent string) {
	var methodColor, resetColor string
	if config.EnableColors {
		methodColor = getMethodColor(method)
		resetColor = ColorReset
	}

	// Main request log
	log.Printf("\n%s→ REQUEST%s  %s%s%s %s%s%s",
		ColorCyan, resetColor,
		methodColor, method, resetColor,
		ColorBlue, path, resetColor)

	// Additional details on separate lines for readability
	if ip != "" {
		log.Printf("%s    🌐 IP:%s %s", ColorGray, resetColor, ip)
	}
	if query != "" {
		log.Printf("%s    🔍 Query:%s %s", ColorGray, resetColor, truncateString(query, 100))
	}
	if contentType != "" {
		log.Printf("%s    📋 Content-Type:%s %s", ColorGray, resetColor, contentType)
	}
	if body != "" {
		log.Printf("%s    📄 Body:%s %s", ColorGray, resetColor, body)
	}
	if userAgent != "" && !isBoringUserAgent(userAgent) {
		log.Printf("%s    🖥️  UA:%s %s", ColorGray, resetColor, truncateString(userAgent, 60))
	}
}

func logOutgoingResponse(config LoggerConfig, method, path string, status int, latency time.Duration, size int, userID, email, body string) {
	var statusColor, methodColor, resetColor, statusFlag, statusEmoji string
	if config.EnableColors {
		statusColor = getStatusColor(status)
		methodColor = getMethodColor(method)
		resetColor = ColorReset
	}

	// Determine status flag and emoji based on status code
	switch {
	case status >= 200 && status < 300:
		statusFlag = "SUCCESS"
		statusEmoji = "✅"
		if config.EnableColors {
			statusColor = ColorGreen
		}
	case status >= 300 && status < 400:
		statusFlag = "REDIRECT"
		statusEmoji = "↪️"
		if config.EnableColors {
			statusColor = ColorCyan
		}
	case status >= 400 && status < 500:
		statusFlag = "CLIENT ERROR"
		statusEmoji = "⚠️"
		if config.EnableColors {
			statusColor = ColorYellow
		}
	case status >= 500:
		statusFlag = "SERVER ERROR"
		statusEmoji = "❌"
		if config.EnableColors {
			statusColor = ColorRed
		}
	default:
		statusFlag = "UNKNOWN"
		statusEmoji = "❓"
	}

	// Format size nicely
	sizeStr := formatSize(int64(size))

	// Status header with flag
	log.Printf("%s%s %s [%d]%s", statusColor, statusEmoji, statusFlag, status, resetColor)

	// Main response log
	log.Printf("%s← RESPONSE%s %s%s%s %s%s%s %sTime: %v%s  %sSize: %s%s",
		ColorPurple, resetColor,
		methodColor, method, resetColor,
		ColorBlue, path, resetColor,
		ColorGray, latency, resetColor,
		ColorGray, sizeStr, resetColor)

	// User info if available
	if userID != "" {
		userInfo := userID
		if email != "" {
			userInfo = email
		}
		log.Printf("%s    👤 User:%s %s", ColorGray, resetColor, userInfo)
	}

	// Response body if enabled (already truncated)
	if body != "" {
		log.Printf("%s    📄 Response:%s %s", ColorGray, resetColor, body)
	}

	// Add separator after each request
	log.Printf("%s%s%s\n", ColorGray, strings.Repeat("─", 70), resetColor)
}

// Helper functions
func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
}

func isBoringUserAgent(ua string) bool {
	boring := []string{"insomnia", "postman", "curl", "wget", "httpie"}
	lowerUA := strings.ToLower(ua)
	for _, b := range boring {
		if strings.Contains(lowerUA, b) {
			return true
		}
	}
	return false
}

func getMethodColor(method string) string {
	switch method {
	case "GET":
		return ColorGreen
	case "POST":
		return ColorBlue
	case "PUT":
		return ColorYellow
	case "DELETE":
		return ColorRed
	case "PATCH":
		return ColorPurple
	default:
		return ColorWhite
	}
}

func getStatusColor(status int) string {
	switch {
	case status >= 200 && status < 300:
		return ColorGreen
	case status >= 300 && status < 400:
		return ColorCyan
	case status >= 400 && status < 500:
		return ColorYellow
	case status >= 500:
		return ColorRed
	default:
		return ColorWhite
	}
}

func sanitizeBody(body, contentType string) string {
	if len(body) == 0 {
		return ""
	}

	if len(body) > 1024 {
		return "[Body too large to log]"
	}

	// Try to format JSON nicely
	if strings.Contains(contentType, "application/json") {
		var jsonData interface{}
		if json.Unmarshal([]byte(body), &jsonData) == nil {
			sanitized := hideSensitiveFields(jsonData)
			if formatted, err := json.Marshal(sanitized); err == nil {
				return string(formatted)
			}
		}
	}

	return truncateString(body, 200)
}

func hideSensitiveFields(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			lowerKey := strings.ToLower(key)
			if isSensitiveField(lowerKey) {
				result[key] = "********"
			} else {
				result[key] = hideSensitiveFields(value)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = hideSensitiveFields(item)
		}
		return result
	default:
		return v
	}
}

func isSensitiveField(field string) bool {
	sensitive := []string{"password", "token", "secret", "key", "auth", "credential"}
	for _, s := range sensitive {
		if strings.Contains(field, s) {
			return true
		}
	}
	return false
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func sanitizeResponseBody(body string) string {
	if len(body) == 0 {
		return ""
	}

	// Try to format JSON responses nicely
	var jsonData interface{}
	if json.Unmarshal([]byte(body), &jsonData) == nil {
		if formatted, err := json.MarshalIndent(jsonData, "", "  "); err == nil {
			// Truncate formatted JSON if too long
			if len(formatted) > 500 {
				return string(formatted[:500]) + "...\n}"
			}
			return string(formatted)
		}
	}

	return truncateString(body, 200)
}
