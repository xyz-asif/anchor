package anchor_follows

import (
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/middleware"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	repo := NewRepository(db)
	anchorsRepo := anchors.NewRepository(db)
	authRepo := auth.NewRepository(db)

	handler := NewHandler(repo, anchorsRepo, authRepo, cfg)

	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)

	// Anchor follow routes
	anchorRoutes := router.Group("/anchors/:id")
	anchorRoutes.Use(authMiddleware)
	{
		anchorRoutes.POST("/follow", handler.FollowAnchor)
		anchorRoutes.GET("/follow/status", handler.GetFollowStatus)
		anchorRoutes.PATCH("/follow/notifications", handler.ToggleNotifications)
	}

	// User following anchors route
	userRoutes := router.Group("/users/me")
	userRoutes.Use(authMiddleware)
	{
		userRoutes.GET("/following-anchors", handler.ListFollowingAnchors)
	}
}

// GetRepository returns repository instance for use by other modules
func GetRepository(db *mongo.Database) *Repository {
	return NewRepository(db)
}
