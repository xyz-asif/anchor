package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"go.mongodb.org/mongo-driver/mongo"
)

func SetupRoutes(router *gin.Engine, db *mongo.Database, cfg *config.Config) {
	// API v1 group
	api := router.Group("/api/v1")

	// Register feature routes
	auth.RegisterRoutes(api, db, cfg)
	anchors.RegisterRoutes(api, db, cfg)
	follows.RegisterRoutes(api, db, cfg)

	// Future features will be registered here:
	// follows.RegisterRoutes(api, db, cfg)
	// likes.RegisterRoutes(api, db, cfg)
}
