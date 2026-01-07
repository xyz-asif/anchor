package comments

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	// Initialize repositories
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)
	anchorsRepo := anchors.NewRepository(db)

	// Initialize notification service
	notificationService := notifications.GetService(db)

	// Initialize handler with notification service
	handler := NewHandler(repo, authRepo, anchorsRepo, notificationService, cfg)

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)
	optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

	// Anchor comment routes
	anchorComments := router.Group("/anchors/:id/comments")
	{
		anchorComments.POST("", authMiddleware, handler.AddComment)
		anchorComments.GET("", optionalAuth, handler.ListComments)
	}

	// Direct comment routes
	comments := router.Group("/comments")
	{
		comments.GET("/:id", optionalAuth, handler.GetComment)
		comments.PATCH("/:id", authMiddleware, handler.EditComment)
		comments.DELETE("/:id", authMiddleware, handler.DeleteComment)
		comments.POST("/:id/like", authMiddleware, handler.LikeComment)
		comments.GET("/:id/like/status", authMiddleware, handler.GetCommentLikeStatus)
	}
}
