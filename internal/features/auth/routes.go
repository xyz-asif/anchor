package auth

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	idToken "github.com/xyz-asif/gotodo/internal/pkg/jwt"
	"go.mongodb.org/mongo-driver/mongo"
)

// RegisterRoutes registers the auth routes and initializes dependencies
// We accept followService and anchorService as interfaces because we can't import those packages due to cycle
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config, followService FollowService, anchorService AnchorService) {
	// Init Firebase
	firebaseClient, err := InitFirebase(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase: %v", err)
	}

	// Initialize Cloudinary service
	cloudinarySvc, err := cloudinary.NewService(cfg.CloudinaryCloudName, cfg.CloudinaryAPIKey, cfg.CloudinaryAPISecret, "profiles")
	if err != nil {
		log.Printf("Failed to initialize cloudinary service for auth: %v", err)
	}

	// Init dependencies
	repo := NewRepository(db)

	// Use the passed services
	handler := NewHandler(repo, firebaseClient, cfg, cloudinarySvc, followService, anchorService)
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

		// Public profile routes (after /me)
		users.GET("/:id", func(c *gin.Context) {
			// Manually run auth check without aborting
			tokenString := c.GetHeader("Authorization")
			if tokenString == "" || len(tokenString) < 8 {
				c.Next()
				return
			}
			bearerToken := tokenString[7:]
			claims, err := idToken.ValidateToken(bearerToken, cfg.JWTSecret)
			if err == nil {
				user, err := repo.GetUserByID(c.Request.Context(), claims.UserID)
				if err == nil {
					c.Set("user", user)
				}
			}
			c.Next()
		}, handler.GetPublicProfile)

		// Pinned anchors route
		// Note: We use the same manual auth middleware for optional auth here too if needed,
		// but GetPinnedAnchors logic handles "user" context if present.
		// However, handler.go logic currently uses `c.Get("user")` directly.
		// If we want it to work for unauthenticated users (public view), we need optional auth.
		// Reuse the inline middleware logic
		optionalAuth := func(c *gin.Context) {
			tokenString := c.GetHeader("Authorization")
			if tokenString == "" || len(tokenString) < 8 {
				c.Next()
				return
			}
			bearerToken := tokenString[7:]
			claims, err := idToken.ValidateToken(bearerToken, cfg.JWTSecret)
			if err == nil {
				user, err := repo.GetUserByID(c.Request.Context(), claims.UserID)
				if err == nil {
					c.Set("user", user)
				}
			}
			c.Next()
		}

		users.GET("/:id/pinned", optionalAuth, handler.GetPinnedAnchors)
	}
}
