package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	idToken "github.com/xyz-asif/gotodo/internal/pkg/jwt"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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

type Handler struct {
	repo          *Repository
	config        *config.Config
	cloudinary    *cloudinary.Service
	followService FollowService
	anchorService AnchorService
}

func NewHandler(repo *Repository, cfg *config.Config, cld *cloudinary.Service, followService FollowService, anchorService AnchorService) *Handler {
	return &Handler{
		repo:          repo,
		config:        cfg,
		cloudinary:    cld,
		followService: followService,
		anchorService: anchorService,
	}
}

// CheckUsernameAvailability godoc
// @Summary Check if username is available
// @Description Check if a username is available for registration
// @Tags auth
// @Produce json
// @Param username query string true "Username to check (3-20 chars)"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Router /auth/username/check [get]
func (h *Handler) CheckUsernameAvailability(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		response.BadRequest(c, "Username query parameter is required", "MISSING_USERNAME")
		return
	}

	// Normalize
	username = strings.ToLower(strings.TrimSpace(username))

	// Same rules as PATCH /auth/username (length, characters, reserved names)
	if err := ValidateUsername(username); err != nil {
		response.Success(c, gin.H{
			"username":  username,
			"available": false,
			"reason":    err.Error(),
		})
		return
	}

	ctx := c.Request.Context()

	// Check if exists (a DB failure must not be reported as "available")
	exists, err := h.repo.UsernameExists(ctx, username)
	if err != nil {
		response.InternalServerError(c, "Failed to check username", "DATABASE_ERROR")
		return
	}
	available := !exists

	resp := gin.H{
		"username":  username,
		"available": available,
	}
	if !available {
		resp["reason"] = "Username is already taken"
	}

	response.Success(c, resp)
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

// issueTokens creates an access/refresh token pair for the user and stores the refresh
// session. On failure it writes the error response and returns ok=false.
func (h *Handler) issueTokens(c *gin.Context, user *User) (accessToken, refreshToken string, ok bool) {
	jwtConfig := h.getJWTConfig()
	refreshToken, err := idToken.GenerateRefreshToken(user.ID.Hex(), user.Email, jwtConfig)
	if err != nil {
		response.InternalServerError(c, "Failed to generate tokens", "TOKEN_FAILED")
		return "", "", false
	}

	claims, err := idToken.ValidateRefreshToken(refreshToken, h.config.JWTSecret)
	if err != nil {
		response.InternalServerError(c, "Failed to generate tokens", "TOKEN_FAILED")
		return "", "", false
	}

	session := &RefreshTokenSession{
		ID:        primitive.NewObjectID(),
		UserID:    user.ID,
		TokenID:   claims.ID,
		ExpiresAt: claims.ExpiresAt.Time,
		CreatedAt: time.Now(),
		UserAgent: c.Request.UserAgent(),
		IPAddress: c.ClientIP(),
	}
	if err := h.repo.SaveRefreshToken(c.Request.Context(), session); err != nil {
		log.Printf("Failed to save refresh token session: %v", err)
		response.InternalServerError(c, "Failed to create session", "SESSION_FAILED")
		return "", "", false
	}

	// The access token is bound to this session, so logging out of it ends the access token too
	accessToken, err = idToken.GenerateSessionAccessToken(user.ID.Hex(), user.Email, claims.ID, jwtConfig)
	if err != nil {
		_ = h.repo.RevokeRefreshToken(c.Request.Context(), claims.ID)
		response.InternalServerError(c, "Failed to generate tokens", "TOKEN_FAILED")
		return "", "", false
	}

	return accessToken, refreshToken, true
}

// maxSequentialUsernameTries bounds how many "name1, name2, ..." candidates are checked before
// switching to a random suffix, so a very common name can't trigger thousands of DB queries.
const maxSequentialUsernameTries = 5

// uniqueUsername returns base, or base with a suffix that makes it unique. Every result passes
// ValidateUsername (length, characters, reserved names).
func (h *Handler) uniqueUsername(ctx context.Context, base string) (string, error) {
	base = usernameBase(base)
	for attempt := 0; attempt < maxSequentialUsernameTries+5; attempt++ {
		var candidate string
		switch {
		case attempt == 0:
			candidate = base
		case attempt <= maxSequentialUsernameTries:
			candidate = withUsernameSuffix(base, fmt.Sprintf("%d", attempt))
		default:
			candidate = withUsernameSuffix(base, randomDigits(6))
		}
		if ValidateUsername(candidate) != nil {
			continue
		}
		exists, err := h.repo.UsernameExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", errors.New("could not generate a unique username")
}

// cleanDisplayName trims a display name taken from an external source (Google, dev login) and
// cuts it to the 50-character limit; it returns fallback when too little is left.
func cleanDisplayName(name, fallback string) string {
	name = strings.TrimSpace(stripControlChars(name))
	if r := []rune(name); len(r) > 50 {
		name = strings.TrimSpace(string(r[:50]))
	}
	if ValidateDisplayName(name) != nil {
		return fallback
	}
	return name
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
	googleUser, err := VerifyGoogleToken(c.Request.Context(), req.GoogleIDToken, h.config.GoogleClientIDs)
	if errors.Is(err, ErrGoogleLoginNotConfigured) {
		log.Printf("Google login attempted but GOOGLE_CLIENT_ID is not configured")
		response.ServiceUnavailable(c, "Google login is not configured", "GOOGLE_LOGIN_UNAVAILABLE")
		return
	}
	if err != nil {
		log.Printf("Google token verification failed: %v", err)
		response.Unauthorized(c, "Invalid Google token", "INVALID_TOKEN")
		return
	}

	// Only trust the email (for account creation and linking) if Google verified it
	if !googleUser.EmailVerified || googleUser.Email == "" {
		response.Unauthorized(c, "Google account email is not verified", "EMAIL_NOT_VERIFIED")
		return
	}

	// Normalize email to ensure consistency
	googleUser.Email = strings.ToLower(googleUser.Email)

	user, isNewUser, aerr := h.findOrCreateGoogleUser(c.Request.Context(), googleUser)
	if aerr != nil {
		response.Error(c, aerr.status, aerr.message, aerr.code)
		return
	}

	accessToken, refreshToken, ok := h.issueTokens(c, user)
	if !ok {
		return
	}

	stcode := 200
	if isNewUser {
		stcode = 201
	}

	response.Respond(c, stcode, true, "Login successful", AuthResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		IsNewUser:    isNewUser,
	})
}

// findOrCreateGoogleUser returns the account for a verified Google user: by Google ID, else by
// email (linking the Google ID), else a newly created account.
func (h *Handler) findOrCreateGoogleUser(ctx context.Context, googleUser *GoogleUser) (*User, bool, *authError) {
	dbErr := &authError{http.StatusInternalServerError, "Database error", "DATABASE_ERROR"}

	// Retried when a concurrent login for the same person (double tap, two devices) creates
	// the account between our lookup and our insert, or takes the generated username.
	for attempt := 0; attempt < 3; attempt++ {
		user, err := h.repo.GetUserByGoogleID(ctx, googleUser.UID)
		if err != nil {
			return nil, false, dbErr
		}
		if user != nil {
			return user, false, nil
		}

		userByEmail, err := h.repo.GetUserByEmail(ctx, googleUser.Email)
		if err != nil {
			return nil, false, dbErr
		}
		if userByEmail != nil {
			// A concurrent first login for the same person created the account between our two
			// lookups: it's already linked to this Google account
			if userByEmail.GoogleID == googleUser.UID {
				return userByEmail, false, nil
			}
			// A Google account's subject ID never changes, so an account already linked to a
			// *different* Google account must not be taken over just because the email matches
			// (e.g. a recycled Workspace address). Dev-login placeholders may be linked.
			if userByEmail.GoogleID != "" && !strings.HasPrefix(userByEmail.GoogleID, "dev_") {
				log.Printf("Google login: email %s is linked to another Google account", googleUser.Email)
				return nil, false, &authError{http.StatusConflict,
					"This email is already linked to a different Google account", "ACCOUNT_CONFLICT"}
			}
			if err := h.repo.UpdateUser(ctx, userByEmail.ID.Hex(), map[string]interface{}{"googleId": googleUser.UID}); err != nil {
				log.Printf("Failed to link Google ID: %v", err)
				return nil, false, &authError{http.StatusInternalServerError, "Failed to link account", "DATABASE_ERROR"}
			}
			userByEmail.GoogleID = googleUser.UID
			return userByEmail, false, nil
		}

		username, err := h.uniqueUsername(ctx, GenerateUniqueUsername(googleUser.Name))
		if err != nil {
			return nil, false, dbErr
		}
		now := time.Now()
		user = &User{
			GoogleID:          googleUser.UID,
			Email:             googleUser.Email,
			Username:          username,
			DisplayName:       cleanDisplayName(googleUser.Name, username),
			ProfilePictureURL: googleUser.Picture,
			JoinedAt:          now,
		}
		err = h.repo.CreateUser(ctx, user)
		if err == nil {
			return user, true, nil
		}
		if !mongo.IsDuplicateKeyError(err) {
			log.Printf("CreateUser failed: %v", err)
			return nil, false, &authError{http.StatusInternalServerError, "Failed to create user", "DATABASE_ERROR"}
		}
		// Duplicate key: loop to pick up the concurrently created account or a new username
	}
	return nil, false, &authError{http.StatusConflict, "Could not create account, please retry", "ACCOUNT_CREATE_CONFLICT"}
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
	// Dev login is never available in production, even if DEV_MODE is set
	if !h.config.DevMode || h.config.IsProduction() {
		response.Forbidden(c, "Dev login is disabled", "DEV_LOGIN_DISABLED")
		return
	}

	var req DevLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_REQUEST")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	ctx := c.Request.Context()

	// Try to find existing user by email
	user, err := h.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		response.InternalServerError(c, "Database error", "DATABASE_ERROR")
		return
	}
	if user == nil {
		// Create new dev user
		now := time.Now()
		user = &User{
			ID:          primitive.NewObjectID(),
			GoogleID:    "dev_" + primitive.NewObjectID().Hex(),
			Email:       req.Email,
			Username:    "", // set below
			DisplayName: cleanDisplayName(req.DisplayName, "Dev User"),
			JoinedAt:    now,
		}

		// "dev_" prefix keeps RequiresUsername=true so the client still runs username setup.
		// The email's local part plus a uniqueness check avoids collisions between dev users.
		localPart := strings.SplitN(req.Email, "@", 2)[0]
		username, err := h.uniqueUsername(ctx, "dev_"+GenerateUniqueUsername(localPart))
		if err != nil {
			response.InternalServerError(c, "Database error", "DATABASE_ERROR")
			return
		}
		user.Username = username

		if err := h.repo.CreateUser(ctx, user); err != nil {
			// A concurrent dev login for the same email may have created the account first
			existing, lookupErr := h.repo.GetUserByEmail(ctx, req.Email)
			if !mongo.IsDuplicateKeyError(err) || lookupErr != nil || existing == nil {
				log.Printf("Dev login CreateUser failed: %v", err)
				response.InternalServerError(c, "Failed to create user", "CREATE_FAILED")
				return
			}
			user = existing
		}
	}

	accessToken, refreshToken, ok := h.issueTokens(c, user)
	if !ok {
		return
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

	// Must be a validly signed, unexpired *refresh* token with a token ID
	claims, err := idToken.ValidateRefreshToken(req.RefreshToken, h.config.JWTSecret)
	if err != nil {
		response.Unauthorized(c, "Invalid refresh token", "INVALID_TOKEN")
		return
	}

	// The session must still exist (logout / revoke-all / account deletion remove it)
	session, err := h.repo.GetRefreshToken(c.Request.Context(), claims.ID)
	if err != nil {
		response.InternalServerError(c, "Token lookup failed", "DATABASE_ERROR")
		return
	}
	if session == nil || session.Revoked || session.UserID.Hex() != claims.UserID {
		response.Unauthorized(c, "Token revoked or not found", "TOKEN_REVOKED")
		return
	}

	// Generate a new access token bound to the same session
	jwtConfig := h.getJWTConfig()
	newAccessToken, err := idToken.GenerateSessionAccessToken(claims.UserID, claims.Email, claims.ID, jwtConfig)
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

	claims, err := idToken.ValidateRefreshToken(req.RefreshToken, h.config.JWTSecret)
	if err != nil {
		// Expired or malformed tokens have nothing left to revoke; logging out is still "successful"
		response.Success(c, "Logged out successfully")
		return
	}

	if err := h.repo.RevokeRefreshToken(c.Request.Context(), claims.ID); err != nil {
		response.InternalServerError(c, "Failed to log out", "DATABASE_ERROR")
		return
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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
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

	user, ok := h.loadVisibleUser(c, userID)
	if !ok {
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

	// Store exactly what was validated: trimmed, without invisible/control characters
	// (validation used to trim but the untrimmed value was saved)
	if req.DisplayName != nil {
		v := strings.TrimSpace(stripControlChars(*req.DisplayName))
		req.DisplayName = &v
	}
	if req.Bio != nil {
		v := strings.TrimSpace(stripControlCharsKeepNewlines(*req.Bio))
		req.Bio = &v
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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
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
		response.InternalServerError(c, "Failed to update profile", "DATABASE_ERROR")
		return
	}

	// Fetch updated user
	updatedUser, err := h.repo.GetUserByID(c.Request.Context(), user.ID.Hex())
	if err != nil {
		response.InternalServerError(c, "Failed to fetch updated user", "DATABASE_ERROR")
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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
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

	// Keeping the current name is not a change (and must not be reported as "taken" by yourself)
	if req.Username == user.Username {
		response.Success(c, user)
		return
	}

	usernameExists, err := h.repo.UsernameExists(c.Request.Context(), req.Username)
	if err != nil {
		response.InternalServerError(c, "Database error", "DATABASE_ERROR")
		return
	}
	if usernameExists {
		response.Conflict(c, "Username already taken", "USERNAME_TAKEN")
		return
	}

	// Atomic "only once" check: two concurrent requests from the same user can't both succeed
	if err := h.repo.SetUsernameOnce(c.Request.Context(), user.ID, req.Username); err != nil {
		if errors.Is(err, ErrUsernameAlreadyChanged) {
			response.BadRequest(c, "Username can only be changed once", "USERNAME_ALREADY_CHANGED")
			return
		}
		// Two users racing for the same name: the unique index rejects the second one
		if mongo.IsDuplicateKeyError(err) {
			response.Conflict(c, "Username already taken", "USERNAME_TAKEN")
			return
		}
		response.InternalServerError(c, "Failed to update username", "DATABASE_ERROR")
		return
	}

	// Fetch updated user
	updatedUser, err := h.repo.GetUserByID(c.Request.Context(), user.ID.Hex())
	if err != nil {
		response.InternalServerError(c, "Failed to fetch updated user", "DATABASE_ERROR")
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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	if h.cloudinary == nil {
		response.ServiceUnavailable(c, "Image uploads are not configured", "UPLOADS_UNAVAILABLE")
		return
	}

	// Reject oversized bodies before parsing them (images are at most 10MB)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 12<<20)

	// Get file
	file, err := c.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			response.Error(c, http.StatusRequestEntityTooLarge, "File is too large", "FILE_TOO_LARGE")
			return
		}
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

	// Upload new picture
	uploadResult, err := h.cloudinary.UploadImage(c.Request.Context(), fileContent, file.Filename)
	if err != nil {
		response.InternalServerError(c, "Failed to upload image", "UPLOAD_FAILED")
		return
	}

	// Point the profile at the new image and get back the image it replaced (read atomically,
	// so concurrent uploads never delete an image that is still in use or leave one orphaned)
	previousID, err := h.repo.SwapUserImage(c.Request.Context(), user.ID, "profilePictureUrl", "profilePicturePublicId", uploadResult.URL, uploadResult.PublicID)
	if err != nil {
		// Don't leave the just-uploaded image orphaned
		h.deleteAsset(c.Request.Context(), uploadResult.PublicID)
		response.InternalServerError(c, "Failed to update user profile", "DATABASE_ERROR")
		return
	}
	h.deleteAsset(c.Request.Context(), previousID)

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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	if h.cloudinary == nil {
		response.ServiceUnavailable(c, "Image uploads are not configured", "UPLOADS_UNAVAILABLE")
		return
	}

	// Reject oversized bodies before parsing them (images are at most 10MB)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 12<<20)

	// Get file
	file, err := c.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			response.Error(c, http.StatusRequestEntityTooLarge, "File is too large", "FILE_TOO_LARGE")
			return
		}
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

	// Upload new cover
	uploadResult, err := h.cloudinary.UploadImage(c.Request.Context(), fileContent, file.Filename)
	if err != nil {
		response.InternalServerError(c, "Failed to upload image", "UPLOAD_FAILED")
		return
	}

	// Point the profile at the new image and get back the image it replaced (read atomically,
	// so concurrent uploads never delete an image that is still in use or leave one orphaned)
	previousID, err := h.repo.SwapUserImage(c.Request.Context(), user.ID, "coverImageUrl", "coverImagePublicId", uploadResult.URL, uploadResult.PublicID)
	if err != nil {
		// Don't leave the just-uploaded image orphaned
		h.deleteAsset(c.Request.Context(), uploadResult.PublicID)
		response.InternalServerError(c, "Failed to update user profile", "DATABASE_ERROR")
		return
	}
	h.deleteAsset(c.Request.Context(), previousID)

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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// Clear the profile first and delete the file only afterwards, so a failed update never
	// leaves the profile pointing at a deleted image
	previousID, err := h.repo.SwapUserImage(c.Request.Context(), user.ID, "profilePictureUrl", "profilePicturePublicId", "", "")
	if err != nil {
		response.InternalServerError(c, "Failed to update user profile", "DATABASE_ERROR")
		return
	}
	h.deleteAsset(c.Request.Context(), previousID)

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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// Clear the profile first and delete the file only afterwards, so a failed update never
	// leaves the profile pointing at a deleted image
	previousID, err := h.repo.SwapUserImage(c.Request.Context(), user.ID, "coverImageUrl", "coverImagePublicId", "", "")
	if err != nil {
		response.InternalServerError(c, "Failed to update user profile", "DATABASE_ERROR")
		return
	}
	h.deleteAsset(c.Request.Context(), previousID)

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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
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
	if _, ok := h.loadVisibleUser(c, userID); !ok {
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
	pinnedAnchors := []PinnedAnchorResponse{} // [] rather than null when nothing is pinned
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
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// 1. Delete all user content (Anchors, Items, Assets) via AnchorService
	if h.anchorService != nil {
		if err := h.anchorService.DeleteAllByUser(c.Request.Context(), user.ID); err != nil {
			log.Printf("Failed to delete user content: %v", err)
			response.InternalServerError(c, "Failed to delete user content", "DELETE_FAILED")
			return
		}
	}

	// 2. Delete Profile Picture and Cover Image from Cloudinary
	if user.ProfilePicturePublicID != "" {
		h.deleteAsset(c.Request.Context(), user.ProfilePicturePublicID)
	}
	if user.CoverImagePublicID != "" {
		h.deleteAsset(c.Request.Context(), user.CoverImagePublicID)
	}

	// 3. Delete User from DB
	if err := h.repo.DeleteUser(c.Request.Context(), user.ID); err != nil {
		response.InternalServerError(c, "Failed to delete user", "DATABASE_ERROR")
		return
	}

	response.Success(c, "Account deleted successfully")
}

// loadVisibleUser loads a user's profile for the (optional) viewer. A user who has blocked the
// viewer is reported as not found, like elsewhere on social platforms. On failure it writes the
// response and returns ok=false.
func (h *Handler) loadVisibleUser(c *gin.Context, userID primitive.ObjectID) (*User, bool) {
	user, err := h.repo.GetUserByObjectID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response.NotFound(c, "User not found", "USER_NOT_FOUND")
		} else {
			response.InternalServerError(c, "Failed to load user", "DATABASE_ERROR")
		}
		return nil, false
	}
	if val, exists := c.Get("user"); exists {
		if viewer, ok := val.(*User); ok && !CanViewProfile(user, viewer) {
			response.NotFound(c, "User not found", "USER_NOT_FOUND")
			return nil, false
		}
	}
	return user, true
}

// deleteAsset removes an image from Cloudinary (best effort; no-op when Cloudinary isn't configured)
func (h *Handler) deleteAsset(ctx context.Context, publicID string) {
	if h.cloudinary == nil || publicID == "" {
		return
	}
	if err := h.cloudinary.Delete(ctx, publicID, "image"); err != nil {
		log.Printf("Failed to delete Cloudinary asset %s: %v", publicID, err)
	}
}
