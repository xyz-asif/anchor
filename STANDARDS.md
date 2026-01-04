# Coding Standards & Conventions

This document outlines the coding standards, conventions, and best practices for the **GoTodo** project. Adherence to these standards ensures code consistency, maintainability, and readability.

## Table of Contents
- [API Response Format](#api-response-format)
- [Error Handling & Codes](#error-handling--codes)
- [Naming Conventions](#naming-conventions)
- [Request Validation](#request-validation)
- [Database Conventions](#database-conventions)
- [Documentation (Swagger)](#documentation-swagger)
- [Package Organization](#package-organization)

---

## API Response Format

All API endpoints MUST return responses in the unified `APIResponse` envelope structure. Never return raw objects or strings.

### Success Response
- **HTTP Status**: `200 OK` or `201 Created`
- **Structure**:
```json
{
  "success": true,
  "statusCode": 200,
  "message": "Operation successful",
  "data": {
    "id": "507f1f77bcf86cd799439011",
    "title": "Buy groceries"
  }
}
```

### Failure Response
- **HTTP Status**: `4xx` or `5xx`
- **Structure**:
```json
{
  "success": false,
  "statusCode": 400,
  "message": "Invalid request format",
  "code": "INVALID_JSON"
}
```

### Usage in Code
Use the helper functions in `internal/pkg/response`:
```go
// Success
response.Success(c, todo)
response.Created(c, newTodo)

// Error
response.BadRequest(c, "Invalid input", "INVALID_INPUT")
response.NotFound(c, "Todo not found")
```

---

## Error Handling & Codes

### Global Sentinel Errors
Define reusable errors in `pkg/errors/errors.go`:
```go
var (
    ErrNotFound     = errors.New("resource not found")
    ErrUnauthorized = errors.New("unauthorized")
)
```

### Error Codes
When returning errors to the client, use UPPER_SNAKE_CASE error codes to help the frontend identify the error type programmatically.

| HTTP Status | Message | Code |
| :--- | :--- | :--- |
| 400 | Invalid request format | `INVALID_JSON` |
| 401 | Security Token Invalid | `AUTH_FAILED` |
| 403 | Forbidden access | `FORBIDDEN` |
| 422 | Validation failed | `VALIDATION_FAILED` |
| 500 | Database error | `DATABASE_ERROR` |

---

## Naming Conventions

### Files & Directories
- **Packages/Directories**: lowercase, short, one word if possible. (e.g., `todos`, `auth`, `pkg`).
- **Files**: snake_case. (e.g., `handler.go`, `model_test.go`, `api_response.go`).

### Code Elements
| Type | Style | Example |
| :--- | :--- | :--- |
| **Structs** | PascalCase | `CreateTodoRequest`, `TodoHandler` |
| **Interfaces** | PascalCase | `Repository`, `Service` |
| **Functions** | PascalCase | `GetByID`, `ValidateCreateTodo` |
| **Variables** | camelCase | `todoID`, `repo` |
| **Constants** | PascalCase | `DefaultLimit`, `MaxRetries` |

### JSON & BSON
- **JSON Fields**: camelCase.
- **BSON Fields**: camelCase using `bson` tag.

**Example Model:**
```go
type Todo struct {
    ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    UserID    string             `bson:"userId" json:"userId"` // camelCase in tags
    CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}
```

---

## Request Validation

Validation is performed in two steps:
1.  **Structural Validation**: Using Gin `binding` tags on request structs.
2.  **Logic Validation**: Custom validation functions in `validator.go`.

### Input Struct Pattern
Create explicit Request structs for every endpoint that accepts a body.
```go
type CreateTodoRequest struct {
    Title    string `json:"title" binding:"required,min=3"` // Gin binding
    Priority string `json:"priority" enums:"low,medium,high"`
}
```

### Custom Validator Pattern
```go
// internal/features/todos/validator.go
func ValidateCreateTodo(req *CreateTodoRequest) error {
    req.Title = strings.TrimSpace(req.Title) // Sanitize
    if len(req.Title) > 200 {
        return errors.New("Title cannot exceed 200 characters")
    }
    return nil
}
```

---

## Database Conventions

### Indexes
Indexes MUST be defined in code, specifically in the `NewRepository` constructor. Do not rely on manual DB setup.

### ID Handling
- Use `primitive.ObjectID` for `ID` fields in structs.
- Accept IDs as `string` in Handler/Service methods and convert to `ObjectID` inside the Repository.

---

## Documentation (Swagger)

All Handlers MUST be documented using `swaggo` annotations immediately preceding the handler function.

**Format:**
```go
// Create godoc
// @Summary Short summary
// @Description Detailed description
// @Tags feature_name
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateTodoRequest true "Request Body"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} response.APIResponse
// @Router /todos/ [post]
func (h *Handler) Create(c *gin.Context) { ... }
```

---

## Package Organization

New features should be added to `internal/features/` as a self-contained package.

**Structure for a new feature `projects`:**
1.  Create `internal/features/projects/`
2.  Add `model.go` (Structs)
3.  Add `repository.go` (DB logic)
4.  Add `handler.go` (HTTP logic)
5.  Add `routes.go` (RegisterRoutes)
6.  Register the new routes in `internal/routes/routes.go`.
