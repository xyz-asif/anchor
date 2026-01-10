package feed

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchor_follows"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/likes"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	// Initialize repositories
	feedRepo := NewRepository(db)
	authRepo := auth.NewRepository(db)
	followsRepo := follows.NewRepository(db)
	likesRepo := likes.NewRepository(db)
	anchorsRepo := anchors.NewRepository(db)
	anchorFollowsRepo := anchor_follows.GetRepository(db)

	// Initialize service
	service := NewService(feedRepo, authRepo, followsRepo, likesRepo, anchorsRepo)

	// Initialize handler
	handler := NewHandler(service, anchorFollowsRepo, cfg)

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)
	optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

	// Feed routes
	feed := router.Group("/feed")
	{
		// Home feed - requires auth
		feed.GET("/following", authMiddleware, handler.GetFollowingFeed)

		// Discovery feed - optional auth
		feed.GET("/discover", optionalAuth, handler.GetDiscoverFeed)
	}
}
