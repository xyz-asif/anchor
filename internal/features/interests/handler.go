package interests

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/xyz-asif/gotodo/internal/config"
	"github.com/xyz-asif/gotodo/internal/features/auth"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Handler struct {
	service *Service
	config  *config.Config
}

func NewHandler(service *Service, cfg *config.Config) *Handler {
	return &Handler{
		service: service,
		config:  cfg,
	}
}

// GetSuggestedInterests godoc
// @Summary Get suggested interest categories
// @Description Get personalized category suggestions based on user activity
// @Tags interests
// @Produce json
// @Param limit query int false "Max categories (default 10, max 20)"
// @Success 200 {object} response.APIResponse{data=SuggestedInterestsResponse}
// @Router /interests/suggested [get]
func (h *Handler) GetSuggestedInterests(c *gin.Context) {
	var query SuggestedInterestsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		query.Limit = 10
	}

	if query.Limit < 1 {
		query.Limit = 10
	}
	if query.Limit > 20 {
		query.Limit = 20
	}

	// Get current user if authenticated
	var userID *primitive.ObjectID
	if usr, exists := c.Get("user"); exists {
		if user, ok := usr.(*auth.User); ok {
			userID = &user.ID
		}
	}

	result, err := h.service.GetSuggestedInterests(c.Request.Context(), userID, query.Limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get suggested interests")
		return
	}

	response.Success(c, result)
}
