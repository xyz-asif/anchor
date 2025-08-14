// ================== internal/routes/routes.go ==================
package routes

import (
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/todos"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.Engine, db *mongo.Database) {
	// API v1 routes
	api := router.Group("/api/v1")
	{
		// Auth routes - public endpoints for login/register
		auth.RegisterRoutes(api, db)

		// Todo routes - protected endpoints
		todos.RegisterRoutes(api, db)

		// Add more feature routes here as the app grows
		// users.RegisterRoutes(api, db)
		// projects.RegisterRoutes(api, db)
		// notifications.RegisterRoutes(api, db)
	}

	// You can add API v2 routes in the future
	// apiV2 := router.Group("/api/v2")
	// {
	//     // v2 routes
	// }
}
