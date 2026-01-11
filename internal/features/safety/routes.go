package safety

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	repo := NewRepository(db)
	authRepo := auth.NewRepository(db)
	handler := NewHandler(repo, authRepo, cfg)
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)

	// Reports
	router.POST("/reports", authMiddleware, handler.CreateReport)

	// Block User - mounted under /users
	users := router.Group("/users")
	users.Use(authMiddleware)
	{
		users.POST("/:id/block", handler.BlockUser)
		users.DELETE("/:id/block", handler.UnblockUser)
		users.GET("/me/blocks", handler.GetBlockedUsers)
	}
}
