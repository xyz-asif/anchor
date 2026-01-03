// ================== internal/features/todos/handler.go ==================
package todos

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// Create godoc
// @Summary Create a new todo
// @Description Create a new todo for the authenticated user
// @Tags todos
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateTodoRequest true "Todo creation data"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 500 {object} response.APIResponse
// @Router /todos/ [post]
func (h *Handler) Create(c *gin.Context) {
	userID := c.GetString("userID")

	var req CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BindJSONError(c, err)
		return
	}

	if err := ValidateCreateTodo(&req); err != nil {
		response.ValidationFailed(c, err.Error())
		return
	}

	todo := &Todo{
		UserID:      userID,
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		Tags:        req.Tags,
		DueDate:     req.DueDate,
	}

	if todo.Priority == "" {
		todo.Priority = "medium"
	}

	if todo.Tags == nil {
		todo.Tags = []string{}
	}

	if err := h.repo.Create(c.Request.Context(), todo); err != nil {
		response.DatabaseError(c, "Failed to create todo")
		return
	}

	response.Created(c, todo)
}

// Get godoc
// @Summary Get a todo by ID
// @Description Get a specific todo by ID for the authenticated user
// @Tags todos
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Todo ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /todos/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	userID := c.GetString("userID")
	todoID := c.Param("id")

	todo, err := h.repo.GetByID(c.Request.Context(), todoID, userID)
	if err != nil {
		response.BadRequest(c, TranslateTodoError(err))
		return
	}

	if todo == nil {
		response.NotFound(c, "Todo not found")
		return
	}

	response.Success(c, todo)
}

// Update godoc
// @Summary Update a todo
// @Description Update an existing todo for the authenticated user
// @Tags todos
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Todo ID"
// @Param request body UpdateTodoRequest true "Todo update data"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /todos/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	userID := c.GetString("userID")
	todoID := c.Param("id")

	var req UpdateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BindJSONError(c, err)
		return
	}

	if err := ValidateUpdateTodo(&req); err != nil {
		response.ValidationFailed(c, err.Error())
		return
	}

	// Build update document
	update := bson.M{}
	if req.Title != "" {
		update["title"] = req.Title
	}
	if req.Description != "" {
		update["description"] = req.Description
	}
	if req.Completed != nil {
		update["completed"] = *req.Completed
	}
	if req.Priority != "" {
		update["priority"] = req.Priority
	}
	if req.Tags != nil {
		update["tags"] = req.Tags
	}
	if req.DueDate != nil {
		update["dueDate"] = req.DueDate
	}

	if len(update) == 0 {
		response.BadRequest(c, "No fields to update")
		return
	}

	if err := h.repo.Update(c.Request.Context(), todoID, userID, update); err != nil {
		if err.Error() == "Todo not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.BadRequest(c, TranslateTodoError(err))
		return
	}

	// Get updated todo
	todo, err := h.repo.GetByID(c.Request.Context(), todoID, userID)
	if err != nil || todo == nil {
		response.InternalServerError(c, "Failed to retrieve updated todo")
		return
	}

	response.Success(c, todo)
}

// Delete godoc
// @Summary Delete a todo
// @Description Delete a todo for the authenticated user
// @Tags todos
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Todo ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} response.APIResponse
// @Failure 401 {object} response.APIResponse
// @Failure 404 {object} response.APIResponse
// @Router /todos/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	userID := c.GetString("userID")
	todoID := c.Param("id")

	if err := h.repo.Delete(c.Request.Context(), todoID, userID); err != nil {
		if err.Error() == "Todo not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.BadRequest(c, TranslateTodoError(err))
		return
	}

	response.Success(c, map[string]string{"message": "Todo deleted successfully"})
}

// List godoc
// @Summary List todos
// @Description Get a list of todos for the authenticated user with optional filtering
// @Tags todos
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param completed query bool false "Filter by completion status"
// @Param limit query int false "Maximum number of todos to return (default: 50, max: 100)"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} response.APIResponse
// @Failure 500 {object} response.APIResponse
// @Router /todos/ [get]
func (h *Handler) List(c *gin.Context) {
	userID := c.GetString("userID")

	// Query params
	completedStr := c.Query("completed")
	limitStr := c.Query("limit")

	var completed *bool
	if completedStr != "" {
		val, err := strconv.ParseBool(completedStr)
		if err == nil {
			completed = &val
		}
	}

	limit := 50 // Default limit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	todos, err := h.repo.List(c.Request.Context(), userID, completed, limit)
	if err != nil {
		response.InternalServerError(c, "Failed to get todos")
		return
	}

	// Get total count
	total, err := h.repo.CountByUser(c.Request.Context(), userID)
	if err != nil {
		response.InternalServerError(c, "Failed to count todos")
		return
	}

	// Use paginated response helper
	response.Paginated(c, todos, total, limit)
}
