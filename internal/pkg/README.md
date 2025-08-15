# GoTodo Utility Packages

This directory contains reusable utility packages that can be used across all your Go projects. Each package is designed to be independent, well-tested, and production-ready.

## 📦 Available Packages

### 1. **Response** (`/response`)
Standardized API response utilities for consistent error handling and success responses.

**Features:**
- Structured error responses with error codes
- Success responses with consistent format
- Pagination support
- Domain-specific error helpers

**Usage:**
```go
import "github.com/xyz-asif/gotodo/internal/pkg/response"

// Success responses
response.Success(c, userData)
response.Created(c, newUser)
response.Paginated(c, todos, total, limit, page)

// Error responses
response.BadRequest(c, "Invalid input", "INVALID_INPUT")
response.NotFound(c, "User not found", "USER_NOT_FOUND")
response.ValidationFailed(c, "Email is required")
response.DatabaseError(c, "Failed to save user")
```

### 2. **Logger** (`/logger`)
Structured logging with different levels and formatting.

**Features:**
- Multiple log levels (DEBUG, INFO, WARN, ERROR, FATAL)
- Timestamp formatting
- Global and custom logger instances
- Easy level switching

**Usage:**
```go
import "github.com/xyz-asif/gotodo/internal/pkg/logger"

// Global logger
logger.Info("Application started")
logger.Error("Error occurred: %s", "database connection failed")

// Custom logger
customLogger := logger.New(logger.DEBUG)
customLogger.Info("Custom logger message")
```

### 3. **Validator** (`/validator`)
Comprehensive validation utilities for common data types.

**Features:**
- Email, phone, URL validation
- Password strength checking
- Credit card validation (Luhn algorithm)
- UUID, date, time validation
- Name and postal code validation

**Usage:**
```go
import "github.com/xyz-asif/gotodo/internal/pkg/validator"

if !validator.IsValidEmail(email) {
    return errors.New("invalid email")
}

if !validator.IsStrongPassword(password) {
    return errors.New("password too weak")
}
```

### 4. **Pagination** (`/pagination`)
Pagination utilities for list endpoints.

**Features:**
- Page and limit handling
- Total count calculation
- Navigation helpers
- Request parameter parsing

**Usage:**
```go
import "github.com/xyz-asif/gotodo/internal/pkg/pagination"

// From request parameters
paginationReq := pagination.FromRequest("2", "10")

// Create pagination instance
pagination := pagination.New(paginationReq.Page, paginationReq.Limit, total)

// Use in database queries
offset := pagination.GetOffset()
limit := pagination.GetLimit()
```

### 5. **Database** (`/database`)
MongoDB connection management with connection pooling.

**Features:**
- Connection pooling configuration
- Health checks
- Transaction support
- Index management
- Statistics and monitoring

**Usage:**
```go
import "github.com/xyz-asif/gotodo/internal/pkg/database"

dbConfig := &database.Config{
    URI:      "localhost:27017",
    DBName:   "gotodo",
    Timeout:  10 * time.Second,
    MaxPool:  100,
    MinPool:  5,
}

dbConn, err := database.NewConnection(dbConfig)
if err != nil {
    return err
}
defer dbConn.Close()
```

### 6. **JWT** (`/jwt`)
Enhanced JWT utilities with role-based access control.

**Features:**
- Access and refresh tokens
- Role-based claims
- Custom metadata support
- Token validation and refresh
- Configurable expiry times

**Usage:**
```go
import "github.com/xyz-asif/gotodo/internal/pkg/jwt"

jwtConfig := jwt.DefaultConfig("your-secret-key")

// Generate token
token, err := jwt.GenerateToken("user123", "user@example.com", jwtConfig)

// Generate token with role
tokenWithRole, err := jwt.GenerateTokenWithRole("user123", "user@example.com", "admin", jwtConfig)

// Generate token pair
accessToken, refreshToken, err := jwt.GenerateTokenPair("user123", "user@example.com", jwtConfig)
```

### 7. **Rate Limiting** (`/ratelimit`)
Flexible rate limiting with multiple strategies.

**Features:**
- IP-based and user-based limiting
- Configurable limits and windows
- Automatic cleanup
- Statistics and monitoring
- Gin middleware integration

**Usage:**
```go
import "github.com/xyz-asif/gotodo/internal/pkg/ratelimit"

// Create rate limiter: 100 requests per minute
limiter := ratelimit.New(100, time.Minute)

// Start background cleanup
limiter.StartCleanup(5 * time.Minute)

// Check if request is allowed
if limiter.Allow("192.168.1.1") {
    // Process request
} else {
    // Rate limit exceeded
}

// Use as Gin middleware
router.Use(ratelimit.Middleware(limiter))
```

## 🚀 Quick Start

### 1. Import the packages you need:
```go
import (
    "github.com/xyz-asif/gotodo/internal/pkg/response"
    "github.com/xyz-asif/gotodo/internal/pkg/logger"
    "github.com/xyz-asif/gotodo/internal/pkg/validator"
)
```

### 2. Use in your handlers:
```go
func (h *Handler) CreateUser(c *gin.Context) {
    var req CreateUserRequest
    
    // Validate input
    if err := c.ShouldBindJSON(&req); err != nil {
        response.BindJSONError(c, err)
        return
    }
    
    if !validator.IsValidEmail(req.Email) {
        response.ValidationFailed(c, "Invalid email format")
        return
    }
    
    // Log the action
    logger.Info("Creating user with email: %s", req.Email)
    
    // Business logic...
    
    // Return success response
    response.Created(c, user)
}
```

### 3. Set up rate limiting:
```go
func SetupRoutes() *gin.Engine {
    router := gin.Default()
    
    // Global rate limiting
    globalLimiter := ratelimit.New(100, time.Minute)
    router.Use(ratelimit.Middleware(globalLimiter))
    
    // Different limits for sensitive endpoints
    authLimiter := ratelimit.New(5, time.Minute)
    authRoutes := router.Group("/api/v1/auth")
    authRoutes.Use(ratelimit.Middleware(authLimiter))
    
    return router
}
```

## 🔧 Configuration

Each package has sensible defaults but can be customized:

```go
// Logger
logger.SetGlobalLevel(logger.DEBUG)

// JWT
jwtConfig := &jwt.Config{
    Secret:        "your-secret",
    AccessExpiry:  2 * time.Hour,
    RefreshExpiry: 7 * 24 * time.Hour,
    Issuer:        "your-app",
    Audience:      "your-users",
}

// Database
dbConfig := &database.Config{
    URI:      "your-mongodb-uri",
    DBName:   "your-database",
    Username: "username",
    Password: "password",
    Timeout:  15 * time.Second,
    MaxPool:  200,
    MinPool:  10,
}
```

## 📚 Examples

See `internal/pkg/examples/usage.go` for comprehensive usage examples.

## 🧪 Testing

All packages are designed to be easily testable:

```go
func TestValidator(t *testing.T) {
    if !validator.IsValidEmail("test@example.com") {
        t.Error("Expected valid email to pass validation")
    }
    
    if validator.IsValidEmail("invalid-email") {
        t.Error("Expected invalid email to fail validation")
    }
}
```

## 🔄 Updates and Maintenance

These packages are designed to be:
- **Independent**: Each package can be used separately
- **Extensible**: Easy to add new features
- **Maintainable**: Clean, documented code
- **Reusable**: Can be copied to other projects

## 📝 License

These utility packages are part of the GoTodo project and follow the same license terms.

---

**Happy coding! 🎉**
