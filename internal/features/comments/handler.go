package comments

import (
	"context"
	"log"
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
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	// Parse anchor ID
	anchorIDStr := c.Param("id")
	anchorID, err := primitive.ObjectIDFromHex(anchorIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	// Bind request
	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_REQUEST")
		return
	}

	// Validate
	if err := ValidateCreateCommentRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	// Get anchor and check access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	// Check access - can comment if owner OR public/unlisted
	if !anchor.IsOwnedBy(currentUser.ID) {
		if anchor.Visibility == anchors.VisibilityPrivate {
			response.Forbidden(c, "Cannot comment on private anchor", "ACCESS_DENIED")
			return
		}
	}

	// Blocked users (either direction) can't comment on each other's anchors
	if !anchor.IsOwnedBy(currentUser.ID) {
		if blocked, err := h.authRepo.IsBlockedBetween(c.Request.Context(), currentUser, anchor.UserID); err != nil {
			response.InternalServerError(c, "Failed to create comment", "CREATE_FAILED")
			return
		} else if blocked {
			response.Forbidden(c, "You can't interact with this user", "USER_BLOCKED")
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
		response.InternalServerError(c, "Failed to create comment", "CREATE_FAILED")
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
		response.BadRequest(c, "Invalid anchor ID", "INVALID_ID")
		return
	}

	// Get anchor and check access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
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
			response.Forbidden(c, "Cannot view comments on private anchor", "ACCESS_DENIED")
			return
		}
	}

	// Bind and validate query
	var query CommentListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, "Invalid query parameters", "INVALID_QUERY")
		return
	}

	if err := ValidateCommentListQuery(&query); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}

	// Get comments
	comments, total, err := h.repo.GetCommentsByAnchor(c.Request.Context(), anchorID, query.Sort, query.Page, query.Limit)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch comments", "FETCH_FAILED")
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
		response.BadRequest(c, "Invalid comment ID", "INVALID_ID")
		return
	}

	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "Comment not found", "COMMENT_NOT_FOUND")
		return
	}

	// Check anchor access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), comment.AnchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
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
			response.Forbidden(c, "Cannot view comment on private anchor", "ACCESS_DENIED")
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
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid comment ID", "INVALID_ID")
		return
	}

	var req UpdateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_REQUEST")
		return
	}

	if err := ValidateUpdateCommentRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	// Get comment
	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "Comment not found", "COMMENT_NOT_FOUND")
		return
	}

	// Only author can edit
	if comment.UserID != currentUser.ID {
		response.Forbidden(c, "Cannot edit others' comments", "FORBIDDEN")
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
		response.InternalServerError(c, "Failed to update comment", "UPDATE_FAILED")
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

	// Editing doesn't change whether the author liked their own comment: report the real value
	hasLiked, _ := h.repo.ExistsCommentLike(c.Request.Context(), updatedComment.ID, currentUser.ID)
	commentResponse := h.buildCommentResponse(updatedComment, currentUser, hasLiked)

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
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid comment ID", "INVALID_ID")
		return
	}

	// Get comment
	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "Comment not found", "COMMENT_NOT_FOUND")
		return
	}

	// Get anchor to check ownership
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), comment.AnchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	// Permission: comment author OR anchor owner
	if comment.UserID != currentUser.ID && anchor.UserID != currentUser.ID {
		response.Forbidden(c, "Cannot delete this comment", "FORBIDDEN")
		return
	}

	// Soft delete
	if err := h.repo.SoftDeleteComment(c.Request.Context(), commentID); err != nil {
		response.InternalServerError(c, "Failed to delete comment", "DELETE_FAILED")
		return
	}

	// Keep counts in sync. anchor.commentCount counts top-level comments and replies.
	removed := 1
	if comment.ParentID != nil {
		// A reply: one fewer reply on its parent
		_ = h.repo.IncrementReplyCount(c.Request.Context(), *comment.ParentID, -1)
	} else if n, err := h.repo.SoftDeleteReplies(c.Request.Context(), comment.ID); err == nil {
		// A top-level comment: its replies go with it
		removed += int(n)
	}
	_ = h.anchorsRepo.IncrementCommentCount(c.Request.Context(), comment.AnchorID, -removed)

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
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid comment ID", "INVALID_ID")
		return
	}

	var req CommentLikeActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_REQUEST")
		return
	}

	if err := ValidateCommentLikeActionRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_ACTION")
		return
	}

	// Get comment
	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "Comment not found", "COMMENT_NOT_FOUND")
		return
	}

	// Check anchor access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), comment.AnchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.Visibility == anchors.VisibilityPrivate && !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "Cannot like comment on private anchor", "ACCESS_DENIED")
		return
	}

	var hasLiked bool

	if req.Action == "like" {
		if comment.UserID != currentUser.ID {
			if blocked, err := h.authRepo.IsBlockedBetween(c.Request.Context(), currentUser, comment.UserID); err != nil {
				response.InternalServerError(c, "Failed to like comment", "LIKE_FAILED")
				return
			} else if blocked {
				response.Forbidden(c, "You can't interact with this user", "USER_BLOCKED")
				return
			}
		}
		created, err := h.repo.CreateCommentLike(c.Request.Context(), commentID, currentUser.ID)
		if err != nil {
			response.InternalServerError(c, "Failed to like comment", "LIKE_FAILED")
			return
		}
		// Only a like that was actually inserted changes the count
		if created {
			_ = h.repo.IncrementCommentLikeCount(c.Request.Context(), commentID, 1)
		}
		hasLiked = true
	} else {
		deleted, err := h.repo.DeleteCommentLike(c.Request.Context(), commentID, currentUser.ID)
		if err != nil {
			response.InternalServerError(c, "Failed to unlike comment", "UNLIKE_FAILED")
			return
		}
		if deleted {
			_ = h.repo.IncrementCommentLikeCount(c.Request.Context(), commentID, -1)
		}
		hasLiked = false
	}

	// Get updated count (keep the old value if the re-read fails)
	likeCount := comment.LikeCount
	if updated, err := h.repo.GetCommentByID(c.Request.Context(), commentID); err == nil && updated != nil {
		likeCount = updated.LikeCount
	}

	response.Success(c, CommentLikeResponse{
		HasLiked:  hasLiked,
		LikeCount: likeCount,
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
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		response.BadRequest(c, "Invalid comment ID", "INVALID_ID")
		return
	}

	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		response.NotFound(c, "Comment not found", "COMMENT_NOT_FOUND")
		return
	}

	// Check anchor access
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), comment.AnchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	if anchor.Visibility == anchors.VisibilityPrivate && !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "Cannot access comment on private anchor", "ACCESS_DENIED")
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
		ID:         comment.ID,
		AnchorID:   comment.AnchorID,
		ParentID:   comment.ParentID,
		ReplyCount: comment.ReplyCount,
		Content:    comment.Content,
		Mentions:   comment.Mentions,
		LikeCount:  comment.LikeCount,
		IsEdited:   comment.IsEdited,
		CreatedAt:  comment.CreatedAt,
		UpdatedAt:  comment.UpdatedAt,
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

// resolveMentions turns @usernames in content into existing user IDs
func (h *Handler) resolveMentions(ctx context.Context, content string) []primitive.ObjectID {
	usernames := ExtractMentions(content)
	if len(usernames) == 0 {
		return nil
	}
	userIDMap, err := h.authRepo.GetUserIDsByUsernames(ctx, usernames)
	if err != nil {
		return nil
	}
	var ids []primitive.ObjectID
	for _, username := range usernames {
		if id, ok := userIDMap[username]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// AddReply godoc
// @Summary Reply to a comment
// @Description Replies are one level deep: replying to a reply attaches the new reply to the same
// @Description top-level comment, and the author of the comment you replied to is notified.
// @Tags comments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Comment ID being replied to"
// @Param request body CreateCommentRequest true "Reply content"
// @Success 201 {object} response.APIResponse{data=CommentResponse}
// @Failure 400 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id}/replies [post]
func (h *Handler) AddReply(c *gin.Context) {
	usr, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "UNAUTHORIZED")
		return
	}
	currentUser := usr.(*auth.User)

	targetID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid comment ID", "INVALID_ID")
		return
	}

	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_REQUEST")
		return
	}
	if err := ValidateCreateCommentRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	ctx := c.Request.Context()

	target, err := h.repo.GetCommentByID(ctx, targetID)
	if err != nil {
		response.NotFound(c, "Comment not found", "COMMENT_NOT_FOUND")
		return
	}

	anchor, err := h.anchorsRepo.GetAnchorByID(ctx, target.AnchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}
	if anchor.Visibility == anchors.VisibilityPrivate && !anchor.IsOwnedBy(currentUser.ID) {
		response.Forbidden(c, "Cannot comment on private anchor", "ACCESS_DENIED")
		return
	}

	// Blocked users (either direction) can't reply to each other or on each other's anchors
	for _, other := range []primitive.ObjectID{anchor.UserID, target.UserID} {
		if other == currentUser.ID {
			continue
		}
		if blocked, err := h.authRepo.IsBlockedBetween(ctx, currentUser, other); err != nil {
			response.InternalServerError(c, "Failed to create reply", "CREATE_FAILED")
			return
		} else if blocked {
			response.Forbidden(c, "You can't interact with this user", "USER_BLOCKED")
			return
		}
	}

	// Replies always hang off the top-level comment
	parentID := target.ID
	if target.ParentID != nil {
		parentID = *target.ParentID
	}

	reply := &Comment{
		AnchorID: target.AnchorID,
		UserID:   currentUser.ID,
		ParentID: &parentID,
		Content:  req.Content,
		Mentions: h.resolveMentions(ctx, req.Content),
	}
	if err := h.repo.CreateComment(ctx, reply); err != nil {
		response.InternalServerError(c, "Failed to create reply", "CREATE_FAILED")
		return
	}

	if err := h.repo.IncrementReplyCount(ctx, parentID, 1); err != nil {
		log.Printf("Failed to increment reply count for comment %s: %v", parentID.Hex(), err)
	}
	if err := h.anchorsRepo.IncrementCommentCount(ctx, anchor.ID, 1); err != nil {
		log.Printf("Failed to increment comment count for anchor %s: %v", anchor.ID.Hex(), err)
	}

	go func(anchorID, anchorOwner, repliedTo primitive.ObjectID) {
		_ = h.anchorsRepo.UpdateEngagementScore(context.Background(), anchorID)
		data := &notifications.CommentData{
			ID:       reply.ID,
			AnchorID: reply.AnchorID,
			Content:  reply.Content,
			Mentions: reply.Mentions,
		}
		if err := h.notificationService.CreateReplyNotifications(context.Background(), data, repliedTo, anchorOwner, currentUser); err != nil {
			log.Printf("Failed to create reply notifications: %v", err)
		}
	}(anchor.ID, anchor.UserID, target.UserID)

	response.Created(c, h.buildCommentResponse(reply, currentUser, false))
}

// ListReplies godoc
// @Summary List replies to a comment
// @Description Oldest first. Works for public/unlisted anchors without login.
// @Tags comments
// @Produce json
// @Param id path string true "Top-level comment ID"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Items per page (default 20, max 50)"
// @Success 200 {object} response.APIResponse{data=PaginatedRepliesResponse}
// @Failure 403 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id}/replies [get]
func (h *Handler) ListReplies(c *gin.Context) {
	parentID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid comment ID", "INVALID_ID")
		return
	}

	var query RepliesListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.BadRequest(c, "page must be >= 1 and limit between 1 and 50", "INVALID_QUERY")
		return
	}

	ctx := c.Request.Context()

	parent, err := h.repo.GetCommentByID(ctx, parentID)
	if err != nil {
		response.NotFound(c, "Comment not found", "COMMENT_NOT_FOUND")
		return
	}

	anchor, err := h.anchorsRepo.GetAnchorByID(ctx, parent.AnchorID)
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, "Anchor not found", "ANCHOR_NOT_FOUND")
		return
	}

	var currentUserID *primitive.ObjectID
	if usr, exists := c.Get("user"); exists {
		if user, ok := usr.(*auth.User); ok {
			currentUserID = &user.ID
		}
	}
	if anchor.Visibility == anchors.VisibilityPrivate && (currentUserID == nil || !anchor.IsOwnedBy(*currentUserID)) {
		response.Forbidden(c, "Cannot view comments on private anchor", "ACCESS_DENIED")
		return
	}

	replies, total, err := h.repo.GetReplies(ctx, parentID, query.Page, query.Limit)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch replies", "FETCH_FAILED")
		return
	}

	resp := PaginatedRepliesResponse{
		ParentID: parentID,
		Replies:  h.buildCommentListResponse(ctx, replies, currentUserID),
	}
	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))
	resp.Pagination.Page = query.Page
	resp.Pagination.Limit = query.Limit
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = totalPages
	resp.Pagination.HasMore = query.Page < totalPages

	response.Success(c, resp)
}
