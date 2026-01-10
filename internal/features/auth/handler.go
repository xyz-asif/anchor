package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	idToken "github.com/xyz-asif/gotodo/internal/pkg/jwt"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// FollowService defines the interface for follow operations to avoid import cycle
type FollowService interface {
	GetStatus(ctx context.Context, followerID, followedID primitive.ObjectID) (bool, bool, error)
}

// AnchorService defines the interface for anchor operations to avoid import cycle
type AnchorService interface {
	GetPinnedAnchors(ctx context.Context, userID primitive.ObjectID, includePrivate bool) ([]PinnedAnchorData, error)
	DeleteAllByUser(ctx context.Context, userID primitive.ObjectID) error
}

// PinnedAnchorData represents anchor data returned from anchor service
type PinnedAnchorData struct {
	ID              primitive.ObjectID
	Title           string
	Description     string
	CoverMediaType  string
	CoverMediaValue string
	Visibility      string
	ItemCount       int
	LikeCount       int
	CloneCount      int
	CreatedAt       time.Time
}

// Handler handles HTTP requests for auth feature
type Handler struct {
	repo           *Repository
	firebaseClient *auth.Client
	config         *config.Config
	cloudinary     *cloudinary.Service
	followService  FollowService
	anchorService  AnchorService
}

// NewHandler creates a new auth handler
func NewHandler(repo *Repository, firebaseClient *auth.Client, cfg *config.Config, cld *cloudinary.Service, followService FollowService, anchorService AnchorService) *Handler {
	return &Handler{
		repo:           repo,
		firebaseClient: firebaseClient,
		config:         cfg,
		cloudinary:     cld,
		followService:  followService,
		anchorService:  anchorService,
	}
}

func (h *Handler) getJWTConfig() *idToken.Config {
	return &idToken.Config{
		Secret:        h.config.JWTSecret,
		AccessExpiry:  time.Duration(h.config.JWTExpireHours) * time.Hour,
		RefreshExpiry: time.Duration(h.config.RefreshTokenExpireHours) * time.Hour,
		Issuer:        "gotodo-api",
		Audience:      "gotodo-users",
		SigningMethod: jwt.SigningMethodHS256,
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

	// Verify Google Token
	googleUser, err := VerifyGoogleToken(c.Request.Context(), req.GoogleIDToken, h.config.GoogleClientID)
	if err != nil {
		fmt.Printf("Token verification failed: %v\n", err)
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
				fmt.Printf("CreateUser failed: %v\n", err)
				response.BadRequest(c, "Failed to create user", "DATABASE_ERROR")
				return
			}
		}
	}

	// Generate Token Pair
	jwtConfig := h.getJWTConfig()
	accessToken, refreshToken, err := idToken.GenerateTokenPair(user.ID.Hex(), user.Email, jwtConfig)
	if err != nil {
		response.BadRequest(c, "Failed to generate tokens", "AUTH_FAILED")
		return
	}

	// Save Refresh Token
	claims, _ := idToken.GetTokenClaims(refreshToken)
	if claims != nil {
		tokenID := claims.ID // JTI
		session := &RefreshTokenSession{
			ID:        primitive.NewObjectID(),
			UserID:    user.ID,
			TokenID:   tokenID,
			ExpiresAt: claims.ExpiresAt.Time,
			CreatedAt: time.Now(),
			Revoked:   false,
			UserAgent: c.Request.UserAgent(),
			IPAddress: c.ClientIP(),
		}
		if err := h.repo.SaveRefreshToken(c.Request.Context(), session); err != nil {
			// Log error but proceed? Or fail? Better fail for consistency.
			fmt.Printf("Failed to save refresh token: %v\n", err)
		}
	}

	stcode := 200
	if isNewUser {
		stcode = 201
	}

	response.Respond(c, stcode, true, "Login successful", AuthResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

// DevLogin godoc
// @Summary Dev login (DEVELOPMENT ONLY)
// @Description Login without Google OAuth - FOR DEVELOPMENT/TESTING ONLY
// @Tags auth
// @Accept json
// @Produce json
// @Param request body DevLoginRequest true "Dev login request"
// @Success 200 {object} response.APIResponse{data=LoginResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
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

	ctx := c.Request.Context()

	// Try to find existing user by email
	user, err := h.repo.GetUserByEmail(ctx, req.Email)
	if err != nil || user == nil {
		// Create new dev user
		displayName := "Dev User"
		if req.DisplayName != "" {
			displayName = req.DisplayName
		}

		user = &User{
			ID:                primitive.NewObjectID(),
			GoogleID:          "dev_" + primitive.NewObjectID().Hex(),
			Email:             req.Email,
			Username:          "dev_" + primitive.NewObjectID().Hex()[:8],
			DisplayName:       displayName,
			ProfilePictureURL: "",
			IsVerified:        false,
			FollowerCount:     0,
			FollowingCount:    0,
			AnchorCount:       0,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}

		if err := h.repo.CreateUser(ctx, user); err != nil {
			response.InternalServerError(c, "CREATE_FAILED", "Failed to create user")
			return
		}
	}

	// Generate Token Pair
	jwtConfig := h.getJWTConfig()
	accessToken, refreshToken, err := idToken.GenerateTokenPair(user.ID.Hex(), user.Email, jwtConfig)
	if err != nil {
		response.InternalServerError(c, "TOKEN_FAILED", "Failed to generate tokens")
		return
	}

	// Save Refresh Token
	claims, _ := idToken.GetTokenClaims(refreshToken)
	if claims != nil {
		tokenID := claims.ID // JTI
		session := &RefreshTokenSession{
			ID:        primitive.NewObjectID(),
			UserID:    user.ID,
			TokenID:   tokenID,
			ExpiresAt: claims.ExpiresAt.Time,
			CreatedAt: time.Now(),
			Revoked:   false,
			UserAgent: c.Request.UserAgent(),
			IPAddress: c.ClientIP(),
		}
		if err := h.repo.SaveRefreshToken(c.Request.Context(), session); err != nil {
			fmt.Printf("Failed to save refresh token: %v\n", err)
		}
	}

	// Determine if username setup is needed
	needsUsername := user.Username == "" || strings.HasPrefix(user.Username, "dev_")

	response.Success(c, LoginResponse{
		Token:            accessToken,
		RefreshToken:     refreshToken,
		User:             user,
		IsNewUser:        needsUsername,
		RequiresUsername: needsUsername,
	})
}

// RefreshToken handles token refresh
// @Summary Refresh access token
// @Description Get a new access token using a valid refresh token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RefreshTokenRequest true "Refresh token"
// @Success 200 {object} response.APIResponse{data=AuthResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /auth/refresh [post]
func (h *Handler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	// Validate Token Signature
	claims, err := idToken.ValidateToken(req.RefreshToken, h.config.JWTSecret)
	if err != nil {
		response.Unauthorized(c, "Invalid refresh token", "INVALID_TOKEN")
		return
	}

	// Check if token exists in DB and is not revoked
	// Note: We need JTI (Token ID) for this. pkg/jwt needs to ensure JTI is set.
	// Standard claims usually have ID. Let's assume GenerateTokenPair adds it.
	// If not, we might need to rely on just claims, but we should verify DB.
	if claims.ID == "" {
		// Fallback: If no JTI, we can't strict verify. But we should enforce JTI usage.
		// For now, let's assume JTI is present.
	}

	session, err := h.repo.GetRefreshToken(c.Request.Context(), claims.ID)
	if err != nil {
		response.Unauthorized(c, "Token lookup failed", "INVALID_TOKEN")
		return
	}
	if session == nil {
		response.Unauthorized(c, "Token revoked or not found", "TOKEN_REVOKED")
		return
	}
	if session.Revoked {
		response.Unauthorized(c, "Token has been revoked", "TOKEN_REVOKED")
		return
	}

	// Generate NEW Access Token
	jwtConfig := h.getJWTConfig()
	newAccessToken, err := idToken.GenerateToken(claims.UserID, claims.Email, jwtConfig)
	if err != nil {
		response.InternalServerError(c, "Failed to generate token", "INTERNAL_ERROR")
		return
	}

	// Return new access token (and keep same refresh token)
	response.Success(c, AuthResponse{
		AccessToken:  newAccessToken,
		RefreshToken: req.RefreshToken,
		// User is optional here, but client might want it?
		// Let's optimize and not fetch full user unless needed.
		// Struct has User *User, so it can be nil.
	})
}

// Logout Revokes the current refresh token
// @Summary Logout
// @Description Revoke the refresh token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body RefreshTokenRequest true "Refresh token to revoke"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Router /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	// We don't necessarily need to validate signature to revoke it, but it's safer.
	claims, err := idToken.GetTokenClaims(req.RefreshToken)
	if err != nil {
		response.BadRequest(c, "Invalid token format", "INVALID_TOKEN")
		return
	}

	if claims.ID != "" {
		_ = h.repo.RevokeRefreshToken(c.Request.Context(), claims.ID)
	}

	response.Success(c, "Logged out successfully")
}

// RevokeAllTokens Revokes all tokens for the user
// @Summary Revoke all tokens
// @Description Revoke all refresh tokens for the current user
// @Tags auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /auth/revoke-all [post]
func (h *Handler) RevokeAllTokens(c *gin.Context) {
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

	if err := h.repo.RevokeAllUserTokens(c.Request.Context(), user.ID); err != nil {
		response.InternalServerError(c, "Failed to revoke tokens", "DATABASE_ERROR")
		return
	}

	response.Success(c, "All sessions revoked")
}

// GetOwnProfile returns the authenticated user's private profile
// @Summary Get own profile
// @Description Get the authenticated user's complete profile
// @Tags users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=OwnProfileResponse}
// @Failure 401 {object} response.APIResponse
// @Router /users/me [get]
func (h *Handler) GetOwnProfile(c *gin.Context) {
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

	resp := OwnProfileResponse{
		ID:                user.ID,
		GoogleID:          user.GoogleID,
		Email:             user.Email,
		Username:          user.Username,
		DisplayName:       user.DisplayName,
		Bio:               user.Bio,
		ProfilePictureURL: user.ProfilePictureURL,
		CoverImageURL:     user.CoverImageURL,
		FollowerCount:     user.FollowerCount,
		FollowingCount:    user.FollowingCount,
		AnchorCount:       user.AnchorCount,
		IsVerified:        user.IsVerified,
		JoinedAt:          user.JoinedAt,
		CreatedAt:         user.CreatedAt,
		UpdatedAt:         user.UpdatedAt,
	}

	response.Success(c, resp)
}

// GetPublicProfile returns a user's public profile
// @Summary Get public profile
// @Description Get a user's public profile by ID
// @Tags users
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} response.APIResponse{data=PublicProfileResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id} [get]
func (h *Handler) GetPublicProfile(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}

	user, err := h.repo.GetUserByObjectID(c.Request.Context(), userID)
	if err != nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	// Check follow status if authenticated
	var isFollowing, isFollowedBy, isMutual bool
	if val, exists := c.Get("user"); exists {
		if currentUser, ok := val.(*User); ok {
			// Don't check follow status for self
			if currentUser.ID != userID && h.followService != nil {
				isFollowing, isFollowedBy, err = h.followService.GetStatus(c.Request.Context(), currentUser.ID, userID)
				if err == nil {
					isMutual = isFollowing && isFollowedBy
				}
			}
		}
	}

	resp := PublicProfileResponse{
		ID:                user.ID,
		Username:          user.Username,
		DisplayName:       user.DisplayName,
		Bio:               user.Bio,
		ProfilePictureURL: user.ProfilePictureURL,
		CoverImageURL:     user.CoverImageURL,
		FollowerCount:     user.FollowerCount,
		FollowingCount:    user.FollowingCount,
		AnchorCount:       user.AnchorCount,
		IsVerified:        user.IsVerified,
		JoinedAt:          user.JoinedAt,
		IsFollowing:       isFollowing,
		IsFollowedBy:      isFollowedBy,
		IsMutual:          isMutual,
	}

	response.Success(c, resp)
}

// UpdateProfile updates the user's profile
// @Summary Update user profile
// @Description Update display name and/or bio
// @Tags users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body UpdateProfileRequest true "Profile updates"
// @Success 200 {object} response.APIResponse{data=OwnProfileResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /users/me [patch]
func (h *Handler) UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	if err := ValidateUpdateProfileRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
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

	if req.DisplayName != nil {
		updates["displayName"] = *req.DisplayName
	}

	if req.Bio != nil {
		updates["bio"] = *req.Bio
	}

	if len(updates) == 0 {
		// Just return current profile if no changes
		h.GetOwnProfile(c)
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

	resp := OwnProfileResponse{
		ID:                updatedUser.ID,
		GoogleID:          updatedUser.GoogleID,
		Email:             updatedUser.Email,
		Username:          updatedUser.Username,
		DisplayName:       updatedUser.DisplayName,
		Bio:               updatedUser.Bio,
		ProfilePictureURL: updatedUser.ProfilePictureURL,
		CoverImageURL:     updatedUser.CoverImageURL,
		FollowerCount:     updatedUser.FollowerCount,
		FollowingCount:    updatedUser.FollowingCount,
		AnchorCount:       updatedUser.AnchorCount,
		IsVerified:        updatedUser.IsVerified,
		JoinedAt:          updatedUser.JoinedAt,
		CreatedAt:         updatedUser.CreatedAt,
		UpdatedAt:         updatedUser.UpdatedAt,
	}

	response.Success(c, resp)
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

// UploadProfilePicture uploads a new profile picture
// @Summary Upload profile picture
// @Description Upload a new profile picture (max 5MB)
// @Tags users
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param file formData file true "Profile picture file"
// @Success 200 {object} response.APIResponse{data=ProfilePictureResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /users/me/profile-picture [post]
func (h *Handler) UploadProfilePicture(c *gin.Context) {
	// Get user
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

	// Get file
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "File is required", "MISSING_FILE")
		return
	}

	// Validate file
	if err := ValidateProfilePicture(file); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_FILE")
		return
	}

	// Open file
	fileContent, err := file.Open()
	if err != nil {
		response.InternalServerError(c, "Failed to open file", "FILE_ERROR")
		return
	}
	defer fileContent.Close()

	// Delete old picture if exists
	if user.ProfilePicturePublicID != "" {
		_ = h.cloudinary.Delete(c.Request.Context(), user.ProfilePicturePublicID, "image")
	}

	// Upload new picture
	uploadResult, err := h.cloudinary.UploadImage(c.Request.Context(), fileContent, file.Filename)
	if err != nil {
		response.InternalServerError(c, "Failed to upload image", "UPLOAD_FAILED")
		return
	}

	// Update user
	updates := map[string]interface{}{
		"profilePictureUrl":      uploadResult.URL,
		"profilePicturePublicId": uploadResult.PublicID,
	}

	if err := h.repo.UpdateUser(c.Request.Context(), user.ID.Hex(), updates); err != nil {
		response.BadRequest(c, "Failed to update user profile", "DATABASE_ERROR")
		return
	}

	response.Success(c, ProfilePictureResponse{
		ProfilePictureURL: uploadResult.URL,
	})
}

// UploadCoverImage uploads a new cover image
// @Summary Upload cover image
// @Description Upload a new cover image (max 10MB)
// @Tags users
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param file formData file true "Cover image file"
// @Success 200 {object} response.APIResponse{data=CoverImageResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /users/me/cover-image [post]
func (h *Handler) UploadCoverImage(c *gin.Context) {
	// Get user
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

	// Get file
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "File is required", "MISSING_FILE")
		return
	}

	// Validate file
	if err := ValidateCoverImage(file); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_FILE")
		return
	}

	// Open file
	fileContent, err := file.Open()
	if err != nil {
		response.InternalServerError(c, "Failed to open file", "FILE_ERROR")
		return
	}
	defer fileContent.Close()

	// Delete old cover if exists
	if user.CoverImagePublicID != "" {
		_ = h.cloudinary.Delete(c.Request.Context(), user.CoverImagePublicID, "image")
	}

	// Upload new cover
	uploadResult, err := h.cloudinary.UploadImage(c.Request.Context(), fileContent, file.Filename)
	if err != nil {
		response.InternalServerError(c, "Failed to upload image", "UPLOAD_FAILED")
		return
	}

	// Update user
	updates := map[string]interface{}{
		"coverImageUrl":      uploadResult.URL,
		"coverImagePublicId": uploadResult.PublicID,
	}

	if err := h.repo.UpdateUser(c.Request.Context(), user.ID.Hex(), updates); err != nil {
		response.BadRequest(c, "Failed to update user profile", "DATABASE_ERROR")
		return
	}

	response.Success(c, CoverImageResponse{
		CoverImageURL: uploadResult.URL,
	})
}

// RemoveProfilePicture removes the user's profile picture
// @Summary Remove profile picture
// @Description Remove the current user's profile picture
// @Tags users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /users/me/profile-picture [delete]
func (h *Handler) RemoveProfilePicture(c *gin.Context) {
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

	if user.ProfilePicturePublicID != "" {
		_ = h.cloudinary.Delete(c.Request.Context(), user.ProfilePicturePublicID, "image")
	}

	updates := map[string]interface{}{
		"profilePictureUrl":      "",
		"profilePicturePublicId": "",
	}

	if err := h.repo.UpdateUser(c.Request.Context(), user.ID.Hex(), updates); err != nil {
		response.BadRequest(c, "Failed to update user profile", "DATABASE_ERROR")
		return
	}

	response.Success(c, "Profile picture removed")
}

// RemoveCoverImage removes the user's cover image
// @Summary Remove cover image
// @Description Remove the current user's cover image
// @Tags users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /users/me/cover-image [delete]
func (h *Handler) RemoveCoverImage(c *gin.Context) {
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

	if user.CoverImagePublicID != "" {
		_ = h.cloudinary.Delete(c.Request.Context(), user.CoverImagePublicID, "image")
	}

	updates := map[string]interface{}{
		"coverImageUrl":      "",
		"coverImagePublicId": "",
	}

	if err := h.repo.UpdateUser(c.Request.Context(), user.ID.Hex(), updates); err != nil {
		response.BadRequest(c, "Failed to update user profile", "DATABASE_ERROR")
		return
	}

	response.Success(c, "Cover image removed")
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

// GetPinnedAnchors returns the pinned anchors for a user
// @Summary Get pinned anchors
// @Description Get pinned anchors for a specific user
// @Tags users
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} response.APIResponse{data=[]PinnedAnchorResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/pinned [get]
func (h *Handler) GetPinnedAnchors(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}

	// Determine visibility access and user existence
	_, err = h.repo.GetUserByObjectID(c.Request.Context(), userID)
	if err != nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	includePrivate := false
	if val, exists := c.Get("user"); exists {
		if currentUser, ok := val.(*User); ok {
			if currentUser.ID == userID {
				includePrivate = true
			}
		}
	}

	if h.anchorService == nil {
		// Should not happen if wired correctly, but fail gracefully
		response.InternalServerError(c, "Anchor service not available", "INTERNAL_ERROR")
		return
	}

	// Fetch pinned anchors
	anchorsList, err := h.anchorService.GetPinnedAnchors(c.Request.Context(), userID, includePrivate)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch pinned anchors", "DATABASE_ERROR")
		return
	}

	// Convert to response
	var pinnedAnchors []PinnedAnchorResponse
	for _, a := range anchorsList {
		pinnedAnchors = append(pinnedAnchors, PinnedAnchorResponse{
			ID:              a.ID,
			Title:           a.Title,
			Description:     a.Description,
			CoverMediaType:  a.CoverMediaType,
			CoverMediaValue: a.CoverMediaValue,
			Visibility:      a.Visibility,
			ItemCount:       a.ItemCount,
			LikeCount:       a.LikeCount,
			CloneCount:      a.CloneCount,
			CreatedAt:       a.CreatedAt,
		})
	}

	response.Success(c, pinnedAnchors)
}

// DeleteAccount deletes the user's account and all associated data
// @Summary Delete account
// @Description Permanently delete the authenticated user's account and all content
// @Tags users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 500 {object} response.APIResponse
// @Router /users/me [delete]
func (h *Handler) DeleteAccount(c *gin.Context) {
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

	// 1. Delete all user content (Anchors, Items, Assets) via AnchorService
	if h.anchorService != nil {
		if err := h.anchorService.DeleteAllByUser(c.Request.Context(), user.ID); err != nil {
			fmt.Printf("Failed to delete user content: %v\n", err)
			response.InternalServerError(c, "Failed to delete user content", "DELETE_FAILED")
			return
		}
	}

	// 2. Delete Profile Picture and Cover Image from Cloudinary
	if user.ProfilePicturePublicID != "" {
		_ = h.cloudinary.Delete(c.Request.Context(), user.ProfilePicturePublicID, "image")
	}
	if user.CoverImagePublicID != "" {
		_ = h.cloudinary.Delete(c.Request.Context(), user.CoverImagePublicID, "image")
	}

	// 3. Delete User from DB
	if err := h.repo.DeleteUser(c.Request.Context(), user.ID); err != nil {
		response.InternalServerError(c, "Failed to delete user", "DATABASE_ERROR")
		return
	}

	response.Success(c, "Account deleted successfully")
}
