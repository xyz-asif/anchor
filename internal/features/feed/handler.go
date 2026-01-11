package feed

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Handler struct {
	service *Service
	config  *config.Config
}

func NewHandler(service *Service, cfg *config.Config) *Handler {
	return &Handler{
		service: service,
		config:  cfg,
	}
}

// GetFollowingFeed godoc
// @Summary Get home feed
// @Description Get personalized feed of anchors from followed users
// @Tags feed
// @Produce json
// @Security BearerAuth
// @Param limit query int false "Items per page (default 20, max 50)"
// @Param cursor query string false "Pagination cursor"
// @Param includeOwn query bool false "Include own anchors (default true)"
// @Success 200 {object} response.APIResponse{data=HomeFeedResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Router /feed/following [get]
func (h *Handler) GetFollowingFeed(c *gin.Context) {
	usr, exists := c.Get("user")

	if !exists {
		response.Error(c, http.StatusUnauthorized, "User not found in context")
		return
	}
	user := usr.(*auth.User)

	var query FeedQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid query parameters")
		return
	}

	if err := ValidateFeedQuery(&query); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	if query.Cursor != "" {
		if _, err := ValidateCursor(query.Cursor); err != nil {
			response.Error(c, http.StatusBadRequest, err.Error())
			return
		}
	}

	feedResponse, err := h.service.GetHomeFeed(c.Request.Context(), user.ID, &query)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to retrieve feed")
		return
	}

	response.Success(c, feedResponse)
}

// GetDiscoverFeed godoc
// @Summary Get discovery feed
// @Description Get discovery feed of trending/popular public anchors from non-followed users
// @Tags feed
// @Produce json
// @Param limit query int false "Items per page (default 20, max 50)"
// @Param cursor query string false "Pagination cursor"
// @Param category query string false "Category: trending, popular, recent (default trending)"
// @Param tag query string false "Filter by tag"
// @Success 200 {object} response.APIResponse{data=DiscoverResponse}
// @Failure 400 {object} response.APIResponse
// @Router /feed/discover [get]
func (h *Handler) GetDiscoverFeed(c *gin.Context) {
	// Get user if authenticated (optional)
	var userID *primitive.ObjectID
	if usr, exists := c.Get("user"); exists {
		if user, ok := usr.(*auth.User); ok {
			userID = &user.ID
		}
	}

	var query DiscoverQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid query parameters")
		return
	}

	if err := ValidateDiscoverQuery(&query); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	if query.Cursor != "" {
		if _, err := DecodeDiscoverCursor(query.Cursor); err != nil {
			response.Error(c, http.StatusBadRequest, "Invalid cursor")
			return
		}
	}

	discoverResponse, err := h.service.GetDiscoverFeed(c.Request.Context(), userID, &query)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to retrieve discovery feed")
		return
	}

	response.Success(c, discoverResponse)
}

// GetTagFeed godoc
// @Summary Get tag feed
// @Description Get public anchors for a specific tag
// @Tags feed
// @Produce json
// @Param tagName path string true "Tag name"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} response.APIResponse{data=DiscoverResponse}
// @Failure 400 {object} response.APIResponse
// @Router /feed/tags/{tagName} [get]
func (h *Handler) GetTagFeed(c *gin.Context) {
	tagName := c.Param("tagName")
	if len(tagName) < 2 {
		response.Error(c, http.StatusBadRequest, "Invalid tag name")
		return
	}

	// Get user if authenticated (optional)
	var userID *primitive.ObjectID
	if usr, exists := c.Get("user"); exists {
		if user, ok := usr.(*auth.User); ok {
			userID = &user.ID
		}
	}

	var query DiscoverQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid query parameters")
		return
	}

	// Force tag from path
	query.Tag = tagName
	if query.Category == "" {
		query.Category = "popular" // Default sort for tag feed
	}

	if err := ValidateDiscoverQuery(&query); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	if query.Cursor != "" {
		if _, err := DecodeDiscoverCursor(query.Cursor); err != nil {
			response.Error(c, http.StatusBadRequest, "Invalid cursor")
			return
		}
	}

	// Reuse Discover Feed logic
	discoverResponse, err := h.service.GetDiscoverFeed(c.Request.Context(), userID, &query)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to retrieve tag feed")
		return
	}

	response.Success(c, discoverResponse)
}
