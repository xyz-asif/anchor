// ================== internal/routes/routes.go ==================
package routes

import (
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.Engine, db *mongo.Database) {
	// API v1 routes
	// api := router.Group("/api/v1")
	// {
	// 	// Register feature routes here
	// }
}
