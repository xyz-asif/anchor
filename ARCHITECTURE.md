# Architecture Documentation

This document provides a comprehensive overview of the architecture, design decisions, and structural organization of the **GoTodo** application.

## Table of Contents
- [Directory Structure](#directory-structure)
- [Architectural Patterns](#architectural-patterns)
- [Request Lifecycle](#request-lifecycle)
- [Database Strategy](#database-strategy)
- [Error Handling](#error-handling)
- [Response Standardization](#response-standardization)
- [Configuration Management](#configuration-management)

---

## Directory Structure

The project follows a standard Go project layout, optimized for modularity and feature isolation.

```
.
├── cmd/
│   └── api/
│       └── main.go           # Application entry point, server startup
├── internal/
│   ├── config/               # Configuration loading and management
│   ├── database/             # Database connection logic (MongoDB)
│   ├── features/             # Feature-based clean architecture modules
│   │   ├── auth/             # Authentication feature (login, register)
│   │   └── todos/            # Todos management feature
│   ├── middleware/           # HTTP Middleware (Auth, Logger, CORS)
│   ├── pkg/                  # Internal shared packages
│   │   └── response/         # Standardized API response helpers
│   └── routes/               # Centralized route registration
├── pkg/
│   └── errors/               # Global sentinel errors
├── docs/                     # Swagger/OpenAPI documentation (auto-generated)
├── .env                      # Environment variables
└── go.mod                    # Dependency definitions
```

### Key Directories Explained

- **`cmd/api`**: Contains the `main` function. It initializes the database, config, middleware, and registers routes before starting the HTTP server.
- **`internal/features`**: The core business logic is organized by **feature** rather than by layer. Each feature (e.g., `todos`) is self-contained with its own:
  - `handler.go`: HTTP Controllers (Handlers)
  - `repository.go`: Database access logic
  - `model.go`: Domain models and DTOs
  - `validator.go`: Input validation logic
  - `routes.go`: Route definitions for that feature
- **`internal/pkg/response`**: Contains the `APIResponse` struct and helper functions (`Success`, `Created`, `BadRequest`) to ensure every API response follows a strict JSON format.

---

## Architectural Patterns

### Feature-Based Organization (Clean Architecture variant)
Instead of a layered architecture (Controllers -> Services -> Repositories), code is grouped by **Feature** (`todos`, `auth`). This keeps related logic together and makes the codebase easier to navigate and maintain as it grows.

### Handler-Repository Pattern
Currently, the application uses a simplified **Handler-Repository** pattern:
- **Handler**: Handles HTTP requests, parses JSON, performs validation, and formats responses.
- **Repository**: Handles direct database interactions (CRUD).
- **Service Layer**: Explicit Service structs are omitted for simplicity where business logic is light. Logic resides in the Handler or is delegated to Helper functions (e.g., validation).

### Dependency Injection
Dependencies are injected explicitly via constructor functions (Factory pattern).
- **Handlers** receive **Repositories**.
- **Repositories** receive the **Database** connection.
- **Routes** receive the **Database** to initialize the dependency tree.

**Example:**
```go
// internal/features/todos/routes.go
func RegisterRoutes(router *gin.RouterGroup, db *mongo.Database) {
    repo := NewRepository(db)      // Inject DB into Repo
    handler := NewHandler(repo)    // Inject Repo into Handler

    group := router.Group("/todos")
    group.POST("/", handler.Create)
}
```

---

## Request Lifecycle

1.  **Incoming Request**: HTTP Request reaches the server.
2.  **Global Middleware**:
    -   `Gin Recovery`: Recovers from panics.
    -   `Logger`: Logs request details.
    -   `CORS`: Handles Cross-Origin Resource Sharing headers.
3.  **Router (`internal/routes`)**: Dispatches the request to the specific Feature Router.
4.  **Feature Middleware** (Optional): e.g., JWT Authentication.
5.  **Handler (`internal/features/*/handler.go`)**:
    -   Extracts inputs (JSON Body, Query Params, URL Params).
    -   Calls `validator` functions.
    -   Calls `Repository` methods.
6.  **Repository (`internal/features/*/repository.go`)**:
    -   Executes MongoDB queries (`InsertOne`, `FindOne`, etc.).
    -   Maps BSON results to Go Structs (`model.go`).
7.  **Handler (Response)**:
    -   Checks for errors.
    -   Translates specific errors (e.g., `Before` date check) to user-friendly messages.
    -   Uses `internal/pkg/response` helper to send JSON.
8.  **Outgoing Response**: Standardized JSON response sent to client.

---

## Database Strategy

-   **Database**: MongoDB
-   **Driver**: `go.mongodb.org/mongo-driver`
-   **Connection**: Managed in `internal/database/mongo.go`. Connected once at startup and passed down to repositories.
-   **Data Modeling**: Models are defined in `model.go` with `bson` tags for database and `json` tags for API.
-   **Indexes**: Created/Ensured in the `NewRepository` constructor function, ensuring indexes exist when the application starts.

**Example Index Management:**
```go
// internal/features/todos/repository.go
func NewRepository(db *mongo.Database) *Repository {
    collection := db.Collection("todos")
    collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
        {Keys: bson.D{{Key: "userId", Value: 1}}},
        {Keys: bson.D{{Key: "createdAt", Value: -1}}},
    })
    return &Repository{collection: collection}
}
```

---

## Error Handling

### Sentinel Errors
Common errors are defined in `pkg/errors/errors.go` (e.g., `ErrNotFound`, `ErrValidation`).

### Error Translation
Handlers are responsible for capturing low-level errors (including DB errors) and translating them into HTTP status codes and user-friendly messages.

**Pattern:**
```go
if err := h.repo.Update(...); err != nil {
    if err.Error() == "Todo not found" {
        response.NotFound(c, "Todo not found")
        return
    }
    response.BadRequest(c, TranslateTodoError(err))
    return
}
```

---

## Response Standardization

All API responses are wrapped in a standard envelope defined in `internal/pkg/response/api.go`.

**Success Response:**
```json
{
  "success": true,
  "statusCode": 200,
  "message": "success",
  "data": { ... }
}
```

**Error Response:**
```json
{
  "success": false,
  "statusCode": 400,
  "message": "Validation failed",
  "code": "VALIDATION_FAILED",
  "data": null
}
```

---

## Configuration Management

Configuration is loaded from environment variables using `godotenv` and accessed via a centralized `Config` struct in `internal/config/config.go`.

-   **Loader**: `config.Load()` reads `.env` if present.
-   **Struct**: Strongly typed configuration (`Port`, `MongoURI`, `JWTSecret`).
-   **Access**: Config object is created in `main.go` and fields are extracted as needed (e.g., passing `FrontendURL` to CORS).
