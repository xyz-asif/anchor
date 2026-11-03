package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token types stored in the "typ" claim
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// ErrWrongTokenType is returned when e.g. a refresh token is presented where an access token is expected
var ErrWrongTokenType = errors.New("wrong token type")

// Claims represents JWT claims
type Claims struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Type   string `json:"typ,omitempty"` // "access" or "refresh"
	// IssuedAtMs is the issue time in milliseconds. The standard "iat" claim only has second
	// precision, which isn't enough to tell whether a token predates a revoke-all.
	IssuedAtMs int64                  `json:"iat_ms,omitempty"`
	Role       string                 `json:"role,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	jwt.RegisteredClaims
}

// Config represents JWT configuration
type Config struct {
	Secret        string
	AccessExpiry  time.Duration
	RefreshExpiry time.Duration
	Issuer        string
	Audience      string
	SigningMethod jwt.SigningMethod
}

// DefaultConfig returns default JWT configuration
func DefaultConfig(secret string) *Config {
	return &Config{
		Secret:        secret,
		AccessExpiry:  1 * time.Hour,
		RefreshExpiry: 7 * 24 * time.Hour, // 7 days
		Issuer:        "gotodo-api",
		Audience:      "gotodo-users",
		SigningMethod: jwt.SigningMethodHS256,
	}
}

// GenerateToken generates a new JWT token
func GenerateToken(userID, email string, cfg *Config) (string, error) {
	if cfg == nil {
		return "", errors.New("JWT config is required")
	}

	now := time.Now()
	claims := &Claims{
		UserID:     userID,
		Email:      email,
		Type:       TokenTypeAccess,
		IssuedAtMs: now.UnixMilli(),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        newTokenID(),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.AccessExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Audience:  []string{cfg.Audience},
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(cfg.SigningMethod, claims)
	return token.SignedString([]byte(cfg.Secret))
}

// GenerateTokenWithRole generates a JWT token with role information
func GenerateTokenWithRole(userID, email, role string, cfg *Config) (string, error) {
	if cfg == nil {
		return "", errors.New("JWT config is required")
	}

	now := time.Now()
	claims := &Claims{
		UserID:     userID,
		Email:      email,
		Role:       role,
		Type:       TokenTypeAccess,
		IssuedAtMs: now.UnixMilli(),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        newTokenID(),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.AccessExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Audience:  []string{cfg.Audience},
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(cfg.SigningMethod, claims)
	return token.SignedString([]byte(cfg.Secret))
}

// GenerateTokenWithMetadata generates a JWT token with custom metadata
func GenerateTokenWithMetadata(userID, email string, metadata map[string]interface{}, cfg *Config) (string, error) {
	if cfg == nil {
		return "", errors.New("JWT config is required")
	}

	now := time.Now()
	claims := &Claims{
		UserID:     userID,
		Email:      email,
		Metadata:   metadata,
		Type:       TokenTypeAccess,
		IssuedAtMs: now.UnixMilli(),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        newTokenID(),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.AccessExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Audience:  []string{cfg.Audience},
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(cfg.SigningMethod, claims)
	return token.SignedString([]byte(cfg.Secret))
}

// GenerateRefreshToken generates a refresh token
func GenerateRefreshToken(userID, email string, cfg *Config) (string, error) {
	if cfg == nil {
		return "", errors.New("JWT config is required")
	}

	now := time.Now()
	claims := &Claims{
		UserID:     userID,
		Email:      email,
		Type:       TokenTypeRefresh,
		IssuedAtMs: now.UnixMilli(),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        newTokenID(),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.RefreshExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    cfg.Issuer,
			Audience:  []string{cfg.Audience},
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(cfg.SigningMethod, claims)
	return token.SignedString([]byte(cfg.Secret))
}

// ValidateToken validates and parses a JWT token
func ValidateToken(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

// RefreshToken refreshes an existing token
func RefreshToken(tokenString, secret string, cfg *Config) (string, error) {
	claims, err := ValidateRefreshToken(tokenString, secret)
	if err != nil {
		return "", err
	}

	// Generate new token with extended expiry
	return GenerateToken(claims.UserID, claims.Email, cfg)
}

// GetTokenExpiry returns the expiry time of a token
func GetTokenExpiry(tokenString, secret string) (time.Time, error) {
	claims, err := ValidateToken(tokenString, secret)
	if err != nil {
		return time.Time{}, err
	}

	return claims.ExpiresAt.Time, nil
}

// IsTokenExpired checks if a token is expired
func IsTokenExpired(tokenString, secret string) (bool, error) {
	expiry, err := GetTokenExpiry(tokenString, secret)
	if err != nil {
		return true, err
	}

	return time.Now().After(expiry), nil
}

// GetTokenClaims returns all claims from a token without validation
func GetTokenClaims(tokenString string) (*Claims, error) {
	parser := jwt.Parser{}
	token, _, err := parser.ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claims format")
	}

	return claims, nil
}

// GenerateTokenPair generates both access and refresh tokens
func GenerateTokenPair(userID, email string, cfg *Config) (accessToken, refreshToken string, err error) {
	accessToken, err = GenerateToken(userID, email, cfg)
	if err != nil {
		return "", "", err
	}

	refreshToken, err = GenerateRefreshToken(userID, email, cfg)
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

// ValidateTokenWithRole validates token and checks if user has required role
func ValidateTokenWithRole(tokenString, secret, requiredRole string) (*Claims, error) {
	claims, err := ValidateToken(tokenString, secret)
	if err != nil {
		return nil, err
	}

	if requiredRole != "" && claims.Role != requiredRole {
		return nil, errors.New("insufficient permissions")
	}

	return claims, nil
}

// newTokenID returns a random, URL-safe unique token ID (used as the JWT "jti" claim)
func newTokenID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand never fails on supported platforms; fall back to time-based uniqueness
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// ValidateAccessToken validates a token and ensures it is an access token
func ValidateAccessToken(tokenString, secret string) (*Claims, error) {
	claims, err := ValidateToken(tokenString, secret)
	if err != nil {
		return nil, err
	}
	if claims.Type != TokenTypeAccess {
		return nil, ErrWrongTokenType
	}
	return claims, nil
}

// ValidateRefreshToken validates a token and ensures it is a refresh token with a token ID
func ValidateRefreshToken(tokenString, secret string) (*Claims, error) {
	claims, err := ValidateToken(tokenString, secret)
	if err != nil {
		return nil, err
	}
	if claims.Type != TokenTypeRefresh || claims.ID == "" {
		return nil, ErrWrongTokenType
	}
	return claims, nil
}

// IssuedTime returns when the token was issued, with millisecond precision when available
func (c *Claims) IssuedTime() time.Time {
	if c.IssuedAtMs > 0 {
		return time.UnixMilli(c.IssuedAtMs)
	}
	if c.IssuedAt != nil {
		return c.IssuedAt.Time
	}
	return time.Time{}
}
