package auth

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"github.com/xyz-asif/gotodo/internal/config"
)

// InitFirebase initializes the Firebase Admin SDK and returns the Auth client
func InitFirebase(cfg *config.Config) (*auth.Client, error) {
	opt := option.WithCredentialsFile(cfg.FirebaseServiceAccountPath)
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

// VerifyGoogleToken verifies the Google ID token using google.golang.org/api/idtoken
func VerifyGoogleToken(ctx context.Context, idToken string, clientID string) (*GoogleUser, error) {
	// If clientID is not provided, we might want to skip audience check or use a default?
	// idtoken.Validate requires valid audience if provided.
	// If clientID is empty, it might fail or validate against any audience (insecure).
	// However, usually we must provide it.

	payload, err := idtoken.Validate(ctx, idToken, clientID)
	if err != nil {
		return nil, fmt.Errorf("invalid google token: %v", err)
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
