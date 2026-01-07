# Dev Login Setup - Easy Token Generation

## Overview

This adds a development-only login endpoint that bypasses Google OAuth. 
**IMPORTANT: Only use in development, remove or disable in production!**

---

## Step 1: Add Dev Login Handler

Add this to `internal/features/auth/handler.go`:

```go
// DevLogin godoc
// @Summary Dev login (DEVELOPMENT ONLY)
// @Description Login without Google OAuth - FOR DEVELOPMENT ONLY
// @Tags auth
// @Accept json
// @Produce json
// @Param request body DevLoginRequest true "Dev login request"
// @Success 200 {object} response.APIResponse{data=LoginResponse}
// @Failure 400 {object} response.APIResponse
// @Router /auth/dev-login [post]
func (h *Handler) DevLogin(c *gin.Context) {
	// Check if dev mode is enabled
	if !h.config.DevMode {
		response.Forbidden(c, "DISABLED", "Dev login is disabled in production")
		return
	}

	var req DevLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", "Invalid request format")
		return
	}

	// Validate
	if req.Email == "" {
		response.BadRequest(c, "VALIDATION_FAILED", "Email is required")
		return
	}

	ctx := c.Request.Context()

	// Try to find existing user by email
	user, err := h.repo.GetUserByEmail(ctx, req.Email)
	if err != nil && user == nil {
		// Create new dev user
		username := generateDevUsername()
		displayName := "Dev User"
		if req.DisplayName != "" {
			displayName = req.DisplayName
		}

		user = &User{
			ID:                primitive.NewObjectID(),
			GoogleID:          "dev_" + primitive.NewObjectID().Hex(),
			Email:             req.Email,
			Username:          username,
			DisplayName:       displayName,
			ProfilePictureURL: "",
			IsVerified:        false,
			FollowerCount:     0,
			FollowingCount:    0,
			AnchorCount:       0,
		}

		if err := h.repo.CreateUser(ctx, user); err != nil {
			response.InternalServerError(c, "CREATE_FAILED", "Failed to create user")
			return
		}
	}

	// Generate JWT token
	token, err := h.generateToken(user)
	if err != nil {
		response.InternalServerError(c, "TOKEN_FAILED", "Failed to generate token")
		return
	}

	response.Success(c, LoginResponse{
		Token:       token,
		User:        h.toUserResponse(user),
		IsNewUser:   user.Username == "" || strings.HasPrefix(user.Username, "dev_"),
		RequiresUsername: user.Username == "" || strings.HasPrefix(user.Username, "dev_"),
	})
}

// Helper to generate dev username
func generateDevUsername() string {
	return "dev_" + primitive.NewObjectID().Hex()[:8]
}
```

---

## Step 2: Add Request DTO

Add to `internal/features/auth/model.go`:

```go
// DevLoginRequest for development login
type DevLoginRequest struct {
	Email       string `json:"email" binding:"required,email"`
	DisplayName string `json:"displayName"`
}
```

---

## Step 3: Add DevMode to Config

Update `internal/config/config.go`:

```go
type Config struct {
	// ... existing fields ...
	DevMode bool
}

func Load() (*Config, error) {
	// ... existing code ...
	
	devMode := os.Getenv("DEV_MODE") == "true"
	
	return &Config{
		// ... existing fields ...
		DevMode: devMode,
	}, nil
}
```

---

## Step 4: Add Route

Update `internal/features/auth/routes.go`:

```go
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config, followService FollowService, anchorService AnchorService) {
	// ... existing setup ...

	auth := router.Group("/auth")
	{
		auth.POST("/google", handler.GoogleLogin)
		auth.POST("/refresh", authMiddleware, handler.RefreshToken)
		auth.GET("/me", authMiddleware, handler.GetCurrentUser)
		auth.POST("/username", authMiddleware, handler.SetUsername)
		auth.GET("/username/check", handler.CheckUsernameAvailability)
		
		// Dev login - only works when DEV_MODE=true
		auth.POST("/dev-login", handler.DevLogin)
	}
}
```

---

## Step 5: Update .env

Add to your `.env` file:

```env
# Development Settings
DEV_MODE=true
```

**IMPORTANT:** Set `DEV_MODE=false` in production!

---

## Usage in Postman

```
POST http://localhost:8080/api/v1/auth/dev-login
Content-Type: application/json

{
    "email": "test@example.com",
    "displayName": "Test User"
}
```

Response:
```json
{
    "success": true,
    "statusCode": 200,
    "data": {
        "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
        "user": {
            "id": "507f1f77bcf86cd799439011",
            "email": "test@example.com",
            "username": "dev_a1b2c3d4",
            "displayName": "Test User"
        },
        "isNewUser": true,
        "requiresUsername": true
    }
}
```

---

## Create Multiple Test Users

```
# User 1
POST /auth/dev-login
{"email": "alice@test.com", "displayName": "Alice"}

# User 2
POST /auth/dev-login
{"email": "bob@test.com", "displayName": "Bob"}

# User 3
POST /auth/dev-login
{"email": "charlie@test.com", "displayName": "Charlie"}
```

Now you can test follow, like, comment features between users!

---

## Security Reminder

⚠️ **NEVER enable DEV_MODE in production!**

Add this check to your main.go or deployment:

```go
if cfg.DevMode && os.Getenv("ENV") == "production" {
    log.Fatal("DEV_MODE cannot be enabled in production!")
}
```