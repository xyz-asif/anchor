package anchors

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterRoutes registers the anchor-related routes
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	// Initialize repositories
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)

	// Initialize handler
	handler := NewHandler(repo, authRepo, cfg)

	// Initialize auth middleware
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)

	// Anchor routes group
	anchors := router.Group("/anchors")
	{
		// Optional Auth Middleware for public routes
		// We use a custom inline middleware or just a wrapper if needed,
		// but per instructions we currently expose them without the strict auth middleware
		// to allow public access. The handler handles "Exists" checks.
		// Note: To fully support "optional auth", an OptionalAuthMiddleware would be needed
		// that attempts to set the user but doesn't abort on failure.
		// For now, we follow the requested structure.

		// Public routes (no auth required)
		anchors.GET("/:id", handler.GetAnchor)
		anchors.GET("", handler.ListUserAnchors)

		// Protected routes (require authentication)
		protected := anchors.Group("")
		protected.Use(authMiddleware)
		{
			protected.POST("", handler.CreateAnchor)
			protected.PATCH("/:id", handler.UpdateAnchor)
			protected.DELETE("/:id", handler.DeleteAnchor)
			protected.PATCH("/:id/pin", handler.TogglePin)

			// Item routes
			protected.POST("/:id/items", handler.AddItem)
			protected.DELETE("/:id/items/:itemId", handler.DeleteItem)
		}
	}
}
