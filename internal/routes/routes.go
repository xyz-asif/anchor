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
	"github.com/xyz-asif/gotodo/internal/features/likes"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/features/search"
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

// authAnchorServiceAdapter adapts anchors.Repository to auth.AnchorService interface
type authAnchorServiceAdapter struct {
	repo *anchors.Repository
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

	// Create adapters for auth package
	followService := &authFollowServiceAdapter{repo: followsRepo}
	anchorService := &authAnchorServiceAdapter{repo: anchorsRepo}

	// Register feature routes
	auth.RegisterRoutes(api, db, cfg, followService, anchorService)
	anchors.RegisterRoutes(api, db, cfg)
	follows.RegisterRoutes(api, db, cfg)
	likes.RegisterRoutes(api, db, cfg)
	comments.RegisterRoutes(api, db, cfg)
	notifications.RegisterRoutes(api, db, cfg, anchorsRepo)
	search.RegisterRoutes(api, db, cfg)
	feed.RegisterRoutes(api, db, cfg)
}
