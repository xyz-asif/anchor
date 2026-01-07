package follows

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterRoutes registers the follow-related routes
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	// Initialize repositories
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)
	notificationService := notifications.GetService(db)

	// Initialize handler
	handler := NewHandler(repo, authRepo, notificationService, cfg)

	// Initialize middlewares
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)
	optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

	// Follow routes under /users
	users := router.Group("/users")
	{
		// Protected routes (require authentication)
		users.POST("/:id/follow", authMiddleware, handler.FollowAction)
		users.GET("/:id/follow/status", authMiddleware, handler.GetFollowStatus)

		// Public route with optional auth
		users.GET("/:id/follows", optionalAuth, handler.ListFollows)
	}
}
