package auth

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	idToken "github.com/xyz-asif/gotodo/internal/pkg/jwt"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

// authError describes why a request could not be authenticated
type authError struct {
	status  int
	message string
	code    string
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
// The scheme is case-insensitive (RFC 7235) and extra whitespace is tolerated.
func bearerToken(header string) (string, bool) {
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return "", false
	}
	return fields[1], true
}

// authenticate validates the bearer token and loads the user. It is shared by the required and
// optional middlewares so both apply exactly the same rules.
func authenticate(c *gin.Context, repo *Repository, cfg *config.Config) (*User, *authError) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return nil, &authError{http.StatusUnauthorized, "Authorization header required", "AUTH_REQUIRED"}
	}
	tokenString, ok := bearerToken(header)
	if !ok {
		return nil, &authError{http.StatusUnauthorized, "Invalid authorization format", "INVALID_AUTH_FORMAT"}
	}

	claims, err := idToken.ValidateAccessToken(tokenString, cfg.JWTSecret)
	if err != nil {
		return nil, &authError{http.StatusUnauthorized, "Invalid or expired token", "INVALID_TOKEN"}
	}

	ctx := c.Request.Context()
	user, err := repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) || claims.UserID == "" {
			return nil, &authError{http.StatusUnauthorized, "User not found", "USER_NOT_FOUND"}
		}
		// A database outage must not look like an invalid session, or clients would log users out
		log.Printf("auth: user lookup failed: %v", err)
		return nil, &authError{http.StatusServiceUnavailable, "Authentication temporarily unavailable", "AUTH_UNAVAILABLE"}
	}

	if user.TokenRevoked(claims.IssuedTime()) {
		return nil, &authError{http.StatusUnauthorized, "Session has been revoked", "TOKEN_REVOKED"}
	}

	// Tokens bound to a session die with it (logout of this device). Tokens issued before
	// session binding existed carry no sid and are only limited by expiry and revoke-all.
	if claims.SessionID != "" {
		active, err := repo.SessionActive(ctx, claims.SessionID)
		if err != nil {
			log.Printf("auth: session lookup failed: %v", err)
			return nil, &authError{http.StatusServiceUnavailable, "Authentication temporarily unavailable", "AUTH_UNAVAILABLE"}
		}
		if !active {
			return nil, &authError{http.StatusUnauthorized, "Session has ended", "TOKEN_REVOKED"}
		}
	}

	// The login session this request belongs to (e.g. push tokens are tied to it)
	c.Set(SessionIDKey, claims.SessionID)
	return user, nil
}

// SessionIDKey is the context key holding the current request's login session ID ("" for
// tokens issued before sessions were bound)
const SessionIDKey = "sessionId"

// NewAuthMiddleware creates a Gin middleware that requires a valid access token
func NewAuthMiddleware(repo *Repository, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, aerr := authenticate(c, repo, cfg)
		if aerr != nil {
			response.Error(c, aerr.status, aerr.message, aerr.code)
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

// NewOptionalAuthMiddleware sets "user" in the context when a valid access token is
// present, and otherwise continues without authentication (never aborts).
func NewOptionalAuthMiddleware(repo *Repository, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") != "" {
			if user, aerr := authenticate(c, repo, cfg); aerr == nil {
				c.Set("user", user)
			}
		}
		c.Next()
	}
}
