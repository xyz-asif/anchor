package examples

import (
	"time"

	"github.com/xyz-asif/gotodo/internal/pkg/logger"
	"github.com/xyz-asif/gotodo/internal/pkg/pagination"
	"github.com/xyz-asif/gotodo/internal/pkg/ratelimit"
	"github.com/xyz-asif/gotodo/internal/pkg/validator"
)

// ExampleUsage demonstrates how to use all utility packages
func ExampleUsage() {
	// ================== LOGGER USAGE ==================
	logger.SetGlobalLevel(logger.INFO)
	logger.Info("Application started")
	logger.Debug("Debug information")
	logger.Warn("Warning message")
	logger.Error("Error occurred: %s", "database connection failed")

	// Create custom logger
	customLogger := logger.New(logger.DEBUG)
	customLogger.Info("Custom logger message")

	// ================== VALIDATOR USAGE ==================
	email := "user@example.com"
	if validator.IsValidEmail(email) {
		logger.Info("Valid email: %s", email)
	}

	password := "StrongPass123!"
	if validator.IsStrongPassword(password) {
		logger.Info("Strong password")
	}

	phone := "+1234567890"
	if validator.IsValidPhone(phone) {
		logger.Info("Valid phone: %s", phone)
	}

	// ================== PAGINATION USAGE ==================
	// Create pagination from request parameters
	paginationReq := pagination.FromRequest("2", "10")

	// Create pagination instance
	pagination := pagination.New(paginationReq.Page, paginationReq.Limit, 100)

	logger.Info("Page: %d, Total: %d, Pages: %d",
		pagination.GetCurrentPage(),
		pagination.GetTotalItems(),
		pagination.GetTotalPages())

	// ================== DATABASE USAGE ==================
	// dbConfig := &database.Config{
	// 	URI:     "localhost:27017",
	// 	DBName:  "gotodo",
	// 	Timeout: 10 * time.Second,
	// 	MaxPool: 100,
	// 	MinPool: 5,
	// }

	// // In real usage, you would handle the error
	// dbConn, err := database.NewConnection(dbConfig)
	// if err != nil {
	//     logger.Error("Failed to connect to database: %v", err)
	//     return
	// }
	// defer dbConn.Close()

	logger.Info("Database configuration created")

	// ================== JWT USAGE ==================
	// jwtConfig := jwt.DefaultConfig("your-secret-key")

	// Generate token
	// token, err := jwt.GenerateToken("user123", "user@example.com", jwtConfig)
	// if err != nil {
	//     logger.Error("Failed to generate token: %v", err)
	//     return
	// }

	// Generate token with role
	// tokenWithRole, err := jwt.GenerateTokenWithRole("user123", "user@example.com", "admin", jwtConfig)

	// Generate token pair (access + refresh)
	// accessToken, refreshToken, err := jwt.GenerateTokenPair("user123", "user@example.com", jwtConfig)

	logger.Info("JWT configuration created")

	// ================== RATE LIMITING USAGE ==================
	// Create rate limiter: 100 requests per minute
	limiter := ratelimit.New(100, time.Minute)

	// Start background cleanup every 5 minutes
	limiter.StartCleanup(5 * time.Minute)

	// Check if request is allowed
	if limiter.Allow("192.168.1.1") {
		logger.Info("Request allowed")
	} else {
		logger.Warn("Rate limit exceeded")
	}

	// Get remaining requests
	remaining := limiter.GetRemaining("192.168.1.1")
	logger.Info("Remaining requests: %d", remaining)

	// ================== RESPONSE USAGE ==================
	// This would be used in HTTP handlers
	// c is gin.Context

	// Success responses
	// response.Success(c, userData)
	// response.Created(c, newUser)
	// response.Paginated(c, todos, total, limit, page)

	// Error responses
	// response.BadRequest(c, "Invalid input", "INVALID_INPUT")
	// response.NotFound(c, "User not found", "USER_NOT_FOUND")
	// response.ValidationFailed(c, "Email is required")
	// response.DatabaseError(c, "Failed to save user")

	logger.Info("All utility packages initialized successfully")
}

// ExampleGinHandler shows how to use utilities in a Gin handler
func ExampleGinHandler() {
	// This is a pseudo-code example showing how to use utilities in handlers

	// func (h *Handler) CreateUser(c *gin.Context) {
	//     var req CreateUserRequest
	//
	//     // Use response utilities for JSON binding errors
	//     if err := c.ShouldBindJSON(&req); err != nil {
	//         response.BindJSONError(c, err)
	//         return
	//     }
	//
	//     // Use validator utilities
	//     if !validator.IsValidEmail(req.Email) {
	//         response.ValidationFailed(c, "Invalid email format")
	//         return
	//     }
	//
	//     if !validator.IsStrongPassword(req.Password) {
	//         response.ValidationFailed(c, "Password must be at least 8 characters with uppercase, lowercase, number, and special character")
	//         return
	//     }
	//
	//     // Use logger
	//     logger.Info("Creating user with email: %s", req.Email)
	//
	//     // Business logic...
	//
	//     // Use response utilities for success
	//     response.Created(c, user)
	// }

	// func (h *Handler) ListUsers(c *gin.Context) {
	//     // Get pagination from query params
	//     pageStr := c.Query("page")
	//     limitStr := c.Query("limit")
	//
	//     paginationReq := pagination.FromRequest(pageStr, limitStr)
	//
	//     // Get users from database with pagination
	//     users, total, err := h.repo.FindAll(c.Request.Context(), paginationReq.Page, paginationReq.Limit)
	//     if err != nil {
	//         response.DatabaseError(c, "Failed to fetch users")
	//         return
	//     }
	//
	//     // Create pagination metadata
	//     pagination := pagination.New(paginationReq.Page, paginationReq.Limit, total)
	//
	//     // Use response utilities for paginated response
	//     response.Paginated(c, users, total, paginationReq.Limit, paginationReq.Page)
	// }
}

// ExampleRateLimitSetup shows how to set up rate limiting in routes
func ExampleRateLimitSetup() {
	// This is a pseudo-code example showing how to set up rate limiting

	// func SetupRoutes() *gin.Engine {
	//     router := gin.Default()
	//
	//     // Global rate limiting: 100 requests per minute
	//     globalLimiter := ratelimit.New(100, time.Minute)
	//     router.Use(ratelimit.Middleware(globalLimiter))
	//
	//     // Different limits for different endpoints
	//     authLimiter := ratelimit.New(5, time.Minute)      // 5 login attempts per minute
	//     todoLimiter := ratelimit.New(100, time.Minute)    // 100 todo operations per minute
	//
	//     // Apply specific limiters to route groups
	//     authRoutes := router.Group("/api/v1/auth")
	//     authRoutes.Use(ratelimit.Middleware(authLimiter))
	//
	//     todoRoutes := router.Group("/api/v1/todos")
	//     todoRoutes.Use(ratelimit.Middleware(todoLimiter))
	//
	//     return router
	// }
}
