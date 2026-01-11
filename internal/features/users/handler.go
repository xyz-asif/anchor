package users

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/likes"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// FollowService defines the interface for follow operations to avoid import cycle
type FollowService interface {
	GetFollowStatus(ctx context.Context, userID, targetID primitive.ObjectID) (bool, bool, error)
}

type Handler struct {
	authRepo      *auth.Repository
	likesRepo     *likes.Repository
	anchorsRepo   *anchors.Repository
	followService FollowService
}

func NewHandler(authRepo *auth.Repository, likesRepo *likes.Repository, anchorsRepo *anchors.Repository, followService FollowService) *Handler {
	return &Handler{
		authRepo:      authRepo,
		likesRepo:     likesRepo,
		anchorsRepo:   anchorsRepo,
		followService: followService,
	}
}

// GetUserByUsername godoc
// @Summary Get user profile by username
// @Description Get public profile of a user by their username
// @Tags users
// @Produce json
// @Param username path string true "Username"
// @Success 200 {object} response.APIResponse{data=auth.PublicProfileResponse}
// @Failure 404 {object} response.APIResponse
// @Router /users/username/{username} [get]
func (h *Handler) GetUserByUsername(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		response.BadRequest(c, "Username is required", "INVALID_USERNAME")
		return
	}

	// Normalize username (lowercase)
	username = strings.ToLower(strings.TrimSpace(username))

	ctx := c.Request.Context()

	// Get user by username
	user, err := h.authRepo.GetUserByUsername(ctx, username)
	if err != nil || user == nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	// Check follow status if authenticated
	var isFollowing, isFollowedBy, isMutual bool
	if val, exists := c.Get("user"); exists {
		if currentUser, ok := val.(*auth.User); ok {
			if currentUser.ID != user.ID && h.followService != nil {
				isFollowing, isFollowedBy, err = h.followService.GetFollowStatus(ctx, currentUser.ID, user.ID)
				if err == nil {
					isMutual = isFollowing && isFollowedBy
				}
			}
		}
	}

	resp := auth.PublicProfileResponse{
		ID:                user.ID,
		Username:          user.Username,
		DisplayName:       user.DisplayName,
		Bio:               user.Bio,
		ProfilePictureURL: user.ProfilePictureURL,
		CoverImageURL:     user.CoverImageURL,
		FollowerCount:     user.FollowerCount,
		FollowingCount:    user.FollowingCount,
		AnchorCount:       user.AnchorCount,
		IsVerified:        user.IsVerified,
		JoinedAt:          user.JoinedAt,
		IsFollowing:       isFollowing,
		IsFollowedBy:      isFollowedBy,
		IsMutual:          isMutual,
	}

	response.Success(c, resp)
}

// GetUserLikes godoc
// @Summary Get user's liked anchors
// @Description Get paginated list of anchors liked by a user
// @Tags users
// @Produce json
// @Param id path string true "User ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/likes [get]
func (h *Handler) GetUserLikes(c *gin.Context) {
	userIDStr := c.Param("id")

	// Handle "me" case
	if userIDStr == "me" {
		currentUser, exists := c.Get("user")
		if !exists {
			response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
			return
		}
		user := currentUser.(*auth.User)
		userIDStr = user.ID.Hex()
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	ctx := c.Request.Context()

	// Check user exists
	user, err := h.authRepo.GetUserByID(ctx, userIDStr)
	if err != nil || user == nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	// Get likes
	likesList, total, err := h.likesRepo.GetUserLikedAnchorsPaginated(ctx, userID, page, limit)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch liked anchors", "FETCH_FAILED")
		return
	}

	// Collect anchor IDs
	anchorIDs := make([]primitive.ObjectID, len(likesList))
	for i, like := range likesList {
		anchorIDs[i] = like.AnchorID
	}

	// Batch fetch anchors
	anchorsList, _ := h.anchorsRepo.GetAnchorsByIDs(ctx, anchorIDs)
	anchorsMap := make(map[primitive.ObjectID]*anchors.Anchor)
	authorIDs := make([]primitive.ObjectID, 0)
	for i := range anchorsList {
		anchorsMap[anchorsList[i].ID] = &anchorsList[i]
		authorIDs = append(authorIDs, anchorsList[i].UserID)
	}

	// Batch fetch authors
	authorsList, _ := h.authRepo.GetUsersByIDs(ctx, authorIDs)
	authorsMap := make(map[primitive.ObjectID]*auth.User)
	for i := range authorsList {
		authorsMap[authorsList[i].ID] = &authorsList[i]
	}

	// Build response
	items := make([]gin.H, 0)
	for _, like := range likesList {
		anchor, ok := anchorsMap[like.AnchorID]
		if !ok || anchor == nil || anchor.DeletedAt != nil {
			continue
		}

		// Get author
		var authorInfo gin.H
		if author, ok := authorsMap[anchor.UserID]; ok {
			authorInfo = gin.H{
				"id":                author.ID,
				"username":          author.Username,
				"displayName":       author.DisplayName,
				"profilePictureUrl": author.ProfilePictureURL,
			}
		}

		items = append(items, gin.H{
			"id":          anchor.ID,
			"title":       anchor.Title,
			"description": anchor.Description,
			"itemCount":   anchor.ItemCount,
			"likeCount":   anchor.LikeCount,
			"likedAt":     like.CreatedAt,
			"author":      authorInfo,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	response.Success(c, gin.H{
		"data": items,
		"pagination": gin.H{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": totalPages,
			"hasMore":    page < totalPages,
		},
	})
}

// GetUserClones godoc
// @Summary Get user's cloned anchors
// @Description Get paginated list of anchors cloned by a user
// @Tags users
// @Produce json
// @Param id path string true "User ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /users/{id}/clones [get]
func (h *Handler) GetUserClones(c *gin.Context) {
	userIDStr := c.Param("id")

	// Handle "me" case
	if userIDStr == "me" {
		currentUser, exists := c.Get("user")
		if !exists {
			response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
			return
		}
		user := currentUser.(*auth.User)
		userIDStr = user.ID.Hex()
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	ctx := c.Request.Context()

	// Check user exists
	user, err := h.authRepo.GetUserByID(ctx, userIDStr)
	if err != nil || user == nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	// Get cloned anchors (isClone=true, userId=userID)
	clones, total, err := h.anchorsRepo.GetUserClonedAnchors(ctx, userID, page, limit)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch cloned anchors", "FETCH_FAILED")
		return
	}

	// Collect original anchor IDs for author info
	originalIDs := make([]primitive.ObjectID, 0)
	for _, clone := range clones {
		if clone.ClonedFromAnchorID != nil {
			originalIDs = append(originalIDs, *clone.ClonedFromAnchorID)
		}
	}

	// Batch fetch original anchors
	originals, _ := h.anchorsRepo.GetAnchorsByIDs(ctx, originalIDs)
	originalsMap := make(map[primitive.ObjectID]*anchors.Anchor)
	authorIDs := make([]primitive.ObjectID, 0)
	for i := range originals {
		originalsMap[originals[i].ID] = &originals[i]
		authorIDs = append(authorIDs, originals[i].UserID)
	}

	// Batch fetch authors
	authors, _ := h.authRepo.GetUsersByIDs(ctx, authorIDs)
	authorsMap := make(map[primitive.ObjectID]*auth.User)
	for i := range authors {
		authorsMap[authors[i].ID] = &authors[i]
	}

	// Build response
	items := make([]gin.H, 0)
	for _, clone := range clones {
		item := gin.H{
			"id":                 clone.ID,
			"title":              clone.Title,
			"description":        clone.Description,
			"itemCount":          clone.ItemCount,
			"likeCount":          clone.LikeCount,
			"isClone":            true,
			"clonedFromAnchorId": clone.ClonedFromAnchorID,
			"clonedAt":           clone.CreatedAt,
		}

		// Add original author info if available
		if clone.ClonedFromAnchorID != nil {
			if orig, ok := originalsMap[*clone.ClonedFromAnchorID]; ok {
				if author, ok := authorsMap[orig.UserID]; ok {
					item["originalAuthor"] = gin.H{
						"id":          author.ID,
						"username":    author.Username,
						"displayName": author.DisplayName,
					}
				}
			}
		}

		items = append(items, item)
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	response.Success(c, gin.H{
		"data": items,
		"pagination": gin.H{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": totalPages,
			"hasMore":    page < totalPages,
		},
	})
}
