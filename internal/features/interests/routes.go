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
	handler := NewHandler(repo, cfg)
	authRepo := auth.NewRepository(db)

	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)
	optionalAuth := middleware.OptionalAuthMiddleware(authRepo, cfg)

	// Interests routes
	interests := router.Group("/interests")
	{
		interests.GET("/suggested", optionalAuth, handler.GetSuggestedInterests)
	}

	// User related routes group could be passed here, or we group under global router
	// Spec says POST /users/me/interests
	userGroup := router.Group("/users")
	userGroup.Use(authMiddleware)
	{
		userGroup.POST("/me/interests", handler.SaveInterests)
	}
}
