package auth

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterRoutes registers the auth routes and initializes dependencies
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	// Init Firebase
	firebaseClient, err := InitFirebase(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase: %v", err)
	}

	// Init dependencies
	repo := NewRepository(db)
	handler := NewHandler(repo, firebaseClient, cfg)
	authMiddleware := NewAuthMiddleware(repo, cfg)

	// Register routes
	auth := router.Group("/auth")
	{
		auth.POST("/google", handler.GoogleLogin)

		// Protected routes
		auth.GET("/me", authMiddleware, handler.GetMe)
		auth.PATCH("/profile", authMiddleware, handler.UpdateProfile)
		auth.PATCH("/username", authMiddleware, handler.UpdateUsername)
	}
}
