package auth

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User represents a registered user in the system
type User struct {
	ID                primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	GoogleID          string             `bson:"googleId" json:"googleId"`
	Email             string             `bson:"email" json:"email"`
	Username          string             `bson:"username" json:"username"`
	UsernameChanged   bool               `bson:"usernameChanged" json:"usernameChanged"`
	UsernameChangedAt *time.Time         `bson:"usernameChangedAt" json:"usernameChangedAt"`
	DisplayName       string             `bson:"displayName" json:"displayName"`
	Bio               string             `bson:"bio" json:"bio"`
	ProfilePictureURL string             `bson:"profilePictureUrl" json:"profilePictureUrl"`
	FollowerCount     int                `bson:"followerCount" json:"followerCount"`
	FollowingCount    int                `bson:"followingCount" json:"followingCount"`
	AnchorCount       int                `bson:"anchorCount" json:"anchorCount"`
	IsVerified        bool               `bson:"isVerified" json:"isVerified"`
	JoinedAt          time.Time          `bson:"joinedAt" json:"joinedAt"`
	CreatedAt         time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt         time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// GoogleAuthRequest represents the payload for Google OAuth login
type GoogleAuthRequest struct {
	GoogleIDToken string `json:"googleIdToken" binding:"required"`
}

// AuthResponse represents the response after successful authentication
type AuthResponse struct {
	User        *User  `json:"user"`
	AccessToken string `json:"accessToken"`
}

// UpdateProfileRequest represents the payload for updating user profile
type UpdateProfileRequest struct {
	DisplayName string `json:"displayName" binding:"omitempty,min=3,max=50"`
	Bio         string `json:"bio" binding:"omitempty,max=160"`
}

// UpdateUsernameRequest represents the payload for updating username
type UpdateUsernameRequest struct {
	Username string `json:"username" binding:"required,min=3,max=20"`
}

// ToPublicUser returns a map of user fields safe for public display
func (u *User) ToPublicUser() map[string]interface{} {
	return map[string]interface{}{
		"id":                u.ID,
		"username":          u.Username,
		"displayName":       u.DisplayName,
		"bio":               u.Bio,
		"profilePictureUrl": u.ProfilePictureURL,
		"followerCount":     u.FollowerCount,
		"followingCount":    u.FollowingCount,
		"anchorCount":       u.AnchorCount,
		"isVerified":        u.IsVerified,
		"joinedAt":          u.JoinedAt,
		"createdAt":         u.CreatedAt,
		"updatedAt":         u.UpdatedAt,
	}
}
