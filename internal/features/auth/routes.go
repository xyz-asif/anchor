package auth

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterRoutes registers the auth routes and initializes dependencies
// We accept followService and anchorService as interfaces because we can't import those packages due to cycle
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config, followService FollowService, anchorService AnchorService) {
	// Initialize Cloudinary service
	cloudinarySvc, err := cloudinary.NewService(cfg.CloudinaryCloudName, cfg.CloudinaryAPIKey, cfg.CloudinaryAPISecret, "profiles")
	if err != nil {
		log.Printf("Failed to initialize cloudinary service for auth: %v", err)
	}

	// Init dependencies
	repo := NewRepository(db)

	// Use the passed services
	handler := NewHandler(repo, cfg, cloudinarySvc, followService, anchorService)
	authMiddleware := NewAuthMiddleware(repo, cfg)

	// Auth routes
	auth := router.Group("/auth")
	{
		auth.POST("/google", handler.GoogleLogin)
		auth.POST("/dev-login", handler.DevLogin)
		auth.POST("/refresh", handler.RefreshToken)
		auth.POST("/logout", handler.Logout)
		auth.POST("/revoke-all", authMiddleware, handler.RevokeAllTokens)

		// Legacy/Compatible routes
		auth.GET("/me", authMiddleware, handler.GetMe)
		auth.PATCH("/profile", authMiddleware, handler.UpdateProfile)
		auth.GET("/username/check", handler.CheckUsernameAvailability)
		auth.PATCH("/username", authMiddleware, handler.UpdateUsername)
	}

	// User routes
	users := router.Group("/users")
	{
		// Own profile routes (must be first!)
		me := users.Group("/me")
		me.Use(authMiddleware)
		{
			me.GET("", handler.GetOwnProfile)
			me.PATCH("", handler.UpdateProfile)
			me.DELETE("", handler.DeleteAccount)
			me.POST("/profile-picture", handler.UploadProfilePicture)
			me.POST("/cover-image", handler.UploadCoverImage)
			me.DELETE("/profile-picture", handler.RemoveProfilePicture)
			me.DELETE("/cover-image", handler.RemoveCoverImage)
		}

		// Public profile routes (after /me). Optional auth: a valid token identifies the viewer.
		optionalAuth := NewOptionalAuthMiddleware(repo, cfg)
		users.GET("/:id", optionalAuth, handler.GetPublicProfile)
		users.GET("/:id/pinned", optionalAuth, handler.GetPinnedAnchors)
	}
}
