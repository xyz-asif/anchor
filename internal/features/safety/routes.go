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

		// Mute: hide someone's content from my feeds without blocking them
		users.POST("/:id/mute", handler.MuteUser)
		users.DELETE("/:id/mute", handler.UnmuteUser)
		users.GET("/me/mutes", handler.GetMutedUsers)
	}

	// "Not interested" in an anchor: hide it from my discover/tag feeds
	anchors := router.Group("/anchors")
	anchors.Use(authMiddleware)
	{
		anchors.POST("/:id/hide", handler.HideAnchor)
		anchors.DELETE("/:id/hide", handler.UnhideAnchor)
	}

	// Moderation (admins are configured via ADMIN_EMAILS)
	admin := router.Group("/admin", authMiddleware, middleware.RequireAdmin(cfg))
	{
		admin.GET("/reports", handler.AdminListReports)
		admin.GET("/reports/:id", handler.AdminGetReport)
		admin.PATCH("/reports/:id", handler.AdminUpdateReport)
		admin.PATCH("/users/:id/verification", handler.AdminSetVerification)
	}
}
