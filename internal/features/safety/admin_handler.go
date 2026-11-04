package safety

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/bind"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// reporterInfo is the public subset of the reporting user shown to moderators
type reporterInfo struct {
	ID          primitive.ObjectID `json:"id"`
	Username    string             `json:"username"`
	DisplayName string             `json:"displayName"`
}

// AdminListReports godoc
// @Summary List reports (admin)
// @Description Newest first. Filter by status and target type. Requires an admin account (ADMIN_EMAILS).
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param status query string false "pending | reviewed | actioned | dismissed"
// @Param targetType query string false "anchor | item | comment | user"
// @Param page query int false "Page (default 1)"
// @Param limit query int false "Items per page (default 20, max 100)"
// @Success 200 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Router /admin/reports [get]
func (h *Handler) AdminListReports(c *gin.Context) {
	q, err := parseReportListQuery(c.Query("status"), c.Query("targetType"), c.Query("page"), c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}

	ctx := c.Request.Context()
	reports, total, err := h.repo.ListReports(ctx, q.Status, q.TargetType, q.Page, q.Limit)
	if err != nil {
		h.fail(c, "listing reports", err)
		return
	}

	// Batch-load reporters
	ids := make([]primitive.ObjectID, 0, len(reports))
	for _, r := range reports {
		ids = append(ids, r.ReporterID)
	}
	reporters := map[primitive.ObjectID]reporterInfo{}
	if len(ids) > 0 {
		users, err := h.authRepo.GetUsersByIDs(ctx, ids)
		if err != nil {
			h.fail(c, "loading reporters", err)
			return
		}
		for _, u := range users {
			reporters[u.ID] = reporterInfo{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName}
		}
	}

	items := make([]gin.H, 0, len(reports))
	for _, r := range reports {
		var reporter interface{}
		if info, ok := reporters[r.ReporterID]; ok {
			reporter = info
		}
		items = append(items, gin.H{"report": r, "reporter": reporter})
	}

	totalPages := int((total + int64(q.Limit) - 1) / int64(q.Limit))
	response.Success(c, gin.H{
		"data": items,
		"pagination": gin.H{
			"page":       q.Page,
			"limit":      q.Limit,
			"total":      total,
			"totalPages": totalPages,
			"hasMore":    q.Page < totalPages,
		},
	})
}

// AdminGetReport godoc
// @Summary Get a report with reporter and target details (admin)
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Param id path string true "Report ID"
// @Success 200 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /admin/reports/{id} [get]
func (h *Handler) AdminGetReport(c *gin.Context) {
	id, ok := parseReportID(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	report, err := h.repo.GetReportByID(ctx, id)
	if err != nil {
		h.fail(c, "loading report", err)
		return
	}
	if report == nil {
		response.NotFound(c, msgReportNotFound, "REPORT_NOT_FOUND")
		return
	}

	var reporter interface{}
	if u, err := h.authRepo.GetUserByObjectID(ctx, report.ReporterID); err == nil && u != nil {
		reporter = reporterInfo{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName}
	}

	// nil target means it has since been deleted
	target, err := h.repo.TargetSummary(ctx, report.TargetType, report.TargetID)
	if err != nil {
		h.fail(c, "loading reported content", err)
		return
	}

	response.Success(c, gin.H{"report": report, "reporter": reporter, "target": target})
}

// AdminUpdateReport godoc
// @Summary Set a report's moderation status (admin)
// @Description Records the decision, the admin who made it, and an optional note.
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Report ID"
// @Param request body UpdateReportRequest true "New status and note"
// @Success 200 {object} response.APIResponse{data=Report}
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /admin/reports/{id} [patch]
func (h *Handler) AdminUpdateReport(c *gin.Context) {
	id, ok := parseReportID(c)
	if !ok {
		return
	}
	admin, ok := currentUser(c)
	if !ok {
		return
	}

	var req UpdateReportRequest
	if !bind.JSON(c, &req) {
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if !validStatus(status) {
		msg := "Please set a status: pending, reviewed, actioned or dismissed"
		if status != "" {
			msg = fmt.Sprintf(`"%s" isn't a report status. Use pending, reviewed, actioned or dismissed`, req.Status)
		}
		response.BadRequest(c, msg, "VALIDATION_FAILED")
		return
	}
	note := strings.TrimSpace(req.Note)
	if n := utf8.RuneCountInString(note); n > maxNoteLength {
		response.BadRequest(c, fmt.Sprintf("The note can be at most %d characters long (this one has %d)", maxNoteLength, n), "VALIDATION_FAILED")
		return
	}

	report, err := h.repo.UpdateReportStatus(c.Request.Context(), id, admin.ID, status, note)
	if errors.Is(err, ErrAlreadyReported) {
		response.Conflict(c, "This report can't be reopened: the same person has a newer open report on the same content", "ALREADY_REPORTED")
		return
	}
	if err != nil {
		h.fail(c, "updating report", err)
		return
	}
	if report == nil {
		response.NotFound(c, msgReportNotFound, "REPORT_NOT_FOUND")
		return
	}

	response.Success(c, report, "Report updated")
}

// AdminSetVerification godoc
// @Summary Grant or remove a user's verified badge (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Param request body SetVerificationRequest true "isVerified true/false"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /admin/users/{id}/verification [patch]
func (h *Handler) AdminSetVerification(c *gin.Context) {
	userID, ok := parseUserID(c)
	if !ok {
		return
	}

	var req SetVerificationRequest
	if !bind.JSON(c, &req) {
		return
	}

	ctx := c.Request.Context()
	target, err := h.authRepo.GetUserByObjectID(ctx, userID)
	if errors.Is(err, auth.ErrUserNotFound) {
		response.NotFound(c, msgUserNotFound, "USER_NOT_FOUND")
		return
	}
	if err != nil {
		h.fail(c, "loading user", err)
		return
	}

	if err := h.authRepo.UpdateUser(ctx, userID.Hex(), map[string]interface{}{
		"isVerified": *req.IsVerified,
		"updatedAt":  time.Now(),
	}); err != nil {
		h.fail(c, "updating verification", err)
		return
	}

	response.Success(c, gin.H{
		"id":         target.ID,
		"username":   target.Username,
		"isVerified": *req.IsVerified,
	}, "Verification updated")
}

const (
	maxNoteLength     = 1000
	maxReportsPage    = 10000
	msgReportNotFound = "This report doesn't exist"
)

func validStatus(s string) bool {
	switch s {
	case ReportStatusPending, ReportStatusReviewed, ReportStatusActioned, ReportStatusDismissed:
		return true
	}
	return false
}

func parseReportID(c *gin.Context) (primitive.ObjectID, bool) {
	raw := c.Param("id")
	id, err := primitive.ObjectIDFromHex(raw)
	if err != nil {
		response.BadRequest(c, fmt.Sprintf(`"%s" isn't a valid report ID`, raw), "INVALID_ID")
		return primitive.NilObjectID, false
	}
	return id, true
}

// parseReportListQuery reads the admin list filters, explaining any bad value
func parseReportListQuery(status, targetType, pageStr, limitStr string) (*ReportListQuery, error) {
	q := &ReportListQuery{Page: 1, Limit: 20}
	q.Status = strings.ToLower(strings.TrimSpace(status))
	if q.Status != "" && !validStatus(q.Status) {
		return nil, fmt.Errorf(`"%s" isn't a report status. Use pending, reviewed, actioned or dismissed`, status)
	}
	q.TargetType = strings.ToLower(strings.TrimSpace(targetType))
	switch q.TargetType {
	case "", TargetAnchor, TargetItem, TargetComment, TargetUser:
	default:
		return nil, fmt.Errorf(`"%s" isn't a target type. Use anchor, item, comment or user`, targetType)
	}
	if s := strings.TrimSpace(pageStr); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return nil, fmt.Errorf(`Page must be a whole number of 1 or more (got "%s")`, pageStr)
		}
		if n > maxReportsPage {
			return nil, fmt.Errorf("Page can't be higher than %d", maxReportsPage)
		}
		q.Page = n
	}
	if s := strings.TrimSpace(limitStr); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return nil, fmt.Errorf(`Limit must be a whole number between 1 and 100 (got "%s")`, limitStr)
		}
		if n > 100 {
			n = 100
		}
		q.Limit = n
	}
	return q, nil
}
