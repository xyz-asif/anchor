package feed

import (
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
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
	user := currentUser(c)
	if user == nil {
		response.Unauthorized(c, "Please sign in to see your feed", "UNAUTHORIZED")
		return
	}

	query, err := ParseFeedQuery(c.Query("limit"), c.Query("cursor"), c.Query("includeOwn"))
	if err != nil {
		response.BadRequest(c, err.Error(), queryErrorCode(err))
		return
	}

	feedResponse, err := h.service.GetHomeFeed(c.Request.Context(), user, query)
	if err != nil {
		log.Printf("feed: home feed for %s failed: %v", user.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't load your feed right now. Please try again", "FEED_FAILED")
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
// @Param category query string false "Category: trending, popular, recent, for_you (default trending). for_you ranks anchors tagged with your saved interests."
// @Param tag query string false "Filter by tag"
// @Success 200 {object} response.APIResponse{data=DiscoverResponse}
// @Failure 400 {object} response.APIResponse
// @Router /feed/discover [get]
func (h *Handler) GetDiscoverFeed(c *gin.Context) {
	query, err := ParseDiscoverQuery(c.Query("limit"), c.Query("cursor"), c.Query("category"), c.Query("tag"), CategoryTrending)
	if err != nil {
		response.BadRequest(c, err.Error(), queryErrorCode(err))
		return
	}

	discoverResponse, err := h.service.GetDiscoverFeed(c.Request.Context(), currentUser(c), query)
	if err != nil {
		log.Printf("feed: discover feed failed: %v", err)
		response.InternalServerError(c, "We couldn't load the discover feed right now. Please try again", "FEED_FAILED")
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
// @Param category query string false "Category: popular (default), trending, recent, for_you"
// @Success 200 {object} response.APIResponse{data=DiscoverResponse}
// @Failure 400 {object} response.APIResponse
// @Router /feed/tags/{tagName} [get]
func (h *Handler) GetTagFeed(c *gin.Context) {
	tagName := c.Param("tagName")
	if strings.TrimSpace(tagName) == "" {
		response.BadRequest(c, "Please say which tag to show, e.g. /feed/tags/travel", "INVALID_TAG")
		return
	}

	// Popular first by default
	query, err := ParseDiscoverQuery(c.Query("limit"), c.Query("cursor"), c.Query("category"), tagName, CategoryPopular)
	if err != nil {
		response.BadRequest(c, err.Error(), queryErrorCode(err))
		return
	}
	// A topic page shows everyone's anchors, including people you follow and your own
	query.IncludeFollowed = true

	discoverResponse, err := h.service.GetDiscoverFeed(c.Request.Context(), currentUser(c), query)
	if err != nil {
		log.Printf("feed: tag feed %q failed: %v", query.Tag, err)
		response.InternalServerError(c, "We couldn't load this tag right now. Please try again", "FEED_FAILED")
		return
	}

	response.Success(c, discoverResponse)
}

// currentUser returns the signed-in user, or nil for anonymous requests
func currentUser(c *gin.Context) *auth.User {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return user
		}
	}
	return nil
}

func queryErrorCode(err error) string {
	if err == errBadCursor {
		return "INVALID_CURSOR"
	}
	return "INVALID_QUERY"
}
