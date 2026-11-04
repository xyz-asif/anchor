package comments

import (
	"context"
	"errors"
	"log"
	"math"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/features/notifications"
	"github.com/xyz-asif/gotodo/internal/pkg/bind"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	msgAnchorNotFound  = "This anchor doesn't exist or has been deleted"
	msgCommentNotFound = "This comment doesn't exist or has been deleted"
	msgTryAgain        = "Something went wrong on our side. Please try again in a moment"
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

// ---------- shared loading and access rules ----------

// currentUser returns the authenticated user, or nil when logged out
func currentUser(c *gin.Context) *auth.User {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return user
		}
	}
	return nil
}

func requireUser(c *gin.Context, action string) (*auth.User, bool) {
	if user := currentUser(c); user != nil {
		return user, true
	}
	response.Unauthorized(c, "Please sign in to "+action, "UNAUTHORIZED")
	return nil, false
}

func parseID(c *gin.Context, what string) (primitive.ObjectID, bool) {
	raw := c.Param("id")
	id, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		response.BadRequest(c, `"`+raw+`" isn't a valid `+what+` ID`, "INVALID_ID")
		return primitive.NilObjectID, false
	}
	return id, true
}

// loadAnchorByID loads an anchor; deleted anchors count as not found
func (h *Handler) loadAnchorByID(c *gin.Context, anchorID primitive.ObjectID, notFoundMsg, notFoundCode string) (*anchors.Anchor, bool) {
	anchor, err := h.anchorsRepo.GetAnchorByID(c.Request.Context(), anchorID)
	if err != nil && !errors.Is(err, anchors.ErrAnchorNotFound) {
		log.Printf("comments: loading anchor %s failed: %v", anchorID.Hex(), err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return nil, false
	}
	if err != nil || anchor.DeletedAt != nil {
		response.NotFound(c, notFoundMsg, notFoundCode)
		return nil, false
	}
	return anchor, true
}

// canView reports whether viewer (nil = anonymous) may see the anchor and its comments: not
// someone else's private anchor, and not owned by someone who blocked the viewer.
func (h *Handler) canView(ctx context.Context, anchor *anchors.Anchor, viewer *auth.User) (bool, error) {
	var viewerID *primitive.ObjectID
	if viewer != nil {
		viewerID = &viewer.ID
	}
	if !anchor.IsAccessibleBy(viewerID) {
		return false, nil
	}
	if viewer == nil || viewer.ID == anchor.UserID {
		return true, nil
	}
	blocked, err := h.authRepo.UserHasBlocked(ctx, anchor.UserID, viewer.ID)
	return !blocked, err
}

// loadViewableAnchor loads the anchor named by :id that the viewer may see. Everything the
// viewer may not see answers 404, so private anchors and blocks aren't revealed.
func (h *Handler) loadViewableAnchor(c *gin.Context, viewer *auth.User) (*anchors.Anchor, bool) {
	anchorID, ok := parseID(c, "anchor")
	if !ok {
		return nil, false
	}
	anchor, ok := h.loadAnchorByID(c, anchorID, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
	if !ok {
		return nil, false
	}
	visible, err := h.canView(c.Request.Context(), anchor, viewer)
	if err != nil {
		log.Printf("comments: access check failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return nil, false
	}
	if !visible {
		response.NotFound(c, msgAnchorNotFound, "ANCHOR_NOT_FOUND")
		return nil, false
	}
	return anchor, true
}

// loadComment loads the (non-deleted) comment named by :id
func (h *Handler) loadComment(c *gin.Context) (*Comment, bool) {
	commentID, ok := parseID(c, "comment")
	if !ok {
		return nil, false
	}
	comment, err := h.repo.GetCommentByID(c.Request.Context(), commentID)
	if err != nil {
		if errors.Is(err, ErrCommentNotFound) {
			response.NotFound(c, msgCommentNotFound, "COMMENT_NOT_FOUND")
		} else {
			log.Printf("comments: loading comment %s failed: %v", commentID.Hex(), err)
			response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		}
		return nil, false
	}
	return comment, true
}

// loadViewableComment loads a comment the viewer may see: its anchor is viewable, and neither
// side of a block relationship between viewer and author is involved (reported as 404).
func (h *Handler) loadViewableComment(c *gin.Context, viewer *auth.User) (*Comment, *anchors.Anchor, bool) {
	comment, ok := h.loadComment(c)
	if !ok {
		return nil, nil, false
	}
	anchor, ok := h.loadAnchorByID(c, comment.AnchorID, msgCommentNotFound, "COMMENT_NOT_FOUND")
	if !ok {
		return nil, nil, false
	}
	ctx := c.Request.Context()
	visible, err := h.canView(ctx, anchor, viewer)
	if err == nil && visible && viewer != nil && viewer.ID != comment.UserID {
		var blocked bool
		blocked, err = h.authRepo.IsBlockedBetween(ctx, viewer, comment.UserID)
		visible = !blocked
	}
	if err != nil {
		log.Printf("comments: access check failed: %v", err)
		response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
		return nil, nil, false
	}
	if !visible {
		response.NotFound(c, msgCommentNotFound, "COMMENT_NOT_FOUND")
		return nil, nil, false
	}
	return comment, anchor, true
}

// resolveMentions turns @usernames into user IDs, keeping only people who can see the anchor
// and who aren't in a block relationship with the author. Mentioning someone on a private
// anchor used to send them a preview of its private content.
func (h *Handler) resolveMentions(ctx context.Context, content string, anchor *anchors.Anchor, author *auth.User) ([]primitive.ObjectID, error) {
	usernames := ExtractMentions(content)
	if len(usernames) == 0 {
		return []primitive.ObjectID{}, nil
	}
	userIDMap, err := h.authRepo.GetUserIDsByUsernames(ctx, usernames)
	if err != nil {
		return nil, err
	}
	hidden, err := h.authRepo.HiddenUserIDs(ctx, author)
	if err != nil {
		return nil, err
	}
	excluded := map[primitive.ObjectID]bool{}
	for _, id := range hidden {
		excluded[id] = true
	}
	if anchor.UserID != author.ID {
		owner, err := h.authRepo.GetUserByObjectID(ctx, anchor.UserID)
		if err != nil && !errors.Is(err, auth.ErrUserNotFound) {
			return nil, err
		}
		if owner != nil {
			for _, id := range owner.BlockedUsers { // they can't open the anchor
				excluded[id] = true
			}
		}
	}

	ids := []primitive.ObjectID{}
	for _, username := range usernames {
		id, ok := userIDMap[username]
		if !ok || excluded[id] {
			continue
		}
		if anchor.Visibility == anchors.VisibilityPrivate && id != anchor.UserID {
			continue // only the owner can see a private anchor
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ---------- endpoints ----------

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
	me, ok := requireUser(c, "comment")
	if !ok {
		return
	}
	var req CreateCommentRequest
	if !bind.JSON(c, &req) {
		return
	}
	if err := ValidateCreateCommentRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	anchor, ok := h.loadViewableAnchor(c, me)
	if !ok {
		return
	}
	if anchor.UserID != me.ID && me.HasBlocked(anchor.UserID) {
		response.Forbidden(c, "You've blocked the owner of this anchor. Unblock them first if you want to comment", "USER_BLOCKED")
		return
	}
	ctx := c.Request.Context()

	mentions, err := h.resolveMentions(ctx, req.Content, anchor, me)
	if err != nil {
		log.Printf("comments: resolving mentions failed: %v", err)
		response.InternalServerError(c, "We couldn't post your comment right now. Please try again", "CREATE_FAILED")
		return
	}

	comment := &Comment{
		AnchorID: anchor.ID,
		UserID:   me.ID,
		Content:  req.Content,
		Mentions: mentions,
	}
	if err := h.repo.CreateComment(ctx, comment); err != nil {
		log.Printf("comments: create failed: %v", err)
		response.InternalServerError(c, "We couldn't post your comment right now. Please try again", "CREATE_FAILED")
		return
	}

	h.afterCommentCountChange(ctx, anchor.ID, 1)

	if h.notificationService != nil {
		go func(anchorID, ownerID primitive.ObjectID) {
			data := &notifications.CommentData{
				ID:       comment.ID,
				AnchorID: comment.AnchorID,
				Content:  comment.Content,
				Mentions: comment.Mentions,
			}
			if err := h.notificationService.CreateCommentNotifications(context.Background(), data, anchorID, ownerID, me); err != nil {
				log.Printf("comments: notifications failed: %v", err)
			}
		}(anchor.ID, anchor.UserID)
	}

	response.Created(c, h.buildCommentResponse(comment, me, false))
}

// afterCommentCountChange keeps the anchor's comment count and ranking score in sync
func (h *Handler) afterCommentCountChange(ctx context.Context, anchorID primitive.ObjectID, delta int) {
	if err := h.anchorsRepo.IncrementCommentCount(ctx, anchorID, delta); err != nil {
		log.Printf("comments: commentCount %+d for %s failed: %v", delta, anchorID.Hex(), err)
	}
	go func() {
		if err := h.anchorsRepo.UpdateEngagementScore(context.Background(), anchorID); err != nil {
			log.Printf("comments: engagement score for %s failed: %v", anchorID.Hex(), err)
		}
	}()
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
// @Failure 404 {object} response.APIResponse
// @Router /anchors/{id}/comments [get]
func (h *Handler) ListComments(c *gin.Context) {
	query, err := ParseCommentListQuery(c.Query("page"), c.Query("limit"), c.Query("sort"))
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
	failMsg := "We couldn't load the comments right now. Please try again"

	// Comments by people in a block relationship with the viewer are left out (and not counted)
	hidden, err := h.authRepo.HiddenUserIDs(ctx, viewer)
	if err != nil {
		log.Printf("comments: loading blocks failed: %v", err)
		response.InternalServerError(c, failMsg, "FETCH_FAILED")
		return
	}

	comments, total, err := h.repo.GetCommentsByAnchor(ctx, anchor.ID, hidden, query.Sort, query.Page, query.Limit)
	if err != nil {
		log.Printf("comments: listing comments of %s failed: %v", anchor.ID.Hex(), err)
		response.InternalServerError(c, failMsg, "FETCH_FAILED")
		return
	}

	commentResponses, err := h.buildCommentListResponse(ctx, comments, viewer)
	if err != nil {
		log.Printf("comments: building comment list failed: %v", err)
		response.InternalServerError(c, failMsg, "FETCH_FAILED")
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(query.Limit)))
	resp := PaginatedCommentsResponse{
		Comments: commentResponses,
		Meta:     CommentListMeta{Sort: query.Sort, AnchorID: anchor.ID},
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
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id} [get]
func (h *Handler) GetComment(c *gin.Context) {
	viewer := currentUser(c)
	comment, _, ok := h.loadViewableComment(c, viewer)
	if !ok {
		return
	}
	list, err := h.buildCommentListResponse(c.Request.Context(), []Comment{*comment}, viewer)
	if err != nil {
		log.Printf("comments: building comment failed: %v", err)
		response.InternalServerError(c, "We couldn't load this comment right now. Please try again", "FETCH_FAILED")
		return
	}
	response.Success(c, list[0])
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
	me, ok := requireUser(c, "edit comments")
	if !ok {
		return
	}
	var req UpdateCommentRequest
	if !bind.JSON(c, &req) {
		return
	}
	if err := ValidateUpdateCommentRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	comment, anchor, ok := h.loadViewableComment(c, me)
	if !ok {
		return
	}
	if comment.UserID != me.ID {
		response.Forbidden(c, "You can only edit your own comments", "FORBIDDEN")
		return
	}
	ctx := c.Request.Context()
	failMsg := "We couldn't save your changes right now. Please try again"

	// Nothing changed: don't mark the comment as edited or re-send mentions
	if req.Content == comment.Content {
		hasLiked, _ := h.repo.ExistsCommentLike(ctx, comment.ID, me.ID)
		response.Success(c, h.buildCommentResponse(comment, me, hasLiked))
		return
	}

	newMentions, err := h.resolveMentions(ctx, req.Content, anchor, me)
	if err != nil {
		log.Printf("comments: resolving mentions failed: %v", err)
		response.InternalServerError(c, failMsg, "UPDATE_FAILED")
		return
	}

	if err := h.repo.UpdateComment(ctx, comment.ID, map[string]interface{}{
		"content":  req.Content,
		"mentions": newMentions,
		"isEdited": true,
	}); err != nil {
		if errors.Is(err, ErrCommentNotFound) {
			response.NotFound(c, "This comment was deleted while you were editing it", "COMMENT_NOT_FOUND")
			return
		}
		log.Printf("comments: update %s failed: %v", comment.ID.Hex(), err)
		response.InternalServerError(c, failMsg, "UPDATE_FAILED")
		return
	}

	// Only people mentioned for the first time are notified
	if h.notificationService != nil {
		oldMentions := comment.Mentions
		go func(commentID, anchorID primitive.ObjectID, content string) {
			data := &notifications.CommentData{ID: commentID, AnchorID: anchorID, Content: content, Mentions: newMentions}
			if err := h.notificationService.CreateEditCommentNotifications(context.Background(), data, oldMentions, me); err != nil {
				log.Printf("comments: edit notifications failed: %v", err)
			}
		}(comment.ID, comment.AnchorID, req.Content)
	}

	updated, err := h.repo.GetCommentByID(ctx, comment.ID)
	if err != nil {
		// The edit is saved; answer with what we know rather than failing (or crashing)
		log.Printf("comments: re-reading %s failed: %v", comment.ID.Hex(), err)
		updated = comment
		updated.Content, updated.Mentions, updated.IsEdited = req.Content, newMentions, true
	}
	hasLiked, _ := h.repo.ExistsCommentLike(ctx, comment.ID, me.ID)
	response.Success(c, h.buildCommentResponse(updated, me, hasLiked))
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
	me, ok := requireUser(c, "delete comments")
	if !ok {
		return
	}
	comment, ok := h.loadComment(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	// Authors can always delete their own comments (even if the anchor went private, was
	// deleted, or its owner blocked them). Anchor owners can delete any comment on their anchor.
	if comment.UserID != me.ID {
		anchor, err := h.anchorsRepo.GetAnchorByID(ctx, comment.AnchorID)
		if err != nil && !errors.Is(err, anchors.ErrAnchorNotFound) {
			log.Printf("comments: loading anchor failed: %v", err)
			response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
			return
		}
		if err != nil || anchor.UserID != me.ID {
			response.Forbidden(c, "You can only delete your own comments, or comments on your own anchors", "FORBIDDEN")
			return
		}
	}

	// Atomic: of two concurrent deletes only one succeeds, so counts change once
	if err := h.repo.SoftDeleteComment(ctx, comment.ID); err != nil {
		if errors.Is(err, ErrCommentNotFound) {
			response.NotFound(c, "This comment has already been deleted", "COMMENT_NOT_FOUND")
			return
		}
		log.Printf("comments: delete %s failed: %v", comment.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't delete this comment right now. Please try again", "DELETE_FAILED")
		return
	}

	// Keep counts in sync. anchor.commentCount counts top-level comments and replies.
	removed := 1
	if comment.ParentID != nil {
		// A reply: one fewer reply on its parent
		if err := h.repo.IncrementReplyCount(ctx, *comment.ParentID, -1); err != nil {
			log.Printf("comments: replyCount -1 failed: %v", err)
		}
	} else if n, err := h.repo.SoftDeleteReplies(ctx, comment.ID); err != nil {
		log.Printf("comments: deleting replies of %s failed: %v", comment.ID.Hex(), err)
	} else {
		// A top-level comment: its replies go with it
		removed += int(n)
	}
	h.afterCommentCountChange(ctx, comment.AnchorID, -removed)

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
	me, ok := requireUser(c, "like comments")
	if !ok {
		return
	}
	var req CommentLikeActionRequest
	if !bind.JSON(c, &req) {
		return
	}
	if err := ValidateCommentLikeActionRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "INVALID_ACTION")
		return
	}
	ctx := c.Request.Context()

	var comment *Comment
	if req.Action == "unlike" {
		// Removing a like always works, whatever happened to the anchor or its people since
		if comment, ok = h.loadComment(c); !ok {
			return
		}
		deleted, err := h.repo.DeleteCommentLike(ctx, comment.ID, me.ID)
		if err != nil {
			log.Printf("comments: unlike failed: %v", err)
			response.InternalServerError(c, "We couldn't remove your like right now. Please try again", "UNLIKE_FAILED")
			return
		}
		if deleted {
			if err := h.repo.IncrementCommentLikeCount(ctx, comment.ID, -1); err != nil {
				log.Printf("comments: likeCount -1 failed: %v", err)
			}
		}
		h.respondWithLikeCount(c, comment, false)
		return
	}

	// Liking needs a comment you can see; a comment by someone who blocked you is "not found"
	if comment, _, ok = h.loadCommentToActOn(c, me, "like it"); !ok {
		return
	}

	created, err := h.repo.CreateCommentLike(ctx, comment.ID, me.ID)
	if err != nil {
		log.Printf("comments: like failed: %v", err)
		response.InternalServerError(c, "We couldn't like this comment right now. Please try again", "LIKE_FAILED")
		return
	}
	// Only a like that was actually inserted changes the count
	if created {
		if err := h.repo.IncrementCommentLikeCount(ctx, comment.ID, 1); err != nil {
			log.Printf("comments: likeCount +1 failed: %v", err)
		}
	}
	h.respondWithLikeCount(c, comment, true)
}

// loadCommentToActOn is loadViewableComment for liking or replying: a user who blocked the
// comment's author or the anchor's owner is told how to fix it instead of getting a 404
// (only the other direction, being blocked, stays hidden behind "not found").
func (h *Handler) loadCommentToActOn(c *gin.Context, me *auth.User, action string) (*Comment, *anchors.Anchor, bool) {
	comment, ok := h.loadComment(c)
	if !ok {
		return nil, nil, false
	}
	if comment.UserID != me.ID && me.HasBlocked(comment.UserID) {
		response.Forbidden(c, "You've blocked the author of this comment. Unblock them first if you want to "+action, "USER_BLOCKED")
		return nil, nil, false
	}
	anchor, ok := h.loadAnchorByID(c, comment.AnchorID, msgCommentNotFound, "COMMENT_NOT_FOUND")
	if !ok {
		return nil, nil, false
	}
	if anchor.UserID != me.ID && me.HasBlocked(anchor.UserID) {
		response.Forbidden(c, "You've blocked the owner of this anchor. Unblock them first if you want to "+action, "USER_BLOCKED")
		return nil, nil, false
	}
	return h.loadViewableComment(c, me)
}

func (h *Handler) respondWithLikeCount(c *gin.Context, comment *Comment, hasLiked bool) {
	likeCount := comment.LikeCount
	if updated, err := h.repo.GetCommentByID(c.Request.Context(), comment.ID); err == nil {
		likeCount = updated.LikeCount
	}
	response.Success(c, CommentLikeResponse{HasLiked: hasLiked, LikeCount: likeCount})
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
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id}/like/status [get]
func (h *Handler) GetCommentLikeStatus(c *gin.Context) {
	me, ok := requireUser(c, "check comment likes")
	if !ok {
		return
	}
	comment, _, ok := h.loadViewableComment(c, me)
	if !ok {
		return
	}
	hasLiked, err := h.repo.ExistsCommentLike(c.Request.Context(), comment.ID, me.ID)
	if err != nil {
		log.Printf("comments: like lookup failed: %v", err)
		response.InternalServerError(c, "We couldn't check whether you liked this comment. Please try again", "STATUS_CHECK_FAILED")
		return
	}
	response.Success(c, CommentLikeResponse{HasLiked: hasLiked, LikeCount: comment.LikeCount})
}

// ---------- response building ----------

func (h *Handler) buildCommentResponse(comment *Comment, author *auth.User, hasLiked bool) CommentResponse {
	var profilePic *string
	if author.ProfilePictureURL != "" {
		pic := author.ProfilePictureURL
		profilePic = &pic
	}
	mentions := comment.Mentions
	if mentions == nil {
		mentions = []primitive.ObjectID{}
	}

	return CommentResponse{
		ID:         comment.ID,
		AnchorID:   comment.AnchorID,
		ParentID:   comment.ParentID,
		ReplyCount: comment.ReplyCount,
		Content:    comment.Content,
		Mentions:   mentions,
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
		Engagement: CommentEngagement{HasLiked: hasLiked},
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

func (h *Handler) buildCommentListResponse(ctx context.Context, comments []Comment, viewer *auth.User) ([]CommentResponse, error) {
	if len(comments) == 0 {
		return []CommentResponse{}, nil
	}

	authorIDs := make([]primitive.ObjectID, len(comments))
	commentIDs := make([]primitive.ObjectID, len(comments))
	for i, c := range comments {
		authorIDs[i] = c.UserID
		commentIDs[i] = c.ID
	}

	authors, err := h.authRepo.GetUsersByIDs(ctx, authorIDs)
	if err != nil {
		return nil, err
	}
	authorMap := make(map[primitive.ObjectID]*auth.User, len(authors))
	for i := range authors {
		authorMap[authors[i].ID] = &authors[i]
	}

	likedMap := map[primitive.ObjectID]bool{}
	if viewer != nil {
		if likedMap, err = h.repo.GetUserLikedComments(ctx, viewer.ID, commentIDs); err != nil {
			return nil, err
		}
	}

	responses := make([]CommentResponse, len(comments))
	for i := range comments {
		responses[i] = h.buildCommentResponseWithAuthor(&comments[i], authorMap[comments[i].UserID], likedMap[comments[i].ID])
	}
	return responses, nil
}

// ---------- replies ----------

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
	me, ok := requireUser(c, "reply")
	if !ok {
		return
	}
	var req CreateCommentRequest
	if !bind.JSON(c, &req) {
		return
	}
	if err := ValidateCreateCommentRequest(&req); err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	target, anchor, ok := h.loadCommentToActOn(c, me, "reply")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	failMsg := "We couldn't post your reply right now. Please try again"

	// Replies always hang off the top-level comment
	parentID := target.ID
	if target.ParentID != nil {
		parentID = *target.ParentID
	}

	mentions, err := h.resolveMentions(ctx, req.Content, anchor, me)
	if err != nil {
		log.Printf("comments: resolving mentions failed: %v", err)
		response.InternalServerError(c, failMsg, "CREATE_FAILED")
		return
	}

	reply := &Comment{
		AnchorID: target.AnchorID,
		UserID:   me.ID,
		ParentID: &parentID,
		Content:  req.Content,
		Mentions: mentions,
	}
	if err := h.repo.CreateComment(ctx, reply); err != nil {
		log.Printf("comments: creating reply failed: %v", err)
		response.InternalServerError(c, failMsg, "CREATE_FAILED")
		return
	}

	// If the thread was deleted while we were saving, the reply would be invisible but still
	// counted: take it back
	if _, err := h.repo.GetCommentByID(ctx, parentID); errors.Is(err, ErrCommentNotFound) {
		if delErr := h.repo.SoftDeleteComment(ctx, reply.ID); delErr != nil {
			log.Printf("comments: removing orphaned reply %s failed: %v", reply.ID.Hex(), delErr)
		}
		response.NotFound(c, "The comment you replied to was just deleted", "COMMENT_NOT_FOUND")
		return
	}

	if err := h.repo.IncrementReplyCount(ctx, parentID, 1); err != nil {
		log.Printf("Failed to increment reply count for comment %s: %v", parentID.Hex(), err)
	}
	h.afterCommentCountChange(ctx, anchor.ID, 1)

	if h.notificationService != nil {
		go func(anchorOwner, repliedTo primitive.ObjectID) {
			data := &notifications.CommentData{
				ID:       reply.ID,
				AnchorID: reply.AnchorID,
				Content:  reply.Content,
				Mentions: reply.Mentions,
			}
			if err := h.notificationService.CreateReplyNotifications(context.Background(), data, repliedTo, anchorOwner, me); err != nil {
				log.Printf("Failed to create reply notifications: %v", err)
			}
		}(anchor.UserID, target.UserID)
	}

	response.Created(c, h.buildCommentResponse(reply, me, false))
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
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /comments/{id}/replies [get]
func (h *Handler) ListReplies(c *gin.Context) {
	page, limit, err := parsePaging(c.Query("page"), c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}
	viewer := currentUser(c)
	parent, _, ok := h.loadViewableComment(c, viewer)
	if !ok {
		return
	}
	if parent.ParentID != nil {
		response.BadRequest(c, "This is a reply. Replies are listed under the comment they belong to: "+
			"use GET /comments/"+parent.ParentID.Hex()+"/replies", "NOT_TOP_LEVEL")
		return
	}
	ctx := c.Request.Context()
	failMsg := "We couldn't load the replies right now. Please try again"

	hidden, err := h.authRepo.HiddenUserIDs(ctx, viewer)
	if err != nil {
		log.Printf("comments: loading blocks failed: %v", err)
		response.InternalServerError(c, failMsg, "FETCH_FAILED")
		return
	}
	replies, total, err := h.repo.GetReplies(ctx, parent.ID, hidden, page, limit)
	if err != nil {
		log.Printf("comments: listing replies of %s failed: %v", parent.ID.Hex(), err)
		response.InternalServerError(c, failMsg, "FETCH_FAILED")
		return
	}
	list, err := h.buildCommentListResponse(ctx, replies, viewer)
	if err != nil {
		log.Printf("comments: building replies failed: %v", err)
		response.InternalServerError(c, failMsg, "FETCH_FAILED")
		return
	}

	resp := PaginatedRepliesResponse{ParentID: parent.ID, Replies: list}
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	resp.Pagination.Page = page
	resp.Pagination.Limit = limit
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = totalPages
	resp.Pagination.HasMore = page < totalPages

	response.Success(c, resp)
}
