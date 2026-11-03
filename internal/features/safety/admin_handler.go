package safety

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/features/auth"
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
// @Param targetType query string false "anchor | item | user"
// @Param page query int false "Page (default 1)"
// @Param limit query int false "Items per page (default 20, max 100)"
// @Success 200 {object} response.APIResponse
// @Failure 403 {object} response.APIResponse
// @Router /admin/reports [get]
func (h *Handler) AdminListReports(c *gin.Context) {
	var q ReportListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.BadRequest(c, "Invalid query: "+err.Error(), "INVALID_QUERY")
		return
	}

	ctx := c.Request.Context()
	reports, total, err := h.repo.ListReports(ctx, q.Status, q.TargetType, q.Page, q.Limit)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch reports", "DATABASE_ERROR")
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
			response.InternalServerError(c, "Failed to fetch reporters", "DATABASE_ERROR")
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
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid report ID", "INVALID_ID")
		return
	}

	ctx := c.Request.Context()
	report, err := h.repo.GetReportByID(ctx, id)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch report", "DATABASE_ERROR")
		return
	}
	if report == nil {
		response.NotFound(c, "Report not found", "REPORT_NOT_FOUND")
		return
	}

	var reporter interface{}
	if u, err := h.authRepo.GetUserByObjectID(ctx, report.ReporterID); err == nil && u != nil {
		reporter = reporterInfo{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName}
	}

	// nil target means it has since been deleted
	target, err := h.repo.TargetSummary(ctx, report.TargetType, report.TargetID)
	if err != nil {
		response.InternalServerError(c, "Failed to fetch reported content", "DATABASE_ERROR")
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
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid report ID", "INVALID_ID")
		return
	}

	var req UpdateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "status must be one of: pending, reviewed, actioned, dismissed", "INVALID_REQUEST")
		return
	}

	admin, ok := c.MustGet("user").(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	report, err := h.repo.UpdateReportStatus(c.Request.Context(), id, admin.ID, req.Status, req.Note)
	if err != nil {
		response.InternalServerError(c, "Failed to update report", "DATABASE_ERROR")
		return
	}
	if report == nil {
		response.NotFound(c, "Report not found", "REPORT_NOT_FOUND")
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
	userID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "Invalid user ID", "INVALID_ID")
		return
	}

	var req SetVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "isVerified (true or false) is required", "INVALID_REQUEST")
		return
	}

	ctx := c.Request.Context()
	target, err := h.authRepo.GetUserByObjectID(ctx, userID)
	if err != nil || target == nil {
		response.NotFound(c, "User not found", "USER_NOT_FOUND")
		return
	}

	if err := h.authRepo.UpdateUser(ctx, userID.Hex(), map[string]interface{}{
		"isVerified": *req.IsVerified,
		"updatedAt":  time.Now(),
	}); err != nil {
		response.InternalServerError(c, "Failed to update user", "DATABASE_ERROR")
		return
	}

	response.Success(c, gin.H{
		"id":         target.ID,
		"username":   target.Username,
		"isVerified": *req.IsVerified,
	}, "Verification updated")
}
