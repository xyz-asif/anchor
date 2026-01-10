package auth

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User represents a registered user in the system
type User struct {
	ID                     primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	GoogleID               string               `bson:"googleId" json:"googleId"`
	Email                  string               `bson:"email" json:"email"`
	Username               string               `bson:"username" json:"username"`
	UsernameChanged        bool                 `bson:"usernameChanged" json:"usernameChanged"`
	UsernameChangedAt      *time.Time           `bson:"usernameChangedAt" json:"usernameChangedAt"`
	DisplayName            string               `bson:"displayName" json:"displayName"`
	Bio                    string               `bson:"bio" json:"bio"`
	ProfilePictureURL      string               `bson:"profilePictureUrl" json:"profilePictureUrl"`
	ProfilePicturePublicID string               `bson:"profilePicturePublicId" json:"-"`
	CoverImageURL          string               `bson:"coverImageUrl" json:"coverImageUrl"`
	CoverImagePublicID     string               `bson:"coverImagePublicId" json:"-"`
	FollowerCount          int                  `bson:"followerCount" json:"followerCount"`
	FollowingCount         int                  `bson:"followingCount" json:"followingCount"`
	AnchorCount            int                  `bson:"anchorCount" json:"anchorCount"`
	IsVerified             bool                 `bson:"isVerified" json:"isVerified"`
	JoinedAt               time.Time            `bson:"joinedAt" json:"joinedAt"`
	CreatedAt              time.Time            `bson:"createdAt" json:"createdAt"`
	UpdatedAt              time.Time            `bson:"updatedAt" json:"updatedAt"`
	Interests              []string             `bson:"interests" json:"interests"`
	BlockedUsers           []primitive.ObjectID `bson:"blockedUsers" json:"blockedUsers"`
}

// GoogleAuthRequest represents the payload for Google OAuth login
type GoogleAuthRequest struct {
	GoogleIDToken string `json:"googleIdToken" binding:"required"`
}

// DevLoginRequest for development login (bypasses Google OAuth)
type DevLoginRequest struct {
	Email       string `json:"email" binding:"required,email"`
	DisplayName string `json:"displayName"`
}

// AuthResponse represents the response after successful authentication
type AuthResponse struct {
	User        *User  `json:"user"`
	AccessToken string `json:"accessToken"`
}

// LoginResponse represents the response for DevLogin
type LoginResponse struct {
	Token            string      `json:"token"`
	User             interface{} `json:"user"`
	IsNewUser        bool        `json:"isNewUser"`
	RequiresUsername bool        `json:"requiresUsername"`
}

// UpdateProfileRequest represents the payload for updating user profile
type UpdateProfileRequest struct {
	DisplayName *string `json:"displayName" binding:"omitempty,min=2,max=50"`
	Bio         *string `json:"bio" binding:"omitempty,max=200"`
}

// PublicProfileResponse represents a user's public profile
type PublicProfileResponse struct {
	ID                primitive.ObjectID `json:"id"`
	Username          string             `json:"username"`
	DisplayName       string             `json:"displayName"`
	Bio               string             `json:"bio"`
	ProfilePictureURL string             `json:"profilePictureUrl"`
	CoverImageURL     string             `json:"coverImageUrl"`
	FollowerCount     int                `json:"followerCount"`
	FollowingCount    int                `json:"followingCount"`
	AnchorCount       int                `json:"anchorCount"`
	IsVerified        bool               `json:"isVerified"`
	JoinedAt          time.Time          `json:"joinedAt"`
	IsFollowing       bool               `json:"isFollowing"`
	IsFollowedBy      bool               `json:"isFollowedBy"`
	IsMutual          bool               `json:"isMutual"`
}

// OwnProfileResponse represents the user's own profile with private details
type OwnProfileResponse struct {
	ID                primitive.ObjectID `json:"id"`
	GoogleID          string             `json:"googleId"`
	Email             string             `json:"email"`
	Username          string             `json:"username"`
	DisplayName       string             `json:"displayName"`
	Bio               string             `json:"bio"`
	ProfilePictureURL string             `json:"profilePictureUrl"`
	CoverImageURL     string             `json:"coverImageUrl"`
	FollowerCount     int                `json:"followerCount"`
	FollowingCount    int                `json:"followingCount"`
	AnchorCount       int                `json:"anchorCount"`
	IsVerified        bool               `json:"isVerified"`
	JoinedAt          time.Time          `json:"joinedAt"`
	CreatedAt         time.Time          `json:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt"`
}

// ProfilePictureResponse represents the response after uploading a profile picture
type ProfilePictureResponse struct {
	ProfilePictureURL string `json:"profilePictureUrl"`
}

// CoverImageResponse represents the response after uploading a cover image
type CoverImageResponse struct {
	CoverImageURL string `json:"coverImageUrl"`
}

// PinnedAnchorResponse represents a pinned anchor in user profile
type PinnedAnchorResponse struct {
	ID              primitive.ObjectID `json:"id"`
	Title           string             `json:"title"`
	Description     string             `json:"description"`
	CoverMediaType  string             `json:"coverMediaType"`
	CoverMediaValue string             `json:"coverMediaValue"`
	Visibility      string             `json:"visibility"`
	ItemCount       int                `json:"itemCount"`
	LikeCount       int                `json:"likeCount"`
	CloneCount      int                `json:"cloneCount"`
	CreatedAt       time.Time          `json:"createdAt"`
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
