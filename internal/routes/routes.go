package routes

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchor_follows"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/comments"
	"github.com/xyz-asif/gotodo/internal/features/feed"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/interests"
	"github.com/xyz-asif/gotodo/internal/features/likes"
	"github.com/xyz-asif/gotodo/internal/features/media"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/features/push"
	"github.com/xyz-asif/gotodo/internal/features/safety"
	"github.com/xyz-asif/gotodo/internal/features/search"
	"github.com/xyz-asif/gotodo/internal/features/users"
	"github.com/xyz-asif/gotodo/internal/pkg/cloudinary"
	"go.mongodb.org/mongo-driver/bson"
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
	// 1. Get all anchors (including soft-deleted ones)
	anchorsList, err := s.repo.GetAllUserAnchors(ctx, userID)
	if err != nil {
		return err
	}

	for _, anchor := range anchorsList {
		// 2. Delete the anchor's media files, unless other users' clones still use them.
		//    If items can't be read, still delete the anchor (files may be left behind).
		items, err := s.repo.GetAnchorItems(ctx, anchor.ID)
		if err != nil {
			log.Printf("Account deletion: could not read items of anchor %s: %v", anchor.ID.Hex(), err)
		}
		if s.cld != nil {
			for _, item := range items {
				var publicID, url, resourceType string
				switch {
				case item.ImageData != nil:
					publicID, url, resourceType = item.ImageData.PublicID, item.ImageData.CloudinaryURL, "image"
				case item.AudioData != nil:
					publicID, url, resourceType = item.AudioData.PublicID, item.AudioData.CloudinaryURL, "video" // audio is "video" in Cloudinary
				case item.FileData != nil:
					publicID, url, resourceType = item.FileData.PublicID, item.FileData.CloudinaryURL, "raw"
				}
				if publicID == "" || assetSharedByOthers(ctx, s.db, anchor.ID, url) {
					continue
				}
				_ = s.cld.Delete(ctx, publicID, resourceType)
			}
		}

		// A deleted clone lowers the original's clone count (soft-deleted ones already did)
		if anchor.ClonedFromAnchorID != nil && anchor.DeletedAt == nil {
			_ = s.repo.IncrementCloneCount(ctx, *anchor.ClonedFromAnchorID, -1)
		}

		// 3. Delete anchor and its items from the DB
		if err := s.repo.DeleteAnchor(ctx, anchor.ID); err != nil {
			return err
		}

		// Activity on this anchor by other users goes with it
		_, _ = s.db.Collection("likes").DeleteMany(ctx, bson.M{"anchorId": anchor.ID})
		_, _ = s.db.Collection("comments").DeleteMany(ctx, bson.M{"anchorId": anchor.ID})
		_, _ = s.db.Collection("anchor_follows").DeleteMany(ctx, bson.M{"anchorId": anchor.ID})
	}

	// 4. The user's activity on other people's content, with counters recomputed
	return cleanupUserActivity(ctx, s.db, userID)
}

// authAnchorServiceAdapter adapts anchors.Repository to auth.AnchorService interface
type authAnchorServiceAdapter struct {
	repo *anchors.Repository
	cld  *cloudinary.Service
	db   *mongo.Database
}

func (s *authAnchorServiceAdapter) GetPinnedAnchors(ctx context.Context, userID primitive.ObjectID, includePrivate bool) ([]auth.PinnedAnchorData, error) {
	anchorsList, err := s.repo.GetPinnedAnchors(ctx, userID, includePrivate)
	if err != nil {
		return nil, err
	}

	// generic anchor to auth DTO
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
	anchorFollowsRepo := anchor_follows.NewRepository(db)

	// Initialize Cloudinary for adapter usage (to delete assets)
	// We can reuse the config.
	cld, _ := cloudinary.NewService(cfg.CloudinaryCloudName, cfg.CloudinaryAPIKey, cfg.CloudinaryAPISecret, "anchor")

	// Create adapters for auth package
	followService := &authFollowServiceAdapter{repo: followsRepo}
	anchorService := &authAnchorServiceAdapter{repo: anchorsRepo, cld: cld, db: db}

	// Register feature routes
	users.RegisterRoutes(api, db, cfg)
	auth.RegisterRoutes(api, db, cfg, followService, anchorService)

	// Set follower provider to break cycle
	notifService := notifications.GetService(db)
	notifService.SetFollowerProvider(anchorFollowsRepo)

	anchors.RegisterRoutes(api, db, cfg, anchorFollowsRepo, notifService)
	anchor_follows.RegisterRoutes(api, db, cfg)
	follows.RegisterRoutes(api, db, cfg)
	likes.RegisterRoutes(api, db, cfg)
	comments.RegisterRoutes(api, db, cfg)

	notifications.RegisterRoutes(api, db, cfg, anchorsRepo)
	push.RegisterRoutes(api, db, cfg) // devices, push settings, and sending pushes for new notifications

	search.RegisterRoutes(api, db, cfg)
	feed.RegisterRoutes(api, db, cfg)
	media.RegisterRoutes(api, db, cfg)
	interests.RegisterRoutes(api, db, cfg)
	safety.RegisterRoutes(api, db, cfg)
}
