// ================== internal/features/todos/model.go ==================
package todos

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Todo represents a todo item
// @Description Todo item with all its properties
type Todo struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id" example:"507f1f77bcf86cd799439011"`
	UserID      string             `bson:"userId" json:"userId" example:"507f1f77bcf86cd799439011"`
	Title       string             `bson:"title" json:"title" example:"Buy groceries"`
	Description string             `bson:"description" json:"description" example:"Get milk, bread, and eggs"`
	Completed   bool               `bson:"completed" json:"completed" example:"false"`
	Priority    string             `bson:"priority" json:"priority" example:"medium" enums:"low,medium,high"`
	Tags        []string           `bson:"tags" json:"tags" example:"groceries,home"`
	DueDate     *time.Time         `bson:"dueDate,omitempty" json:"dueDate,omitempty" example:"2023-12-31T23:59:59Z"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt" example:"2023-01-01T00:00:00Z"`
	UpdatedAt   time.Time          `bson:"updatedAt" json:"updatedAt" example:"2023-01-01T00:00:00Z"`
}

// CreateTodoRequest represents todo creation data
// @Description Data required to create a new todo
type CreateTodoRequest struct {
	Title       string     `json:"title" binding:"required,min=3" example:"Buy groceries"`
	Description string     `json:"description" example:"Get milk, bread, and eggs"`
	Priority    string     `json:"priority" example:"medium" enums:"low,medium,high"`
	Tags        []string   `json:"tags" example:"groceries,home"`
	DueDate     *time.Time `json:"dueDate" example:"2023-12-31T23:59:59Z"`
}

// UpdateTodoRequest represents todo update data
// @Description Data for updating an existing todo
type UpdateTodoRequest struct {
	Title       string     `json:"title" example:"Buy groceries"`
	Description string     `json:"description" example:"Get milk, bread, and eggs"`
	Completed   *bool      `json:"completed" example:"true"`
	Priority    string     `json:"priority" example:"high" enums:"low,medium,high"`
	Tags        []string   `json:"tags" example:"groceries,home,urgent"`
	DueDate     *time.Time `json:"dueDate" example:"2023-12-31T23:59:59Z"`
}
