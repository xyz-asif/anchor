package media

import (
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/middleware"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database, cfg *config.Config) {
	// Initialize Cloudinary service (reusing config)
	// Note: Ideally pass the existing service from main, but creating new instance is acceptable if config is same
	// However, `anchors` follows passed Cld service pattern.
	// But `RegisterRoutes` signature doesn't take services.
	// I'll create a new instance here.

	cld, err := cloudinary.NewService(cfg.CloudinaryCloudName, cfg.CloudinaryAPIKey, cfg.CloudinaryAPISecret, "media")
	if err != nil {
		log.Printf("Failed to initialize cloudinary service for media: %v", err)
	}

	handler := NewHandler(cld)

	// Both endpoints spend our resources (Cloudinary storage, outbound fetches): login required
	authMiddleware := middleware.NewAuthMiddleware(auth.NewRepository(db), cfg)
	media := router.Group("/media")
	media.Use(authMiddleware)
	{
		media.POST("/upload", handler.UploadMedia)
		media.GET("/preview", handler.GetLinkPreview)
	}
}
