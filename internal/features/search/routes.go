package search

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	// Initialize repositories
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)
	followsRepo := follows.NewRepository(db)

	// Initialize handler
	handler := NewHandler(repo, authRepo, followsRepo, cfg)

	// Initialize middleware
	optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

	// Search routes (all with optional auth for isFollowing)
	search := router.Group("/search")
	search.Use(optionalAuth)
	{
		search.GET("", handler.UnifiedSearch)
		search.GET("/anchors", handler.SearchAnchors)
		search.GET("/users", handler.SearchUsers)
		search.GET("/tags", handler.SearchTags)
	}
}
