package safety

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/bind"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Handler struct {
	repo     *Repository
	authRepo *auth.Repository
	cfg      *config.Config
}

func NewHandler(repo *Repository, authRepo *auth.Repository, cfg *config.Config) *Handler {
	return &Handler{
		repo:     repo,
		authRepo: authRepo,
		cfg:      cfg,
	}
}

const (
	minReasonLength  = 5
	maxReasonLength  = 500
	maxReportsPerDay = 30 // keeps one account from flooding the moderation queue

	msgTryAgain     = "Something went wrong on our side. Please try again"
	msgUserNotFound = "This user doesn't exist or has deleted their account"
)

// @Summary Report content or a user
// @Description Report an anchor, an item, a comment or a user to the moderators. You can have one open report per target, and send up to 30 reports a day.
// @Tags safety
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateReportRequest true "Report details: targetType is anchor, item, comment or user; reason is 5-500 characters"
// @Success 201 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Failure 409 {object} response.APIResponse
// @Failure 429 {object} response.APIResponse
// @Router /reports [post]
func (h *Handler) CreateReport(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	var req CreateReportRequest
	if !bind.JSON(c, &req) {
		return
	}

	targetType := strings.ToLower(strings.TrimSpace(req.TargetType))
	switch targetType {
	case TargetAnchor, TargetItem, TargetComment, TargetUser:
	case "":
		response.BadRequest(c, "Please say what you're reporting: targetType is anchor, item, comment or user", "VALIDATION_FAILED")
		return
	default:
		response.BadRequest(c, fmt.Sprintf(`"%s" can't be reported. targetType is anchor, item, comment or user`, req.TargetType), "VALIDATION_FAILED")
		return
	}
	targetID, err := primitive.ObjectIDFromHex(strings.TrimSpace(req.TargetID))
	if err != nil {
		response.BadRequest(c, fmt.Sprintf(`"%s" isn't a valid %s ID`, req.TargetID, targetType), "INVALID_ID")
		return
	}
	reason, err := cleanReason(req.Reason)
	if err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}
	if targetType == TargetUser && targetID == user.ID {
		response.BadRequest(c, "You can't report yourself", "CANNOT_REPORT_SELF")
		return
	}

	ctx := c.Request.Context()
	recent, err := h.repo.CountRecentReports(ctx, user.ID, time.Now().Add(-24*time.Hour))
	if err != nil {
		h.fail(c, "counting reports", err)
		return
	}
	if recent >= maxReportsPerDay {
		response.Error(c, 429, fmt.Sprintf("You've sent %d reports in the last 24 hours, which is the limit. Our team is reviewing them; please try again later", maxReportsPerDay), "TOO_MANY_REPORTS")
		return
	}

	ownerID, found, err := h.repo.VisibleTarget(ctx, targetType, targetID, user.ID)
	if err != nil {
		h.fail(c, "loading report target", err)
		return
	}
	if !found {
		response.NotFound(c, fmt.Sprintf("This %s doesn't exist or was already removed", targetType), "TARGET_NOT_FOUND")
		return
	}
	if ownerID == user.ID {
		response.BadRequest(c, fmt.Sprintf("You can't report your own %s. You can edit or delete it instead", targetType), "CANNOT_REPORT_SELF")
		return
	}

	report := &Report{
		ReporterID: user.ID,
		TargetID:   targetID,
		TargetType: targetType,
		Reason:     reason,
	}
	if err := h.repo.CreateReport(ctx, report); err != nil {
		if errors.Is(err, ErrAlreadyReported) {
			response.Conflict(c, fmt.Sprintf("You've already reported this %s. Our team will review it", targetType), "ALREADY_REPORTED")
			return
		}
		h.fail(c, "saving report", err)
		return
	}

	response.Created(c, gin.H{"id": report.ID, "status": report.Status}, "Thanks, your report was sent to our team")
}

// cleanReason removes invisible characters, trims, and checks the length in characters
func cleanReason(raw string) (string, error) {
	reason := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, raw))
	n := utf8.RuneCountInString(reason)
	if n == 0 {
		return "", errors.New("Please tell us what's wrong, so our team can review it")
	}
	if n < minReasonLength {
		return "", fmt.Errorf("Please describe the problem in at least %d characters", minReasonLength)
	}
	if n > maxReasonLength {
		return "", fmt.Errorf("The reason can be at most %d characters long (this one has %d)", maxReasonLength, n)
	}
	return reason, nil
}

// @Summary Block a user
// @Description Blocked users can't see your profile or content, and you won't see theirs. Follows between you (of each other and of each other's anchors) are removed.
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID to block"
// @Success 200 {object} response.APIResponse
// @Router /users/{id}/block [post]
func (h *Handler) BlockUser(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	targetID, ok := parseUserID(c)
	if !ok {
		return
	}
	if targetID == user.ID {
		response.BadRequest(c, "You can't block yourself", "CANNOT_BLOCK_SELF")
		return
	}
	if !h.userExists(c, targetID) {
		return
	}

	ctx := c.Request.Context()
	if err := h.repo.BlockUser(ctx, user.ID, targetID); err != nil {
		if errors.Is(err, ErrListFull) {
			response.BadRequest(c, fmt.Sprintf("You've blocked %d people, which is the maximum. Unblock someone to block another person", maxBlockedUsers), "BLOCK_LIST_FULL")
			return
		}
		h.fail(c, "blocking", err)
		return
	}

	// Blocking ends any follow relationship in both directions (people and anchors)
	if err := h.repo.RemoveFollowsBetween(ctx, user.ID, targetID); err != nil {
		log.Printf("safety: removing follows between %s and %s failed: %v", user.ID.Hex(), targetID.Hex(), err)
	}
	if err := h.repo.RemoveAnchorFollowsBetween(ctx, user.ID, targetID); err != nil {
		log.Printf("safety: removing anchor follows between %s and %s failed: %v", user.ID.Hex(), targetID.Hex(), err)
	}

	response.Success(c, gin.H{"userId": targetID, "isBlocked": true}, "User blocked")
}

// @Summary Unblock a user
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID to unblock"
// @Success 200 {object} response.APIResponse
// @Router /users/{id}/block [delete]
func (h *Handler) UnblockUser(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	targetID, ok := parseUserID(c)
	if !ok {
		return
	}
	if targetID == user.ID {
		response.BadRequest(c, "You can't block or unblock yourself", "INVALID_ACTION")
		return
	}

	if err := h.repo.RemoveBlock(c.Request.Context(), user.ID, targetID); err != nil {
		h.fail(c, "unblocking", err)
		return
	}
	response.Success(c, gin.H{"userId": targetID, "isBlocked": false}, "User unblocked")
}

// GetBlockedUsers returns the list of users blocked by the current user
// @Summary Get blocked users
// @Description The people you blocked, most recently blocked first
// @Tags safety
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.APIResponse{data=[]MutedUserInfo}
// @Failure 401 {object} response.APIResponse
// @Router /users/me/blocks [get]
func (h *Handler) GetBlockedUsers(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		return
	}
	list, err := h.userList(c, user.BlockedUsers)
	if err != nil {
		h.fail(c, "loading blocked users", err)
		return
	}
	response.Success(c, list)
}

// userList returns public info for the given users, newest addition first (lists are
// appended to, so the last ID is the most recent). Deleted accounts are skipped.
func (h *Handler) userList(c *gin.Context, ids []primitive.ObjectID) ([]MutedUserInfo, error) {
	result := []MutedUserInfo{}
	if len(ids) == 0 {
		return result, nil
	}
	users, err := h.authRepo.GetUsersByIDs(c.Request.Context(), ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[primitive.ObjectID]auth.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	for i := len(ids) - 1; i >= 0; i-- {
		u, ok := byID[ids[i]]
		if !ok {
			continue
		}
		delete(byID, ids[i]) // a duplicate ID is listed once
		// Public fields only: never expose email, Google ID or block lists
		result = append(result, MutedUserInfo{
			ID:                u.ID,
			Username:          u.Username,
			DisplayName:       u.DisplayName,
			ProfilePictureURL: u.ProfilePictureURL,
		})
	}
	return result, nil
}

// userExists writes a 404 or 500 and returns false unless the user exists
func (h *Handler) userExists(c *gin.Context, id primitive.ObjectID) bool {
	_, err := h.authRepo.GetUserByObjectID(c.Request.Context(), id)
	if errors.Is(err, auth.ErrUserNotFound) {
		response.NotFound(c, msgUserNotFound, "USER_NOT_FOUND")
		return false
	}
	if err != nil {
		h.fail(c, "loading user", err)
		return false
	}
	return true
}

func (h *Handler) fail(c *gin.Context, what string, err error) {
	log.Printf("safety: %s failed: %v", what, err)
	response.InternalServerError(c, msgTryAgain, "DATABASE_ERROR")
}

func parseUserID(c *gin.Context) (primitive.ObjectID, bool) {
	raw := c.Param("id")
	id, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		response.BadRequest(c, fmt.Sprintf(`"%s" isn't a valid user ID`, raw), "INVALID_ID")
		return primitive.NilObjectID, false
	}
	return id, true
}
