package interests

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"github.com/xyz-asif/gotodo/internal/pkg/tags"
)

type Handler struct {
	repo *Repository
	cfg  *config.Config
}

func NewHandler(repo *Repository, cfg *config.Config) *Handler {
	return &Handler{
		repo: repo,
		cfg:  cfg,
	}
}

type SaveInterestsRequest struct {
	Tags []string `json:"tags" binding:"required,min=1,max=10,dive,min=2,max=30"`
}

// @Summary Save user interests
// @Description Save interest tags for the authenticated user
// @Tags interests
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body SaveInterestsRequest true "Interest tags"
// @Success 200 {object} response.APIResponse
// @Router /users/me/interests [post]
func (h *Handler) SaveInterests(c *gin.Context) {
	var req SaveInterestsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request format", "INVALID_JSON")
		return
	}

	val, exists := c.Get("user")
	if !exists {
		response.Unauthorized(c, "Authentication required", "AUTH_FAILED")
		return
	}
	user, ok := val.(*auth.User)
	if !ok {
		response.InternalServerError(c, "User context error", "INTERNAL_ERROR")
		return
	}

	// Store tags the same way anchor tags are stored (lowercase, no '#', no duplicates)
	// so the for_you feed can match them against anchors.
	tags := normalizeInterests(req.Tags)
	if len(tags) == 0 {
		response.BadRequest(c, "At least one valid interest is required (2-30 letters, digits or hyphens)", "VALIDATION_FAILED")
		return
	}

	if err := h.repo.UpdateUserInterests(c.Request.Context(), user.ID, tags); err != nil {
		response.InternalServerError(c, "Failed to save interests", "DATABASE_ERROR")
		return
	}

	response.Success(c, gin.H{"interests": tags}, "Interests saved successfully")
}

// GetSuggestedInterests returns personalized or popular interest suggestions
// @Summary Get suggested interests
// @Description Get personalized interest suggestions based on user activity or popular tags
// @Tags interests
// @Produce json
// @Param limit query int false "Max suggestions (default 10, max 20)"
// @Success 200 {object} response.APIResponse{data=SuggestedInterestsResponse}
// @Router /interests/suggested [get]
func (h *Handler) GetSuggestedInterests(c *gin.Context) {
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 20 {
			limit = parsedLimit
		}
	}

	// Check if user is authenticated
	var categories []Category
	var basedOn string
	saved := []string{}

	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			saved = normalizeInterests(user.Interests)
			// Personalized suggestions
			userTags, err := h.repo.GetSuggestedTags(c.Request.Context(), user.ID, limit)
			if err == nil && len(userTags) > 0 {
				categories = userTags
				basedOn = "personalized"
			}
		}
	}

	// Fallback to popular tags if not authenticated or no personalized data
	if len(categories) == 0 {
		popularTags, err := h.repo.GetPopularTags(c.Request.Context(), limit)
		if err != nil {
			response.InternalServerError(c, "Failed to fetch suggestions", "DATABASE_ERROR")
			return
		}
		categories = popularTags
		basedOn = "popular"
	}

	resp := SuggestedInterestsResponse{
		Categories: withSavedInterests(categories, saved, limit),
		BasedOn:    basedOn,
		Selected:   saved,
	}

	response.Success(c, resp)
}

// normalizeInterests uses the same tag rules as anchors, so saved interests can match anchor tags
func normalizeInterests(raw []string) []string {
	return tags.NormalizeList(raw)
}

// withSavedInterests puts the user's saved interests first (marked selected), followed by
// up to `limit` other suggestions, so an interest picker can show them pre-checked.
func withSavedInterests(suggestions []Category, saved []string, limit int) []Category {
	bySlug := map[string]Category{}
	for _, cat := range suggestions {
		bySlug[strings.ToLower(cat.Name)] = cat
	}

	result := make([]Category, 0, len(saved)+limit)
	isSaved := map[string]bool{}
	for _, tag := range saved {
		isSaved[tag] = true
		cat, ok := bySlug[tag]
		if !ok {
			cat = Category{Name: tag, Slug: generateSlug(tag), Icon: getIconForCategory(tag)}
		}
		cat.Selected = true
		result = append(result, cat)
	}

	added := 0
	for _, cat := range suggestions {
		if added >= limit {
			break
		}
		if isSaved[strings.ToLower(cat.Name)] {
			continue
		}
		result = append(result, cat)
		added++
	}
	return result
}
