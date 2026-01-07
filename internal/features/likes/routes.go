package likes

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterRoutes registers the like-related routes
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	// Initialize repositories
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)
	anchorsRepo := anchors.NewRepository(db)
	followsRepo := follows.NewRepository(db)
	notificationService := notifications.GetService(db)

	// Initialize handler
	handler := NewHandler(repo, anchorsRepo, authRepo, notificationService, followsRepo, cfg)

	// Initialize middlewares
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)
	optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

	// Like routes under /anchors
	anchorsGroup := router.Group("/anchors")
	{
		// Protected routes
		anchorsGroup.POST("/:id/like", authMiddleware, handler.LikeAction)
		anchorsGroup.GET("/:id/like/status", authMiddleware, handler.GetLikeStatus)

		// Public routes with optional auth
		anchorsGroup.GET("/:id/likes", optionalAuth, handler.ListLikers)
		anchorsGroup.GET("/:id/like/summary", optionalAuth, handler.GetLikeSummaryEndpoint)
	}
}
