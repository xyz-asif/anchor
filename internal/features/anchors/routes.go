package anchors

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchor_follows"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterRoutes registers the anchor-related routes
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	// Initialize repositories
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)
	notificationService := notifications.GetService(db)
	anchorFollowsRepo := anchor_follows.GetRepository(db)

	// Initialize Cloudinary service
	cloudinarySvc, err := cloudinary.NewService(cfg.CloudinaryCloudName, cfg.CloudinaryAPIKey, cfg.CloudinaryAPISecret, "anchors")
	if err != nil {
		log.Printf("Failed to initialize cloudinary service: %v", err)
	}

	// Initialize handler (repos passed as nil to avoid import cycles)
	handler := NewHandler(repo, authRepo, notificationService, cfg, cloudinarySvc, nil, nil, anchorFollowsRepo)

	// Initialize auth middleware
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)
	optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

	// Anchor routes group
	anchors := router.Group("/anchors")
	{
		// Public routes WITH optional auth (can identify logged-in users)
		anchors.GET("/:id", optionalAuth, handler.GetAnchor)
		anchors.GET("/:id/items", optionalAuth, handler.ListAnchorItems)
		anchors.GET("", optionalAuth, handler.ListUserAnchors)

		// Protected routes (require authentication)
		protected := anchors.Group("")
		protected.Use(authMiddleware)
		{
			protected.POST("", handler.CreateAnchor)
			protected.PATCH("/:id", handler.UpdateAnchor)
			protected.DELETE("/:id", handler.DeleteAnchor)
			protected.POST("/:id/clone", handler.CloneAnchor)
			protected.PATCH("/:id/pin", handler.TogglePin)

			// Item routes
			protected.POST("/:id/items", handler.AddItem)
			protected.POST("/:id/items/upload", handler.UploadItem)
			protected.DELETE("/:id/items/:itemId", handler.DeleteItem)
			protected.PATCH("/:id/items/reorder", handler.ReorderItems)
		}
	}
}
