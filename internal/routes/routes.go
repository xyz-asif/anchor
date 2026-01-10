package routes

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/comments"
	"github.com/xyz-asif/gotodo/internal/features/feed"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/interests"
	"github.com/xyz-asif/gotodo/internal/features/likes"
	"github.com/xyz-asif/gotodo/internal/features/media"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/features/safety"
	"github.com/xyz-asif/gotodo/internal/features/search"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// authFollowServiceAdapter adapts follows.Repository to auth.FollowService interface
type authFollowServiceAdapter struct {
	repo *follows.Repository
}

func (s *authFollowServiceAdapter) GetStatus(ctx context.Context, f1, f2 primitive.ObjectID) (bool, bool, error) {
	return s.repo.GetFollowStatus(ctx, f1, f2)
}

func (s *authAnchorServiceAdapter) DeleteAllByUser(ctx context.Context, userID primitive.ObjectID) error {
	// 1. Get all anchors
	anchorsList, err := s.repo.GetAllUserAnchors(ctx, userID)
	if err != nil {
		return err
	}

	for _, anchor := range anchorsList {
		// 2. Get items to find assets
		items, err := s.repo.GetAnchorItems(ctx, anchor.ID)
		if err != nil {
			// Log error but continue? Or fail?
			// Continue to try to delete as much as possible
			continue
		}

		// 3. Delete assets from Cloudinary
		if s.cld != nil {
			for _, item := range items {
				if item.Type == "image" && item.ImageData != nil && item.ImageData.PublicID != "" {
					_ = s.cld.Delete(ctx, item.ImageData.PublicID, "image")
				}
				if item.Type == "audio" && item.AudioData != nil && item.AudioData.PublicID != "" {
					_ = s.cld.Delete(ctx, item.AudioData.PublicID, "video") // Audio uses "video" resource type in Cloudinary
				}
			}
		}

		// 4. Delete Anchor (and items from DB)
		if err := s.repo.DeleteAnchor(ctx, anchor.ID); err != nil {
			return err
		}
	}
	return nil
}

// authAnchorServiceAdapter adapts anchors.Repository to auth.AnchorService interface
type authAnchorServiceAdapter struct {
	repo *anchors.Repository
	cld  *cloudinary.Service
}

func (s *authAnchorServiceAdapter) GetPinnedAnchors(ctx context.Context, userID primitive.ObjectID, includePrivate bool) ([]auth.PinnedAnchorData, error) {
	anchorsList, err := s.repo.GetPinnedAnchors(ctx, userID, includePrivate)
	if err != nil {
		return nil, err
	}

	// Map generic anchor to auth DTO
	var result []auth.PinnedAnchorData
	for _, a := range anchorsList {
		result = append(result, auth.PinnedAnchorData{
			ID:              a.ID,
			Title:           a.Title,
			Description:     a.Description,
			CoverMediaType:  a.CoverMediaType,
			CoverMediaValue: a.CoverMediaValue,
			Visibility:      a.Visibility,
			ItemCount:       a.ItemCount,
			LikeCount:       a.LikeCount,
			CloneCount:      a.CloneCount,
			CreatedAt:       a.CreatedAt,
		})
	}
	return result, nil
}

func SetupRoutes(router *gin.Engine, db *mongo.Database, cfg *config.Config) {
	// API v1 group
	api := router.Group("/api/v1")

	// Initialize shared repositories needing external wiring
	followsRepo := follows.NewRepository(db)
	anchorsRepo := anchors.NewRepository(db)

	// Initialize Cloudinary for adapter usage (to delete assets)
	// We can reuse the config.
	cld, _ := cloudinary.NewService(cfg.CloudinaryCloudName, cfg.CloudinaryAPIKey, cfg.CloudinaryAPISecret, "anchor")

	// Create adapters for auth package
	followService := &authFollowServiceAdapter{repo: followsRepo}
	anchorService := &authAnchorServiceAdapter{repo: anchorsRepo, cld: cld}

	// Register feature routes
	auth.RegisterRoutes(api, db, cfg, followService, anchorService)
	anchors.RegisterRoutes(api, db, cfg)
	follows.RegisterRoutes(api, db, cfg)
	likes.RegisterRoutes(api, db, cfg)
	comments.RegisterRoutes(api, db, cfg)
	notifications.RegisterRoutes(api, db, cfg, anchorsRepo)
	search.RegisterRoutes(api, db, cfg)
	feed.RegisterRoutes(api, db, cfg)
	media.RegisterRoutes(api, db, cfg)
	interests.RegisterRoutes(api, db, cfg)
	safety.RegisterRoutes(api, db, cfg)
}
