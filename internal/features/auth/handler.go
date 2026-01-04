package auth

import (
	"fmt"
	"strings"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Handler handles HTTP requests for auth feature
type Handler struct {
	repo           *Repository
	firebaseClient *auth.Client
	config         *config.Config
}

// NewHandler creates a new auth handler
func NewHandler(repo *Repository, firebaseClient *auth.Client, cfg *config.Config) *Handler {
	return &Handler{
		repo:           repo,
		firebaseClient: firebaseClient,
		config:         cfg,
	}
}

// GoogleLogin handles Google OAuth login/registration
// @Summary Login with Google
// @Description Authenticate user using Google ID token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body GoogleAuthRequest true "Google ID Token"
// @Success 200 {object} response.APIResponse{data=AuthResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /auth/google [post]
func (h *Handler) GoogleLogin(c *gin.Context) {
	var req GoogleAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	// Verify Google Token ///
	googleUser, err := VerifyGoogleToken(c.Request.Context(), req.GoogleIDToken, h.config.GoogleClientID)
	if err != nil {
		fmt.Printf("Token verification failed: %v\n", err) // Simple logging for debugging
		response.Unauthorized(c, "Invalid Google token", "INVALID_TOKEN")
		return
	}

	// Normalize email to ensure consistency
	googleUser.Email = strings.ToLower(googleUser.Email)

	// Check if user exists by Google ID
	user, err := h.repo.GetUserByGoogleID(c.Request.Context(), googleUser.UID)
	if err != nil {
		response.BadRequest(c, "Database error", "DATABASE_ERROR")
		return
	}

	isNewUser := false
	if user == nil {
		// User not found by Google ID. Check if user exists by email to link account.
		userByEmail, err := h.repo.GetUserByEmail(c.Request.Context(), googleUser.Email)
		if err != nil {
			response.BadRequest(c, "Database error", "DATABASE_ERROR")
			return
		}

		if userByEmail != nil {
			// User exists with this email. Link Google ID.
			user = userByEmail
			updates := map[string]interface{}{
				"googleId":  googleUser.UID,
				"updatedAt": time.Now(),
			}
			if err := h.repo.UpdateUser(c.Request.Context(), user.ID.Hex(), updates); err != nil {
				fmt.Printf("Failed to link Google ID: %v\n", err)
				response.BadRequest(c, "Failed to link account", "DATABASE_ERROR")
				return
			}
			// IsNewUser remains false as we are linking to existing account
		} else {
			// Create new user
			isNewUser = true
			baseUsername := GenerateUniqueUsername(googleUser.Name)
			username := baseUsername

			// Ensure unique username
			counter := 1
			for {
				exists, err := h.repo.UsernameExists(c.Request.Context(), username)
				if err != nil {
					response.BadRequest(c, "Database error", "DATABASE_ERROR")
					return
				}
				if !exists {
					break
				}
				username = fmt.Sprintf("%s%d", baseUsername, counter)
				counter++
			}

			user = &User{
				ID:                primitive.NewObjectID(), // Explicitly generate ID here or let DB do it, but repository handles InsertOne. Repository CreateUser handles ID generation if missing, but better to follow struct. Actually CreateUser handles it.
				GoogleID:          googleUser.UID,
				Email:             googleUser.Email,
				Username:          username,
				DisplayName:       googleUser.Name,
				ProfilePictureURL: googleUser.Picture,
				FollowerCount:     0,
				FollowingCount:    0,
				AnchorCount:       0,
				UsernameChanged:   false,
				IsVerified:        false,
				JoinedAt:          time.Now(),
			}

			if err := h.repo.CreateUser(c.Request.Context(), user); err != nil {
				fmt.Printf("CreateUser failed: %v\n", err) // Log the specific error
				response.BadRequest(c, "Failed to create user", "DATABASE_ERROR")
				return
			}
		}
	}

	// Generate JWT
	accessToken, err := GenerateJWT(user.ID.Hex(), h.config)
	if err != nil {
		response.BadRequest(c, "Failed to generate token", "AUTH_FAILED")
		return
	}

	stcode := 200
	if isNewUser {
		stcode = 201
	}

	response.Respond(c, stcode, true, "Login successful", AuthResponse{
		User:        user,
		AccessToken: accessToken,
	})
}

// GetMe returns the authenticated user's profile
// @Summary Get current user
// @Description Get the authenticated user's profile
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=User}
// @Failure 401 {object} response.APIResponse
// @Router /auth/me [get]
func (h *Handler) GetMe(c *gin.Context) {
	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}

	user, ok := val.(*User)
	if !ok {
		response.BadRequest(c, "User context error", "INTERNAL_ERROR")
		return
	}

	response.Success(c, user)
}

// UpdateProfile updates the user's profile
// @Summary Update user profile
// @Description Update display name and/or bio
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateProfileRequest true "Profile updates"
// @Success 200 {object} response.APIResponse{data=User}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /auth/profile [patch]
func (h *Handler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*User)
	if !ok {
		response.BadRequest(c, "User context error", "INTERNAL_ERROR")
		return
	}

	updates := make(map[string]interface{})

	if req.DisplayName != "" {
		if err := ValidateDisplayName(req.DisplayName); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		updates["displayName"] = req.DisplayName
	}

	if req.Bio != "" {
		if err := ValidateBio(req.Bio); err != nil {
			response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
			return
		}
		updates["bio"] = req.Bio
	}

	if len(updates) == 0 {
		response.Success(c, user)
		return
	}

	if err := h.repo.UpdateUser(c.Request.Context(), user.ID.Hex(), updates); err != nil {
		response.BadRequest(c, "Failed to update profile", "DATABASE_ERROR")
		return
	}

	// Fetch updated user
	updatedUser, err := h.repo.GetUserByID(c.Request.Context(), user.ID.Hex())
	if err != nil {
		response.BadRequest(c, "Failed to fetch updated user", "DATABASE_ERROR")
		return
	}

	response.Success(c, updatedUser)
}

// UpdateUsername updates the user's username
// @Summary Update username
// @Description Change username (can only be done once)
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateUsernameRequest true "New username"
// @Success 200 {object} response.APIResponse{data=User}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /auth/username [patch]
func (h *Handler) UpdateUsername(c *gin.Context) {
	var req UpdateUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*User)
	if !ok {
		response.BadRequest(c, "User context error", "INTERNAL_ERROR")
		return
	}

	if user.UsernameChanged {
		response.BadRequest(c, "Username can only be changed once", "USERNAME_ALREADY_CHANGED")
		return
	}

	if err := ValidateUsername(req.Username); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))

	usernameExists, err := h.repo.UsernameExists(c.Request.Context(), req.Username)
	if err != nil {
		response.BadRequest(c, "Database error", "DATABASE_ERROR")
		return
	}
	if usernameExists {
		response.BadRequest(c, "Username already taken", "USERNAME_TAKEN")
		return
	}

	updates := map[string]interface{}{
		"username":          req.Username,
		"usernameChanged":   true,
		"usernameChangedAt": time.Now(),
	}

	if err := h.repo.UpdateUser(c.Request.Context(), user.ID.Hex(), updates); err != nil {
		response.BadRequest(c, "Failed to update username", "DATABASE_ERROR")
		return
	}

	// Fetch updated user
	updatedUser, err := h.repo.GetUserByID(c.Request.Context(), user.ID.Hex())
	if err != nil {
		response.BadRequest(c, "Failed to fetch updated user", "DATABASE_ERROR")
		return
	}

	response.Success(c, updatedUser)
}
