package interests

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/bind"
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
	// Up to 10 tags; an empty list clears your interests
	Tags *[]string `json:"tags"`
}

const (
	maxInterests       = 10
	maxInterestsInput  = 50 // raw entries accepted before de-duplication
	defaultSuggestions = 10
	maxSuggestions     = 20
)

// @Summary Save user interests
// @Description Replaces your interests (up to 10 tags; an empty list clears them). Tags are stored like anchor tags ("#Web Design" becomes "web-design"), so the for_you feed can match them.
// @Tags interests
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body SaveInterestsRequest true "Interest tags"
// @Success 200 {object} response.APIResponse
// @Failure 400 {object} response.APIResponse
// @Router /users/me/interests [post]
func (h *Handler) SaveInterests(c *gin.Context) {
	user := currentUser(c)
	if user == nil {
		response.Unauthorized(c, "Please sign in to save your interests", "UNAUTHORIZED")
		return
	}

	var req SaveInterestsRequest
	if !bind.JSON(c, &req) {
		return
	}
	tags, err := ValidateInterests(req.Tags)
	if err != nil {
		response.BadRequest(c, err.Error(), "VALIDATION_FAILED")
		return
	}

	if err := h.repo.UpdateUserInterests(c.Request.Context(), user.ID, tags); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response.Unauthorized(c, "Your account no longer exists. Please sign in again", "USER_NOT_FOUND")
			return
		}
		log.Printf("interests: saving for %s failed: %v", user.ID.Hex(), err)
		response.InternalServerError(c, "We couldn't save your interests right now. Please try again", "DATABASE_ERROR")
		return
	}

	message := "Interests saved"
	if len(tags) == 0 {
		message = "Interests cleared"
	}
	response.Success(c, gin.H{"interests": tags}, message)
}

// ValidateInterests normalises the tags exactly like anchor tags and explains any problem.
// Duplicates after normalisation ("Travel", "#travel") count once.
func ValidateInterests(raw *[]string) ([]string, error) {
	if raw == nil {
		return nil, errors.New(`Please send your interests as {"tags": ["travel", "books"]} (an empty list clears them)`)
	}
	if len(*raw) > maxInterestsInput {
		return nil, fmt.Errorf("You can pick at most %d interests", maxInterests)
	}
	out := []string{}
	seen := map[string]bool{}
	for _, t := range *raw {
		n := tags.Normalize(t)
		if n == "" {
			if strings.TrimSpace(t) == "" {
				return nil, errors.New("Interests can't be empty")
			}
			return nil, fmt.Errorf(`"%s" isn't a valid interest. Interests are %d to %d letters or numbers`, t, tags.MinLen, tags.MaxLen)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	if len(out) > maxInterests {
		return nil, fmt.Errorf("You can pick at most %d interests (you picked %d)", maxInterests, len(out))
	}
	return out, nil
}

// GetSuggestedInterests returns personalized or popular interest suggestions
// @Summary Get suggested interests
// @Description Interest suggestions: your saved interests first (selected), then tags from your own and liked anchors, topped up with popular tags
// @Tags interests
// @Produce json
// @Param limit query int false "Max suggestions besides your saved interests (default 10, max 20)"
// @Success 200 {object} response.APIResponse{data=SuggestedInterestsResponse}
// @Failure 400 {object} response.APIResponse
// @Router /interests/suggested [get]
func (h *Handler) GetSuggestedInterests(c *gin.Context) {
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		response.BadRequest(c, err.Error(), "INVALID_QUERY")
		return
	}
	ctx := c.Request.Context()

	saved := []string{}
	var personal []Category
	if user := currentUser(c); user != nil {
		saved = normalizeInterests(user.Interests)
		// Ask for extra, since saved interests are listed separately and skipped below
		if personal, err = h.repo.GetSuggestedTags(ctx, user.ID, limit+len(saved)); err != nil {
			log.Printf("interests: personal suggestions for %s failed, using popular tags: %v", user.ID.Hex(), err)
			personal = nil
		}
	}

	basedOn := "popular"
	if countNew(personal, saved) > 0 {
		basedOn = "personalized"
	}

	// Few personal tags (a new user) are topped up with popular ones
	suggestions := personal
	if countNew(personal, saved) < limit {
		popular, err := h.repo.GetPopularTags(ctx, limit+len(saved)+len(personal))
		if err != nil {
			log.Printf("interests: popular tags failed: %v", err)
			response.InternalServerError(c, "We couldn't load interest suggestions right now. Please try again", "DATABASE_ERROR")
			return
		}
		suggestions = mergeCategories(personal, popular)
	}

	response.Success(c, SuggestedInterestsResponse{
		Categories: withSavedInterests(suggestions, saved, limit),
		BasedOn:    basedOn,
		Selected:   saved,
	})
}

func currentUser(c *gin.Context) *auth.User {
	if val, exists := c.Get("user"); exists {
		if user, ok := val.(*auth.User); ok {
			return user
		}
	}
	return nil
}

func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultSuggestions, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf(`Limit must be a whole number between 1 and %d (got "%s")`, maxSuggestions, raw)
	}
	if n > maxSuggestions {
		n = maxSuggestions
	}
	return n, nil
}

// countNew counts suggestions that aren't already saved interests
func countNew(list []Category, saved []string) int {
	isSaved := map[string]bool{}
	for _, s := range saved {
		isSaved[s] = true
	}
	n := 0
	for _, c := range list {
		if !isSaved[strings.ToLower(c.Name)] {
			n++
		}
	}
	return n
}

// mergeCategories appends the categories of more that aren't in first yet
func mergeCategories(first, more []Category) []Category {
	seen := map[string]bool{}
	out := make([]Category, 0, len(first)+len(more))
	for _, list := range [][]Category{first, more} {
		for _, c := range list {
			key := strings.ToLower(c.Name)
			if !seen[key] {
				seen[key] = true
				out = append(out, c)
			}
		}
	}
	return out
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
