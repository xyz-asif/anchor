package notifications

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config, contentProvider ContentProvider) {
	// Initialize repositories
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)

	// Initialize handler
	handler := NewHandler(repo, authRepo, contentProvider, cfg)

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)

	// Notification routes
	notifications := router.Group("/notifications")
	notifications.Use(authMiddleware)
	{
		notifications.GET("", handler.ListNotifications)
		notifications.GET("/unread-count", handler.GetUnreadCount)
		notifications.PATCH("/:id/read", handler.MarkAsRead)
		notifications.PATCH("/read-all", handler.MarkAllAsRead)
	}
}

// GetService returns a notification service for use by other modules
func GetService(db *mongo.Database) *Service {
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)

	return NewService(repo, authRepo)
}
