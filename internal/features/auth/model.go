// ================== internal/features/auth/model.go ==================
package auth

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User represents a user in the system
// @Description User account information
type User struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id" example:"507f1f77bcf86cd799439011"`
	Email     string             `bson:"email" json:"email" example:"user@example.com"`
	Password  string             `bson:"password" json:"-"`
	Name      string             `bson:"name" json:"name" example:"John Doe"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt" example:"2023-01-01T00:00:00Z"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt" example:"2023-01-01T00:00:00Z"`
}

// LoginRequest represents login credentials
// @Description User login request data
type LoginRequest struct {
	Email    string `json:"email" binding:"required" example:"user@example.com"`
	Password string `json:"password" binding:"required" example:"secret123"`
}

// RegisterRequest represents user registration data
// @Description User registration request data
type RegisterRequest struct {
	Email    string `json:"email" binding:"required" example:"user@example.com"`
	Password string `json:"password" binding:"required,min=6" example:"secret123"`
	Name     string `json:"name" binding:"required,min=2" example:"John Doe"`
}

// AuthResponse represents authentication response
// @Description Authentication response with token and user data
type AuthResponse struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	User  *User  `json:"user"`
}
