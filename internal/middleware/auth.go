package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
)

// NewAuthMiddleware requires a valid access token. It delegates to the auth feature so every
// route group enforces the same rules (revocation, session binding, error codes).
func NewAuthMiddleware(repo *auth.Repository, cfg *config.Config) gin.HandlerFunc {
	return auth.NewAuthMiddleware(repo, cfg)
}

// OptionalAuthMiddleware sets "user" when a valid token is present and otherwise continues
// without authentication (never aborts).
func OptionalAuthMiddleware(repo *auth.Repository, cfg *config.Config) gin.HandlerFunc {
	return auth.NewOptionalAuthMiddleware(repo, cfg)
}
