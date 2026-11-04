package follows

import (
	"context"
	"errors"
	"log"
	"math"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Handler handles follow-related HTTP requests
type Handler struct {
	repo                *Repository
	authRepo            *auth.Repository
	notificationService *notifications.Service
	config              *config.Config
}

// NewHandler creates a new follow handler
func NewHandler(repo *Repository, authRepo *auth.Repository, notificationService *notifications.Service, cfg *config.Config) *Handler {
	return &Handler{
		repo:                repo,
		authRepo:            authRepo,
		notificationService: notificationService,
		config:              cfg,
	}
}

// currentUser returns the authenticated user, or nil for anonymous requests
func currentUser(c *gin.Context) *auth.User {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return user
		}
	}
	return nil
}

// loadTargetUser parses the :id path parameter ("me" = the viewer) and loads that user, hiding
// users who blocked the viewer. On failure it writes the response and returns ok=false.
func (h *Handler) loadTargetUser(c *gin.Context, viewer *auth.User) (*auth.User, bool) {
	idStr := c.Param("id")
	if idStr == "me" {
		if viewer == nil {
			response.Unauthorized(c, "Please sign in to see your own followers and following", "AUTH_REQUIRED")
			return nil, false
		}
		return viewer, true
	}

	targetID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		response.BadRequest(c, `"`+idStr+`" isn't a valid user ID`, "INVALID_ID")
		return nil, false
	}

	target, err := h.authRepo.GetUserByObjectID(c.Request.Context(), targetID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			response.NotFound(c, "This user doesn't exist or has deleted their account", "USER_NOT_FOUND")
		} else {
			log.Printf("follows: loading user %s failed: %v", idStr, err)
			response.InternalServerError(c, "We couldn't load this user right now. Please try again", "DATABASE_ERROR")
		}
		return nil, false
	}
	if !auth.CanViewProfile(target, viewer) {
		response.NotFound(c, "This user doesn't exist or has deleted their account", "USER_NOT_FOUND")
		return nil, false
	}
	return target, true
}

// FollowAction godoc
// @Summary Follow or unfollow user
// @Description Follow or unfollow a user based on the action specified
// @Tags follows
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Target user's ID"
// @Param request body FollowActionRequest true "Follow action"
// @Success 200 {object} response.APIResponse{data=FollowActionResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/follow [post]
func (h *Handler) FollowAction(c *gin.Context) {
	me := currentUser(c)
	if me == nil {
		response.Unauthorized(c, "Please sign in to follow people", "UNAUTHORIZED")
		return
	}

	var req FollowActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, `The request body must be JSON like {"action": "follow"} or {"action": "unfollow"}`, "INVALID_REQUEST")
		return
	}
	if err := ValidateFollowActionRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_ACTION")
		return
	}

	if c.Param("id") == "me" || c.Param("id") == me.ID.Hex() {
		response.BadRequest(c, "You can't follow or unfollow yourself", "CANNOT_FOLLOW_SELF")
		return
	}

	// Unfollowing a user who blocked you must still work, so only "follow" hides them
	var target *auth.User
	if req.Action == "follow" {
		var ok bool
		if target, ok = h.loadTargetUser(c, me); !ok {
			return
		}
	} else {
		var ok bool
		if target, ok = h.loadTargetUser(c, nil); !ok {
			return
		}
	}

	ctx := c.Request.Context()
	targetID := target.ID
	var isFollowing bool

	if req.Action == "follow" {
		if me.HasBlocked(targetID) {
			response.Forbidden(c, "You've blocked this user. Unblock them first if you want to follow them", "USER_BLOCKED")
			return
		}
		if target.HasBlocked(me.ID) {
			// Not reachable through loadTargetUser (it reports 404), kept as a safety net
			response.Forbidden(c, "You can't follow this account", "USER_BLOCKED")
			return
		}

		created, err := h.repo.CreateFollow(ctx, me.ID, targetID)
		if err != nil {
			log.Printf("follows: create %s -> %s failed: %v", me.ID.Hex(), targetID.Hex(), err)
			response.InternalServerError(c, "We couldn't follow this user right now. Please try again", "FOLLOW_FAILED")
			return
		}

		if created {
			// Re-check the block *after* inserting: if a block landed between the check above
			// and the insert, the blocker's follow cleanup may already have run and missed it.
			if blocked, err := h.authRepo.IsBlockedBetween(ctx, me, targetID); err != nil || blocked {
				_, _ = h.repo.DeleteFollow(ctx, me.ID, targetID)
				if err != nil {
					log.Printf("follows: block re-check failed: %v", err)
					response.InternalServerError(c, "We couldn't follow this user right now. Please try again", "FOLLOW_FAILED")
				} else {
					response.Forbidden(c, "You can't follow this account", "USER_BLOCKED")
				}
				return
			}

			// Only the request that actually inserted the follow updates counts and notifies
			if err := h.authRepo.IncrementFollowerCount(ctx, targetID, 1); err != nil {
				log.Printf("follows: followerCount +1 for %s failed: %v", targetID.Hex(), err)
			}
			if err := h.authRepo.IncrementFollowingCount(ctx, me.ID, 1); err != nil {
				log.Printf("follows: followingCount +1 for %s failed: %v", me.ID.Hex(), err)
			}
			if h.notificationService != nil {
				go func(actorID, target primitive.ObjectID) {
					if err := h.notificationService.CreateFollowNotification(context.Background(), actorID, target); err != nil {
						log.Printf("follows: follow notification failed: %v", err)
					}
				}(me.ID, targetID)
			}
		}
		isFollowing = true
	} else {
		deleted, err := h.repo.DeleteFollow(ctx, me.ID, targetID)
		if err != nil {
			log.Printf("follows: delete %s -> %s failed: %v", me.ID.Hex(), targetID.Hex(), err)
			response.InternalServerError(c, "We couldn't unfollow this user right now. Please try again", "UNFOLLOW_FAILED")
			return
		}
		// Only decrement when a follow was actually removed
		if deleted {
			if err := h.authRepo.IncrementFollowerCount(ctx, targetID, -1); err != nil {
				log.Printf("follows: followerCount -1 for %s failed: %v", targetID.Hex(), err)
			}
			if err := h.authRepo.IncrementFollowingCount(ctx, me.ID, -1); err != nil {
				log.Printf("follows: followingCount -1 for %s failed: %v", me.ID.Hex(), err)
			}
		}
		isFollowing = false
	}

	// Fetch updated counts (keep the earlier values if a re-read fails)
	if updated, err := h.authRepo.GetUserByObjectID(ctx, targetID); err == nil {
		target = updated
	}
	if updated, err := h.authRepo.GetUserByObjectID(ctx, me.ID); err == nil {
		me = updated
	}

	response.Success(c, FollowActionResponse{
		IsFollowing: isFollowing,
		TargetUser: FollowTargetUserInfo{
			ID:            target.ID,
			Username:      target.Username,
			DisplayName:   target.DisplayName,
			FollowerCount: target.FollowerCount,
		},
		CurrentUser: FollowCurrentUserInfo{
			FollowingCount: me.FollowingCount,
		},
	})
}

// GetFollowStatus godoc
// @Summary Check follow status
// @Description Check if the current user follows the target user, and if the target follows back
// @Tags follows
// @Produce json
// @Security BearerAuth
// @Param id path string true "Target user's ID"
// @Success 200 {object} response.APIResponse{data=FollowStatusResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/follow/status [get]
func (h *Handler) GetFollowStatus(c *gin.Context) {
	me := currentUser(c)
	if me == nil {
		response.Unauthorized(c, "Please sign in to check follow status", "UNAUTHORIZED")
		return
	}

	target, ok := h.loadTargetUser(c, me)
	if !ok {
		return
	}
	if target.ID == me.ID {
		response.Success(c, FollowStatusResponse{}) // you don't follow yourself
		return
	}

	isFollowing, isFollowedBy, err := h.repo.GetFollowStatus(c.Request.Context(), me.ID, target.ID)
	if err != nil {
		log.Printf("follows: status %s / %s failed: %v", me.ID.Hex(), target.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't check the follow status right now. Please try again", "STATUS_CHECK_FAILED")
		return
	}

	response.Success(c, FollowStatusResponse{
		IsFollowing:  isFollowing,
		IsFollowedBy: isFollowedBy,
		IsMutual:     isFollowing && isFollowedBy,
	})
}

// ListFollows godoc
// @Summary List followers or following
// @Description Get paginated list of followers OR users being followed
// @Tags follows
// @Produce json
// @Param id path string true "Target user's ID (or \"me\")"
// @Param type query string true "Type: followers or following"
// @Param page query int false "Page number (default 1, max 10000)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse{data=PaginatedFollowResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/follows [get]
func (h *Handler) ListFollows(c *gin.Context) {
	viewer := currentUser(c)

	// Validate the query first: a bad request is reported even for an unknown user
	query, err := ParseFollowListQuery(c.Query("type"), c.Query("page"), c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}

	target, ok := h.loadTargetUser(c, viewer)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// People in a block relationship with the viewer are left out of the list (and its total)
	hidden, err := h.authRepo.HiddenUserIDs(ctx, viewer)
	if err != nil {
		log.Printf("follows: loading blocks for viewer failed: %v", err)
		response.InternalServerError(c, "We couldn't load this list right now. Please try again", "FETCH_FAILED")
		return
	}

	var follows []Follow
	var total int64
	if query.Type == "followers" {
		follows, total, err = h.repo.GetFollowers(ctx, target.ID, hidden, query.Page, query.Limit)
	} else {
		follows, total, err = h.repo.GetFollowing(ctx, target.ID, hidden, query.Page, query.Limit)
	}
	if err != nil {
		log.Printf("follows: listing %s of %s failed: %v", query.Type, target.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't load this list right now. Please try again", "FETCH_FAILED")
		return
	}

	// The user on the other side of each follow
	listedID := func(f Follow) primitive.ObjectID {
		if query.Type == "followers" {
			return f.FollowerID
		}
		return f.FollowingID
	}
	userIDs := make([]primitive.ObjectID, 0, len(follows))
	for _, f := range follows {
		userIDs = append(userIDs, listedID(f))
	}

	users, err := h.authRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		log.Printf("follows: loading listed users failed: %v", err)
		response.InternalServerError(c, "We couldn't load this list right now. Please try again", "FETCH_USERS_FAILED")
		return
	}
	userMap := make(map[primitive.ObjectID]*auth.User, len(users))
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}

	// Whether the viewer follows each listed user
	followingMap := map[primitive.ObjectID]bool{}
	if viewer != nil {
		if followingMap, err = h.repo.GetFollowingIDs(ctx, viewer.ID, userIDs); err != nil {
			log.Printf("follows: loading viewer follow status failed: %v", err)
			response.InternalServerError(c, "We couldn't load this list right now. Please try again", "FETCH_FAILED")
			return
		}
	}

	followUsers := make([]FollowUserResponse, 0, len(follows)) // [] rather than null when empty
	for _, f := range follows {
		user, exists := userMap[listedID(f)]
		if !exists {
			continue // account deleted while the follow was still being cleaned up
		}
		picture := user.ProfilePictureURL
		followUsers = append(followUsers, FollowUserResponse{
			ID:             user.ID,
			Username:       user.Username,
			DisplayName:    user.DisplayName,
			ProfilePicture: &picture,
			Bio:            user.Bio,
			IsFollowing:    followingMap[user.ID],
			FollowedAt:     f.CreatedAt,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))
	resp := PaginatedFollowResponse{Data: followUsers}
	resp.Pagination.Page = query.Page
	resp.Pagination.Limit = query.Limit
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = totalPages
	resp.Pagination.HasMore = query.Page < totalPages

	response.Success(c, resp)
}
