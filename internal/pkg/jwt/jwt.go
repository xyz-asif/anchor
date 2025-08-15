package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents JWT claims
type Claims struct {
	UserID   string                 `json:"userId"`
	Email    string                 `json:"email"`
	Role     string                 `json:"role,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
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
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
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
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
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
		UserID:   userID,
		Email:    email,
		Metadata: metadata,
		RegisteredClaims: jwt.RegisteredClaims{
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
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
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
	claims, err := ValidateToken(tokenString, secret)
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
