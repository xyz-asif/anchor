package likes

import (
	"context"
	"errors"
	"log"
	"math"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/follows"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	msgAnchorNotFound = "This anchor doesn't exist or has been deleted"
	msgTryAgain       = "Something went wrong on our side. Please try again in a moment"
	summaryUsersShown = 3  // users named in the like summary
	summaryCandidates = 20 // recent likers considered for the summary
)

// Handler handles like-related HTTP requests
type Handler struct {
	repo                *Repository
	anchorsRepo         *anchors.Repository
	authRepo            *auth.Repository
	notificationService *notifications.Service
	followsRepo         *follows.Repository
	config              *config.Config
}

// NewHandler creates a new like handler
func NewHandler(repo *Repository, anchorsRepo *anchors.Repository, authRepo *auth.Repository, notificationService *notifications.Service, followsRepo *follows.Repository, cfg *config.Config) *Handler {
	return &Handler{
		repo:                repo,
		anchorsRepo:         anchorsRepo,
		authRepo:            authRepo,
		notificationService: notificationService,
		followsRepo:         followsRepo,
		config:              cfg,
	}
}

// currentUser returns the authenticated user, or nil when logged out
func currentUser(c *gin.Context) *auth.User {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return user
		}
	}
	return nil
}

// loadAnchor parses :id and loads the anchor (deleted ones included)
func (h *Handler) loadAnchor(c *gin.Context) (*anchors.Anchor, bool) {
	raw := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		response.BadRequest(c, `"`+raw+`" isn't a valid anchor ID`, "INVALID_ID")
		return nil, false
	}
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		if errors.Is(err, anchors.ErrAnchorNotFound) {
			response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		} else {
			log.Printf("likes: loading anchor %s failed: %v", raw, err)
			response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		}
		return nil, false
	}
	return anchor, true
}

// loadViewableAnchor loads an anchor the viewer (nil = anonymous) may see. Deleted anchors,
// someone else's private anchor, and anchors whose owner blocked the viewer all answer 404,
// so none of them is revealed.
func (h *Handler) loadViewableAnchor(c *gin.Context, viewer *auth.User) (*anchors.Anchor, bool) {
	anchor, ok := h.loadAnchor(c)
	if !ok {
		return nil, false
	}
	var viewerID *primitive.ObjectID
	if viewer != nil {
		viewerID = &viewer.ID
	}
	if !anchor.IsAccessibleBy(viewerID) {
		response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		return nil, false
	}
	if viewer != nil && viewer.ID != anchor.UserID {
		blocked, err := h.authRepo.UserHasBlocked(c.Request.Context(), anchor.UserID, viewer.ID)
		if err != nil {
			log.Printf("likes: block check failed: %v", err)
			response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
			return nil, false
		}
		if blocked {
			response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
			return nil, false
		}
	}
	return anchor, true
}

// LikeAction godoc
// @Summary Like or unlike anchor
// @Description Like or unlike an anchor based on the action specified
// @Tags likes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param request body LikeActionRequest true "Like action"
// @Success 200 {object} response.APIResponse{data=LikeActionResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/like [post]
func (h *Handler) LikeAction(c *gin.Context) {
	me := currentUser(c)
	if me == nil {
		response.Unauthorized(c, "Please sign in to like anchors", "UNAUTHORIZED")
		return
	}

	var req LikeActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, `The request body must be JSON like {"action": "like"} or {"action": "unlike"}`, "INVALID_REQUEST")
		return
	}
	if err := ValidateLikeActionRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_ACTION")
		return
	}

	ctx := c.Request.Context()
	var anchor *anchors.Anchor
	var ok bool

	if req.Action == "unlike" {
		// Unliking always works, even if the anchor went private, was deleted, or its owner
		// blocked you, so nobody is stuck with a like they can't remove
		if anchor, ok = h.loadAnchor(c); !ok {
			return
		}
		deleted, err := h.repo.DeleteLike(ctx, anchor.ID, me.ID)
		if err != nil {
			log.Printf("likes: unlike failed: %v", err)
			response.InternalServerError(c, "We couldn't remove your like right now. Please try again", "UNLIKE_FAILED")
			return
		}
		// Only decrement when a like was actually removed
		if deleted {
			h.afterLikeChange(ctx, anchor.ID, -1)
		}
		h.respondWithCount(c, anchor, false)
		return
	}

	if anchor, ok = h.loadViewableAnchor(c, me); !ok {
		return
	}
	if anchor.UserID != me.ID && me.HasBlocked(anchor.UserID) {
		response.Forbidden(c, "You've blocked the owner of this anchor. Unblock them first if you want to like it", "USER_BLOCKED")
		return
	}

	// The unique index makes this safe under concurrency: only the request that
	// actually inserted the like changes the count and sends a notification.
	created, err := h.repo.CreateLike(ctx, anchor.ID, me.ID)
	if err != nil {
		log.Printf("likes: like failed: %v", err)
		response.InternalServerError(c, "We couldn't like this anchor right now. Please try again", "LIKE_FAILED")
		return
	}

	if created {
		// Re-check the block after inserting, in case the owner blocked this user meanwhile
		if anchor.UserID != me.ID {
			if blocked, err := h.authRepo.IsBlockedBetween(ctx, me, anchor.UserID); err != nil || blocked {
				if _, delErr := h.repo.DeleteLike(ctx, anchor.ID, me.ID); delErr != nil {
					log.Printf("likes: rolling back like failed: %v", delErr)
				}
				if err != nil {
					log.Printf("likes: block re-check failed: %v", err)
					response.InternalServerError(c, "We couldn't like this anchor right now. Please try again", "LIKE_FAILED")
				} else {
					response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
				}
				return
			}
		}

		h.afterLikeChange(ctx, anchor.ID, 1)
		if anchor.UserID != me.ID && h.notificationService != nil {
			go func(anchorID primitive.ObjectID, title string, actorID, ownerID primitive.ObjectID) {
				if err := h.notificationService.CreateLikeNotification(context.Background(), anchorID, title, actorID, ownerID); err != nil {
					log.Printf("likes: like notification failed: %v", err)
				}
			}(anchor.ID, anchor.Title, me.ID, anchor.UserID)
		}
	}

	h.respondWithCount(c, anchor, true)
}

// afterLikeChange updates the anchor's like count and ranking score
func (h *Handler) afterLikeChange(ctx context.Context, anchorID primitive.ObjectID, delta int) {
	if err := h.anchorsRepo.IncrementLikeCount(ctx, anchorID, delta); err != nil {
		log.Printf("likes: likeCount %+d for %s failed: %v", delta, anchorID.Hex(), err)
	}
	go func(id primitive.ObjectID) {
		if err := h.anchorsRepo.UpdateEngagementScore(context.Background(), id); err != nil {
			log.Printf("likes: engagement score for %s failed: %v", id.Hex(), err)
		}
	}(anchorID)
}

// respondWithCount answers a like action with the anchor's current like count
func (h *Handler) respondWithCount(c *gin.Context, anchor *anchors.Anchor, hasLiked bool) {
	likeCount := anchor.LikeCount
	if updated, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchor.ID); err == nil {
		likeCount = updated.LikeCount
	}
	response.Success(c, LikeActionResponse{HasLiked: hasLiked, LikeCount: likeCount})
}

// GetLikeStatus godoc
// @Summary Check like status
// @Description Check if the current user has liked the anchor
// @Tags likes
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse{data=LikeStatusResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/like/status [get]
func (h *Handler) GetLikeStatus(c *gin.Context) {
	me := currentUser(c)
	if me == nil {
		response.Unauthorized(c, "Please sign in to check whether you liked this anchor", "UNAUTHORIZED")
		return
	}
	anchor, ok := h.loadViewableAnchor(c, me)
	if !ok {
		return
	}

	hasLiked, err := h.repo.ExistsLike(c.Request.Context(), anchor.ID, me.ID)
	if err != nil {
		log.Printf("likes: like lookup failed: %v", err)
		response.InternalServerError(c, "We couldn't check whether you liked this anchor. Please try again", "STATUS_CHECK_FAILED")
		return
	}

	response.Success(c, LikeStatusResponse{HasLiked: hasLiked, LikeCount: anchor.LikeCount})
}

// ListLikers godoc
// @Summary List users who liked anchor
// @Description Get paginated list of users who liked the anchor
// @Tags likes
// @Produce json
// @Param id path string true "Anchor ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse{data=PaginatedLikersResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/likes [get]
func (h *Handler) ListLikers(c *gin.Context) {
	query, err := ParseLikeListQuery(c.Query("page"), c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}
	viewer := currentUser(c)
	anchor, ok := h.loadViewableAnchor(c, viewer)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// People in a block relationship with the viewer are left out of the list and its total
	hidden, err := h.authRepo.HiddenUserIDs(ctx, viewer)
	if err != nil {
		log.Printf("likes: loading blocks failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return
	}

	likes, total, err := h.repo.GetLikers(ctx, anchor.ID, hidden, query.Page, query.Limit)
	if err != nil {
		log.Printf("likes: listing likers failed: %v", err)
		response.InternalServerError(c, "We couldn't load who liked this anchor. Please try again", "FETCH_FAILED")
		return
	}

	userIDs := make([]primitive.ObjectID, 0, len(likes))
	for _, like := range likes {
		userIDs = append(userIDs, like.UserID)
	}
	users, err := h.authRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		log.Printf("likes: loading likers failed: %v", err)
		response.InternalServerError(c, "We couldn't load who liked this anchor. Please try again", "FETCH_USERS_FAILED")
		return
	}
	userMap := make(map[primitive.ObjectID]*auth.User, len(users))
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}

	followingMap := map[primitive.ObjectID]bool{}
	if viewer != nil {
		if followingMap, err = h.followsRepo.GetFollowingIDs(ctx, viewer.ID, userIDs); err != nil {
			log.Printf("likes: loading follow status failed: %v", err)
			response.InternalServerError(c, "We couldn't load who liked this anchor. Please try again", "FETCH_FAILED")
			return
		}
	}

	likerUsers := make([]LikerUserResponse, 0, len(likes)) // [] rather than null when empty
	for _, like := range likes {
		user, exists := userMap[like.UserID]
		if !exists {
			continue
		}
		picture := user.ProfilePictureURL
		likerUsers = append(likerUsers, LikerUserResponse{
			ID:             user.ID,
			Username:       user.Username,
			DisplayName:    user.DisplayName,
			ProfilePicture: &picture,
			IsFollowing:    followingMap[like.UserID],
			LikedAt:        like.CreatedAt,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))
	resp := PaginatedLikersResponse{Data: likerUsers}
	resp.Pagination.Page = query.Page
	resp.Pagination.Limit = query.Limit
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = totalPages
	resp.Pagination.HasMore = query.Page < totalPages
	response.Success(c, resp)
}

// GetLikeSummary returns the like summary: the total, whether the viewer liked it, and up to 3
// recent likers, people the viewer follows first.
func (h *Handler) GetLikeSummary(ctx context.Context, anchor *anchors.Anchor, viewer *auth.User) (*LikeSummaryResponse, error) {
	summary := &LikeSummaryResponse{
		TotalCount:       anchor.LikeCount,
		LikedByFollowing: []LikeSummaryUser{},
	}
	if anchor.LikeCount == 0 {
		return summary, nil
	}

	if viewer != nil {
		hasLiked, err := h.repo.ExistsLike(ctx, anchor.ID, viewer.ID)
		if err != nil {
			return nil, err
		}
		summary.HasLiked = hasLiked
	}

	// Never name people in a block relationship with the viewer
	hidden, err := h.authRepo.HiddenUserIDs(ctx, viewer)
	if err != nil {
		return nil, err
	}
	recentLikes, err := h.repo.GetRecentLikers(ctx, anchor.ID, hidden, summaryCandidates)
	if err != nil {
		return nil, err
	}
	likerIDs := make([]primitive.ObjectID, 0, len(recentLikes))
	for _, like := range recentLikes {
		likerIDs = append(likerIDs, like.UserID)
	}

	// Followed likers first, then the rest, each group newest first
	prioritized := likerIDs
	if viewer != nil {
		followingMap, err := h.followsRepo.GetFollowingIDs(ctx, viewer.ID, likerIDs)
		if err != nil {
			return nil, err
		}
		var followed, others []primitive.ObjectID
		for _, id := range likerIDs {
			if followingMap[id] {
				followed = append(followed, id)
			} else {
				others = append(others, id)
			}
		}
		prioritized = append(followed, others...)
	}
	if len(prioritized) > summaryUsersShown {
		prioritized = prioritized[:summaryUsersShown]
	}

	users, err := h.authRepo.GetUsersByIDs(ctx, prioritized)
	if err != nil {
		return nil, err
	}
	byID := make(map[primitive.ObjectID]*auth.User, len(users))
	for i := range users {
		byID[users[i].ID] = &users[i]
	}
	// The database returns users in its own order; keep the priority order instead
	for _, id := range prioritized {
		if user, ok := byID[id]; ok {
			picture := user.ProfilePictureURL
			summary.LikedByFollowing = append(summary.LikedByFollowing, LikeSummaryUser{
				ID:             user.ID,
				Username:       user.Username,
				DisplayName:    user.DisplayName,
				ProfilePicture: &picture,
			})
		}
	}

	summary.OtherLikersCount = anchor.LikeCount - len(summary.LikedByFollowing)
	if summary.OtherLikersCount < 0 {
		summary.OtherLikersCount = 0
	}
	return summary, nil
}

// GetLikeSummaryEndpoint godoc
// @Summary Get like summary for anchor
// @Description Get summary of who liked an anchor, prioritizing users you follow
// @Tags likes
// @Produce json
// @Param id path string true "Anchor ID"
// @Success 200 {object} response.APIResponse{data=LikeSummaryResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/like/summary [get]
func (h *Handler) GetLikeSummaryEndpoint(c *gin.Context) {
	viewer := currentUser(c)
	anchor, ok := h.loadViewableAnchor(c, viewer)
	if !ok {
		return
	}

	summary, err := h.GetLikeSummary(c.Request.Context(), anchor, viewer)
	if err != nil {
		log.Printf("likes: summary for %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't load who liked this anchor. Please try again", "FETCH_FAILED")
		return
	}

	response.Success(c, summary)
}
