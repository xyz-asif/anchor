package interests

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	repo := NewRepository(db)
	service := NewService(repo)
	authRepo := auth.NewRepository(db)

	handler := NewHandler(service, cfg)

	optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

	interests := router.Group("/interests")
	interests.Use(optionalAuth)
	{
		interests.GET("/suggested", handler.GetSuggestedInterests)
	}
}
