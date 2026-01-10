package config

import (
	"log"
	"os"
	"strconv"

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
	GoogleClientID             string
	CloudinaryCloudName        string
	CloudinaryAPIKey           string
	CloudinaryAPISecret        string
	CloudinaryUploadFolder     string
	FrontendURL                string
	DevMode                    bool
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	jwtExpireHours, _ := strconv.Atoi(getEnv("JWT_EXPIRE_HOURS", "72"))
	refreshTokenExpireHours, _ := strconv.Atoi(getEnv("REFRESH_TOKEN_EXPIRE_HOURS", "168")) // 7 days default

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
		GoogleClientID:             getEnv("GOOGLE_CLIENT_ID", ""),
		CloudinaryCloudName:        getEnv("CLOUDINARY_CLOUD_NAME", ""),
		CloudinaryAPIKey:           getEnv("CLOUDINARY_API_KEY", ""),
		CloudinaryAPISecret:        getEnv("CLOUDINARY_API_SECRET", ""),
		CloudinaryUploadFolder:     getEnv("CLOUDINARY_UPLOAD_FOLDER", "anchor"),
		FrontendURL:                getEnv("FRONTEND_URL", "http://localhost:3000"),
		DevMode:                    getEnv("DEV_MODE", "false") == "true",
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
