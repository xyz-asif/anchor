package users

import (
	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/likes"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	authRepo := auth.NewRepository(db)
	likesRepo := likes.NewRepository(db)
	anchorsRepo := anchors.NewRepository(db)
	followsRepo := follows.NewRepository(db)
	handler := NewHandler(authRepo, likesRepo, anchorsRepo, followsRepo)
	authMiddleware := middleware.NewAuthMiddleware(authRepo, cfg)
	optionalAuthMiddleware := middleware.OptionalAuthMiddleware(authRepo, cfg)

	// User routes
	users := router.Group("/users")
	{
		// Profile by username - optional auth for follow status
		users.GET("/username/:username", optionalAuthMiddleware, handler.GetUserByUsername)

		// Profile by ID is already in auth/routes.go, but we might want to override or ensure consistency.
		// For now, focusing on the ones requested in completion doc.

		// Likes
		users.GET("/me/likes", authMiddleware, handler.GetUserLikes)
		users.GET("/:id/likes", handler.GetUserLikes)

		// Clones
		users.GET("/me/clones", authMiddleware, handler.GetUserClones)
		users.GET("/:id/clones", handler.GetUserClones)
	}
}
