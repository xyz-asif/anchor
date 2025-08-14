// ================== internal/features/auth/routes.go ==================
package auth

import (
	"github.com/xyz-asif/gotodo/internal/middleware"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database) {
	repo := NewRepository(db)
	handler := NewHandler(repo)

	auth := router.Group("/auth")
	{
		auth.POST("/register", handler.Register)
		auth.POST("/login", handler.Login)
		auth.GET("/me", middleware.Auth(), handler.Me)
	}
}
