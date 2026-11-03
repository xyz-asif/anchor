package auth

import (
	"context"
	"errors"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"github.com/xyz-asif/gotodo/internal/config"
)

// InitFirebase initializes the Firebase Admin SDK and returns the Auth client
func InitFirebase(cfg *config.Config) (*auth.Client, error) {
	opt := option.WithAuthCredentialsFile(option.ServiceAccount, cfg.FirebaseServiceAccountPath) // only accept service-account files
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase app: %v", err)
	}

	client, err := app.Auth(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error getting firebase auth client: %v", err)
	}

	return client, nil
}

// GoogleUser represents the key information extracted from the validated Google ID Token
type GoogleUser struct {
	UID           string
	Email         string
	Name          string
	Picture       string
	EmailVerified bool
}

// ErrGoogleLoginNotConfigured is returned when no Google OAuth client IDs are configured
var ErrGoogleLoginNotConfigured = errors.New("google login is not configured (GOOGLE_CLIENT_ID is empty)")

// VerifyGoogleToken verifies a Google ID token's signature and expiry, that it was issued
// by Google, and that it was issued for one of our OAuth client IDs (web, iOS, Android...).
func VerifyGoogleToken(ctx context.Context, idToken string, clientIDs []string) (*GoogleUser, error) {
	if len(clientIDs) == 0 {
		return nil, ErrGoogleLoginNotConfigured
	}

	// An empty audience makes idtoken skip the aud check; we check it ourselves below
	// so that several client IDs can be accepted.
	payload, err := idtoken.Validate(ctx, idToken, "")
	if err != nil {
		return nil, fmt.Errorf("invalid google token: %v", err)
	}

	if payload.Issuer != "accounts.google.com" && payload.Issuer != "https://accounts.google.com" {
		return nil, fmt.Errorf("invalid google token: unexpected issuer %q", payload.Issuer)
	}

	audienceOK := false
	for _, id := range clientIDs {
		if payload.Audience == id {
			audienceOK = true
			break
		}
	}
	if !audienceOK {
		return nil, errors.New("invalid google token: issued for a different client")
	}

	// Extract standard claims
	googleUser := &GoogleUser{
		UID: payload.Subject,
	}

	if email, ok := payload.Claims["email"].(string); ok {
		googleUser.Email = email
	}
	if name, ok := payload.Claims["name"].(string); ok {
		googleUser.Name = name
	}
	if picture, ok := payload.Claims["picture"].(string); ok {
		googleUser.Picture = picture
	}
	if verified, ok := payload.Claims["email_verified"].(bool); ok {
		googleUser.EmailVerified = verified
	}

	return googleUser, nil
}
