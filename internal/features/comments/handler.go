package comments

import (
	"context"
	"math"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Handler struct {
	repo                *Repository
	authRepo            *auth.Repository
	anchorsRepo         *anchors.Repository
	notificationService *notifications.Service
	config              *config.Config
}

func NewHandler(repo *Repository, authRepo *auth.Repository, anchorsRepo *anchors.Repository, notificationService *notifications.Service, cfg *config.Config) *Handler {
	return &Handler{
		repo:                repo,
		authRepo:            authRepo,
		anchorsRepo:         anchorsRepo,
		notificationService: notificationService,
		config:              cfg,
	}
}

// AddComment godoc
// @Summary Add comment to anchor
// @Description Add a new comment with @mention support
// @Tags comments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Anchor ID"
// @Param request body CreateCommentRequest true "Comment content"
// @Success 201 {object} response.APIResponse{data=CommentResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/comments [post]
func (h *Handler) AddComment(c *gin.Context) {
	// Get current user
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := usr.(*auth.User)

	// Parse anchor ID
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	// Bind request
	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", "Invalid request format")
		return
	}

	// Validate
	if err := ValidateCreateCommentRequest(&req); err != nil {
		response.BadRequest(c, "VALIDATION_FAILED", err.Error())
		return
	}

	// Get anchor and check access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	if anchor.DeletedAt != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Check access - can comment if owner OR public/unlisted
	if !anchor.IsOwnedBy(currentUser.ID) {
		if anchor.Visibility == anchors.VisibilityPrivate {
			response.Forbidden(c, "ACCESS_DENIED", "Cannot comment on private anchor")
			return
		}
	}

	// Extract mentions
	mentionUsernames := ExtractMentions(req.Content)

	// Validate mentioned users and get their IDs
	var mentionIDs []primitive.ObjectID
	if len(mentionUsernames) > 0 {
		userIDMap, err := h.authRepo.GetUserIDsByUsernames(c.Request.Context(), mentionUsernames)
		if err == nil {
			for _, username := range mentionUsernames {
				if id, ok := userIDMap[username]; ok {
					mentionIDs = append(mentionIDs, id)
				}
			}
		}
	}

	// Create comment
	comment := &Comment{
		AnchorID: anchorID,
		UserID:   currentUser.ID,
		Content:  req.Content,
		Mentions: mentionIDs,
	}

	if err := h.repo.CreateComment(c.Request.Context(), comment); err != nil {
		response.InternalServerError(c, "CREATE_FAILED", "Failed to create comment")
		return
	}

	// Increment anchor comment count
	_ = h.anchorsRepo.IncrementCommentCount(c.Request.Context(), anchorID, 1)

	// Update engagement score (async)
	go func() {
		_ = h.anchorsRepo.UpdateEngagementScore(context.Background(), anchorID)
	}()

	// Create notifications (async)
	go func() {
		commentData := &notifications.CommentData{
			ID:       comment.ID,
			AnchorID: comment.AnchorID,
			Content:  comment.Content,
			Mentions: comment.Mentions,
		}
		_ = h.notificationService.CreateCommentNotifications(context.Background(), commentData, anchor.ID, anchor.UserID, currentUser)
	}()

	// Build response
	commentResponse := h.buildCommentResponse(comment, currentUser, false)

	response.Created(c, commentResponse)
}

// ListComments godoc
// @Summary List comments for anchor
// @Description Get paginated list of comments
// @Tags comments
// @Produce json
// @Param id path string true "Anchor ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Param sort query string false "Sort: newest, oldest, top (default newest)"
// @Success 200 {object} response.APIResponse{data=PaginatedCommentsResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/comments [get]
func (h *Handler) ListComments(c *gin.Context) {
	// Parse anchor ID
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid anchor ID")
		return
	}

	// Get anchor and check access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	if anchor.DeletedAt != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Get current user if authenticated
	var currentUserID *primitive.ObjectID
	if usr, exists := c.Get("user"); exists {
		if user, ok := usr.(*auth.User); ok {
			currentUserID = &user.ID
		}
	}

	// Check access for private anchor
	if anchor.Visibility == anchors.VisibilityPrivate {
		if currentUserID == nil || !anchor.IsOwnedBy(*currentUserID) {
			response.Forbidden(c, "ACCESS_DENIED", "Cannot view comments on private anchor")
			return
		}
	}

	// Bind and validate query
	var query CommentListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", "Invalid query parameters")
		return
	}

	if err := ValidateCommentListQuery(&query); err != nil {
		response.BadRequest(c, "INVALID_QUERY", err.Error())
		return
	}

	// Get comments
	comments, total, err := h.repo.GetCommentsByAnchor(c.Request.Context(), anchorID, query.Sort, query.Page, query.Limit)
	if err != nil {
		response.InternalServerError(c, "FETCH_FAILED", "Failed to fetch comments")
		return
	}

	// Build response
	commentResponses := h.buildCommentListResponse(c.Request.Context(), comments, currentUserID)

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))

	resp := PaginatedCommentsResponse{
		Comments: commentResponses,
		Meta: CommentListMeta{
			Sort:     query.Sort,
			AnchorID: anchorID,
		},
	}
	resp.Pagination.Page = query.Page
	resp.Pagination.Limit = query.Limit
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = totalPages
	resp.Pagination.HasMore = query.Page < totalPages

	response.Success(c, resp)
}

// GetComment godoc
// @Summary Get single comment
// @Description Get comment by ID
// @Tags comments
// @Produce json
// @Param id path string true "Comment ID"
// @Success 200 {object} response.APIResponse{data=CommentResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id} [get]
func (h *Handler) GetComment(c *gin.Context) {
	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid comment ID")
		return
	}

	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "COMMENT_NOT_FOUND", "Comment not found")
		return
	}

	// Check anchor access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), comment.AnchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	var currentUserID *primitive.ObjectID
	if usr, exists := c.Get("user"); exists {
		if user, ok := usr.(*auth.User); ok {
			currentUserID = &user.ID
		}
	}

	if anchor.Visibility == anchors.VisibilityPrivate {
		if currentUserID == nil || !anchor.IsOwnedBy(*currentUserID) {
			response.Forbidden(c, "ACCESS_DENIED", "Cannot view comment on private anchor")
			return
		}
	}

	// Get author
	author, _ := h.authRepo.GetUserByObjectID(c.Request.Context(), comment.UserID)

	// Check if liked
	hasLiked := false
	if currentUserID != nil {
		hasLiked, _ = h.repo.ExistsCommentLike(c.Request.Context(), commentID, *currentUserID)
	}

	commentResponse := h.buildCommentResponseWithAuthor(comment, author, hasLiked)

	response.Success(c, commentResponse)
}

// EditComment godoc
// @Summary Edit comment
// @Description Edit own comment
// @Tags comments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Comment ID"
// @Param request body UpdateCommentRequest true "Updated content"
// @Success 200 {object} response.APIResponse{data=CommentResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id} [patch]
func (h *Handler) EditComment(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := usr.(*auth.User)

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid comment ID")
		return
	}

	var req UpdateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", "Invalid request format")
		return
	}

	if err := ValidateUpdateCommentRequest(&req); err != nil {
		response.BadRequest(c, "VALIDATION_FAILED", err.Error())
		return
	}

	// Get comment
	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "COMMENT_NOT_FOUND", "Comment not found")
		return
	}

	// Only author can edit
	if comment.UserID != currentUser.ID {
		response.Forbidden(c, "FORBIDDEN", "Cannot edit others' comments")
		return
	}

	// Store old mentions for comparison
	oldMentions := comment.Mentions

	// Extract new mentions
	mentionUsernames := ExtractMentions(req.Content)
	var newMentionIDs []primitive.ObjectID
	if len(mentionUsernames) > 0 {
		userIDMap, err := h.authRepo.GetUserIDsByUsernames(c.Request.Context(), mentionUsernames)
		if err == nil {
			for _, username := range mentionUsernames {
				if id, ok := userIDMap[username]; ok {
					newMentionIDs = append(newMentionIDs, id)
				}
			}
		}
	}

	// Update comment
	err = h.repo.UpdateComment(c.Request.Context(), commentID, map[string]interface{}{
		"content":  req.Content,
		"mentions": newMentionIDs,
		"isEdited": true,
	})
	if err != nil {
		response.InternalServerError(c, "UPDATE_FAILED", "Failed to update comment")
		return
	}

	// Create notifications for NEW mentions only (async)
	go func() {
		commentData := &notifications.CommentData{
			ID:       commentID,
			AnchorID: comment.AnchorID,
			Content:  req.Content,
			Mentions: newMentionIDs,
		}
		_ = h.notificationService.CreateEditCommentNotifications(context.Background(), commentData, oldMentions, currentUser)
	}()

	// Get updated comment
	updatedComment, _ := h.repo.GetCommentByID(c.Request.Context(), commentID)

	commentResponse := h.buildCommentResponse(updatedComment, currentUser, false)

	response.Success(c, commentResponse)
}

// DeleteComment godoc
// @Summary Delete comment
// @Description Delete own comment or any comment on own anchor
// @Tags comments
// @Produce json
// @Security BearerAuth
// @Param id path string true "Comment ID"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id} [delete]
func (h *Handler) DeleteComment(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := usr.(*auth.User)

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid comment ID")
		return
	}

	// Get comment
	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "COMMENT_NOT_FOUND", "Comment not found")
		return
	}

	// Get anchor to check ownership
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), comment.AnchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	// Permission: comment author OR anchor owner
	if comment.UserID != currentUser.ID && anchor.UserID != currentUser.ID {
		response.Forbidden(c, "FORBIDDEN", "Cannot delete this comment")
		return
	}

	// Soft delete
	if err := h.repo.SoftDeleteComment(c.Request.Context(), commentID); err != nil {
		response.InternalServerError(c, "DELETE_FAILED", "Failed to delete comment")
		return
	}

	// Decrement comment count
	_ = h.anchorsRepo.IncrementCommentCount(c.Request.Context(), comment.AnchorID, -1)

	// Update engagement score (async)
	go func() {
		_ = h.anchorsRepo.UpdateEngagementScore(context.Background(), comment.AnchorID)
	}()

	response.Success(c, gin.H{"message": "Comment deleted successfully"})
}

// LikeComment godoc
// @Summary Like or unlike comment
// @Description Toggle like on a comment
// @Tags comments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Comment ID"
// @Param request body CommentLikeActionRequest true "Like action"
// @Success 200 {object} response.APIResponse{data=CommentLikeResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id}/like [post]
func (h *Handler) LikeComment(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := usr.(*auth.User)

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid comment ID")
		return
	}

	var req CommentLikeActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "INVALID_REQUEST", "Invalid request format")
		return
	}

	if err := ValidateCommentLikeActionRequest(&req); err != nil {
		response.BadRequest(c, "INVALID_ACTION", err.Error())
		return
	}

	// Get comment
	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "COMMENT_NOT_FOUND", "Comment not found")
		return
	}

	// Check anchor access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), comment.AnchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	if anchor.Visibility == anchors.VisibilityPrivate && !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "ACCESS_DENIED", "Cannot like comment on private anchor")
		return
	}

	var hasLiked bool

	if req.Action == "like" {
		err = h.repo.CreateCommentLike(c.Request.Context(), commentID, currentUser.ID)
		if err != nil {
			response.InternalServerError(c, "LIKE_FAILED", "Failed to like comment")
			return
		}
		_ = h.repo.IncrementCommentLikeCount(c.Request.Context(), commentID, 1)
		hasLiked = true
	} else {
		err = h.repo.DeleteCommentLike(c.Request.Context(), commentID, currentUser.ID)
		if err != nil {
			response.InternalServerError(c, "UNLIKE_FAILED", "Failed to unlike comment")
			return
		}
		_ = h.repo.IncrementCommentLikeCount(c.Request.Context(), commentID, -1)
		hasLiked = false
	}

	// Get updated comment
	updatedComment, _ := h.repo.GetCommentByID(c.Request.Context(), commentID)

	response.Success(c, CommentLikeResponse{
		HasLiked:  hasLiked,
		LikeCount: updatedComment.LikeCount,
	})
}

// GetCommentLikeStatus godoc
// @Summary Get comment like status
// @Description Check if user has liked a comment
// @Tags comments
// @Produce json
// @Security BearerAuth
// @Param id path string true "Comment ID"
// @Success 200 {object} response.APIResponse{data=CommentLikeResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id}/like/status [get]
func (h *Handler) GetCommentLikeStatus(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "UNAUTHORIZED", "Authentication required")
		return
	}
	currentUser := usr.(*auth.User)

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		response.BadRequest(c, "INVALID_ID", "Invalid comment ID")
		return
	}

	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "COMMENT_NOT_FOUND", "Comment not found")
		return
	}

	// Check anchor access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), comment.AnchorID)
	if err != nil {
		response.NotFound(c, "ANCHOR_NOT_FOUND", "Anchor not found")
		return
	}

	if anchor.Visibility == anchors.VisibilityPrivate && !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "ACCESS_DENIED", "Cannot access comment on private anchor")
		return
	}

	hasLiked, _ := h.repo.ExistsCommentLike(c.Request.Context(), commentID, currentUser.ID)

	response.Success(c, CommentLikeResponse{
		HasLiked:  hasLiked,
		LikeCount: comment.LikeCount,
	})
}

// Helper methods

func (h *Handler) buildCommentResponse(comment *Comment, author *auth.User, hasLiked bool) CommentResponse {
	var profilePic *string
	if author.ProfilePictureURL != "" {
		profilePic = &author.ProfilePictureURL
	}

	return CommentResponse{
		ID:        comment.ID,
		AnchorID:  comment.AnchorID,
		Content:   comment.Content,
		Mentions:  comment.Mentions,
		LikeCount: comment.LikeCount,
		IsEdited:  comment.IsEdited,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
		Author: CommentAuthor{
			ID:             author.ID,
			Username:       author.Username,
			DisplayName:    author.DisplayName,
			ProfilePicture: profilePic,
			IsVerified:     author.IsVerified,
		},
		Engagement: CommentEngagement{
			HasLiked: hasLiked,
		},
	}
}

func (h *Handler) buildCommentResponseWithAuthor(comment *Comment, author *auth.User, hasLiked bool) CommentResponse {
	if author == nil {
		author = &auth.User{
			ID:          comment.UserID,
			Username:    "deleted",
			DisplayName: "Deleted User",
		}
	}
	return h.buildCommentResponse(comment, author, hasLiked)
}

func (h *Handler) buildCommentListResponse(ctx context.Context, comments []Comment, currentUserID *primitive.ObjectID) []CommentResponse {
	if len(comments) == 0 {
		return []CommentResponse{}
	}

	// Collect author IDs
	authorIDs := make([]primitive.ObjectID, len(comments))
	commentIDs := make([]primitive.ObjectID, len(comments))
	for i, c := range comments {
		authorIDs[i] = c.UserID
		commentIDs[i] = c.ID
	}

	// Batch fetch authors
	authors, _ := h.authRepo.GetUsersByIDs(ctx, authorIDs)
	authorMap := make(map[primitive.ObjectID]*auth.User)
	for i := range authors {
		authorMap[authors[i].ID] = &authors[i]
	}

	// Batch fetch like status
	var likedMap map[primitive.ObjectID]bool
	if currentUserID != nil {
		likedMap, _ = h.repo.GetUserLikedComments(ctx, *currentUserID, commentIDs)
	}

	// Build responses
	responses := make([]CommentResponse, len(comments))
	for i, comment := range comments {
		author := authorMap[comment.UserID]
		hasLiked := false
		if likedMap != nil {
			hasLiked = likedMap[comment.ID]
		}
		responses[i] = h.buildCommentResponseWithAuthor(&comment, author, hasLiked)
	}

	return responses
}
