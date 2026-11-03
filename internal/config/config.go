package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                       string
	AppEnv                     string
	MongoURI                   string
	DBName                     string
	JWTSecret                  string
	JWTExpireHours             int
	RefreshTokenExpireHours    int
	FirebaseProjectID          string
	FirebaseServiceAccountPath string
	GoogleClientID             string   // kept for compatibility: first entry of GoogleClientIDs
	GoogleClientIDs            []string // all accepted OAuth client IDs (web, iOS, Android)
	CloudinaryCloudName        string
	CloudinaryAPIKey           string
	CloudinaryAPISecret        string
	CloudinaryUploadFolder     string
	FrontendURL                string
	DevMode                    bool
	AdminEmails                []string // users with these emails can access /admin endpoints
	TrustedProxies             []string // proxies whose X-Forwarded-For header is trusted for client IP
	RateLimitPerMinute         int      // general per-IP limit for all API routes
	AuthRateLimitPerMinute     int      // per-IP limit for login/refresh endpoints
	UploadRateLimitPerMinute   int      // per-IP limit for uploads and link previews
}

// defaultJWTSecret is the placeholder secret; it must never be used in production
const defaultJWTSecret = "change-this-secret"

// IsProduction reports whether the app runs in a non-development environment.
// Anything other than development/dev/local/test (e.g. "prod", "staging") is treated as
// production, so safety checks fail closed and dev-login stays disabled.
func (c *Config) IsProduction() bool {
	switch strings.ToLower(strings.TrimSpace(c.AppEnv)) {
	case "development", "dev", "local", "test":
		return false
	}
	return true
}

// IsAdminEmail reports whether the given email belongs to an admin
func (c *Config) IsAdminEmail(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, admin := range c.AdminEmails {
		if admin == email {
			return true
		}
	}
	return false
}

// Validate checks for unsafe settings. In production, problems are fatal errors;
// in other environments they are returned as warnings only.
func (c *Config) Validate() (warnings []string, err error) {
	var problems []string
	if c.JWTSecret == defaultJWTSecret || len(c.JWTSecret) < 32 {
		problems = append(problems, "JWT_SECRET must be set to a random value of at least 32 characters")
	}
	if c.DevMode {
		problems = append(problems, "DEV_MODE=true enables password-less /auth/dev-login")
	}
	if len(c.GoogleClientIDs) == 0 {
		problems = append(problems, "GOOGLE_CLIENT_ID is not set; Google login is disabled")
	}
	if len(problems) == 0 {
		return nil, nil
	}
	if c.IsProduction() {
		return nil, fmt.Errorf("unsafe production configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return problems, nil
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Invalid or non-positive values fall back to the defaults (0 would make every token born expired)
	jwtExpireHours := getEnvInt("JWT_EXPIRE_HOURS", 72)
	refreshTokenExpireHours := getEnvInt("REFRESH_TOKEN_EXPIRE_HOURS", 168) // 7 days default
	googleClientIDs := splitList(getEnv("GOOGLE_CLIENT_ID", ""))
	firstGoogleClientID := ""
	if len(googleClientIDs) > 0 {
		firstGoogleClientID = googleClientIDs[0]
	}

	return &Config{
		Port:                       getEnv("PORT", "8080"),
		AppEnv:                     getEnv("APP_ENV", "development"),
		MongoURI:                   getEnv("MONGODB_URI", "mongodb://localhost:27017/?replicaSet=rs0"),
		DBName:                     getEnv("DB_NAME", "anchor_db"),
		JWTSecret:                  getEnv("JWT_SECRET", "change-this-secret"),
		JWTExpireHours:             jwtExpireHours,
		RefreshTokenExpireHours:    refreshTokenExpireHours,
		FirebaseProjectID:          getEnv("FIREBASE_PROJECT_ID", ""),
		FirebaseServiceAccountPath: getEnv("FIREBASE_SERVICE_ACCOUNT_PATH", "./internal/config/serviceAccountKey.json"),
		GoogleClientID:             firstGoogleClientID,
		GoogleClientIDs:            googleClientIDs,
		CloudinaryCloudName:        getEnv("CLOUDINARY_CLOUD_NAME", ""),
		CloudinaryAPIKey:           getEnv("CLOUDINARY_API_KEY", ""),
		CloudinaryAPISecret:        getEnv("CLOUDINARY_API_SECRET", ""),
		CloudinaryUploadFolder:     getEnv("CLOUDINARY_UPLOAD_FOLDER", "anchor"),
		FrontendURL:                getEnv("FRONTEND_URL", "http://localhost:3000"),
		DevMode:                    getEnv("DEV_MODE", "false") == "true",
		AdminEmails:                lowerAll(splitList(getEnv("ADMIN_EMAILS", ""))),
		TrustedProxies:             splitList(getEnv("TRUSTED_PROXIES", "")),
		RateLimitPerMinute:         getEnvInt("RATE_LIMIT_PER_MINUTE", 300),
		AuthRateLimitPerMinute:     getEnvInt("AUTH_RATE_LIMIT_PER_MINUTE", 20),
		UploadRateLimitPerMinute:   getEnvInt("UPLOAD_RATE_LIMIT_PER_MINUTE", 30),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return defaultValue
}

// splitList splits a comma-separated value, trimming spaces and dropping empty entries
func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func lowerAll(values []string) []string {
	for i, v := range values {
		values[i] = strings.ToLower(v)
	}
	return values
}
