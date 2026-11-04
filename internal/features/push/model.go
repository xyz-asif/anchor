package push

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Supported device platforms
const (
	PlatformAndroid = "android"
	PlatformIOS     = "ios"
	PlatformWeb     = "web"
)

// maxDevicesPerUser caps stored tokens per user; the least recently seen are removed first
const maxDevicesPerUser = 20

// Device is a push token registered by one of a user's app installs
type Device struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID     primitive.ObjectID `bson:"userId" json:"userId"`
	Token      string             `bson:"token" json:"-"`
	Platform   string             `bson:"platform" json:"platform"`
	AppVersion string             `bson:"appVersion,omitempty" json:"appVersion,omitempty"`
	// SessionID is the login session that registered the token: logging out of that session
	// stops pushes to this device, and pushes are only sent while the session is active
	SessionID  string    `bson:"sessionId,omitempty" json:"-"`
	CreatedAt  time.Time `bson:"createdAt" json:"createdAt"`
	LastSeenAt time.Time `bson:"lastSeenAt" json:"lastSeenAt"`
}

// Settings are a user's push preferences. Missing settings mean everything is on.
type Settings struct {
	UserID        primitive.ObjectID `bson:"_id" json:"-"`
	Enabled       bool               `bson:"enabled" json:"enabled"` // master switch
	Likes         bool               `bson:"likes" json:"likes"`
	Comments      bool               `bson:"comments" json:"comments"`
	Replies       bool               `bson:"replies" json:"replies"`
	Mentions      bool               `bson:"mentions" json:"mentions"`
	Follows       bool               `bson:"follows" json:"follows"`
	Clones        bool               `bson:"clones" json:"clones"`
	AnchorUpdates bool               `bson:"anchorUpdates" json:"anchorUpdates"`
	UpdatedAt     time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// DefaultSettings returns the settings used for users who never changed them (all on)
func DefaultSettings(userID primitive.ObjectID) Settings {
	return Settings{
		UserID: userID, Enabled: true,
		Likes: true, Comments: true, Replies: true, Mentions: true,
		Follows: true, Clones: true, AnchorUpdates: true,
	}
}

// Allows reports whether a push of the given notification type should be sent
func (s Settings) Allows(notifType string) bool {
	if !s.Enabled {
		return false
	}
	switch notifType {
	case "like":
		return s.Likes
	case "comment":
		return s.Comments
	case "reply":
		return s.Replies
	case "mention":
		return s.Mentions
	case "follow":
		return s.Follows
	case "clone":
		return s.Clones
	case "anchor_update":
		return s.AnchorUpdates
	}
	return true
}

// Request DTOs

// RegisterDeviceRequest registers (or refreshes) this device's push token (checked by ValidateDevice)
type RegisterDeviceRequest struct {
	Token      string `json:"token"`
	Platform   string `json:"platform"`
	AppVersion string `json:"appVersion"`
}

// UnregisterDeviceRequest stops pushes to a token (sent in the body, so tokens don't end up in URL logs)
type UnregisterDeviceRequest struct {
	Token string `json:"token"`
}

// UpdateSettingsRequest changes only the fields that are sent
type UpdateSettingsRequest struct {
	Enabled       *bool `json:"enabled"`
	Likes         *bool `json:"likes"`
	Comments      *bool `json:"comments"`
	Replies       *bool `json:"replies"`
	Mentions      *bool `json:"mentions"`
	Follows       *bool `json:"follows"`
	Clones        *bool `json:"clones"`
	AnchorUpdates *bool `json:"anchorUpdates"`
}

// IsEmpty reports whether no setting was provided
func (r UpdateSettingsRequest) IsEmpty() bool {
	return r.Enabled == nil && r.Likes == nil && r.Comments == nil && r.Replies == nil &&
		r.Mentions == nil && r.Follows == nil && r.Clones == nil && r.AnchorUpdates == nil
}

// Apply copies the provided fields onto s
func (r UpdateSettingsRequest) Apply(s *Settings) {
	set := func(dst *bool, v *bool) {
		if v != nil {
			*dst = *v
		}
	}
	set(&s.Enabled, r.Enabled)
	set(&s.Likes, r.Likes)
	set(&s.Comments, r.Comments)
	set(&s.Replies, r.Replies)
	set(&s.Mentions, r.Mentions)
	set(&s.Follows, r.Follows)
	set(&s.Clones, r.Clones)
	set(&s.AnchorUpdates, r.AnchorUpdates)
}

// settingsFields maps each setting to its stored field, for partial updates
func (r UpdateSettingsRequest) fields() map[string]*bool {
	return map[string]*bool{
		"enabled": r.Enabled, "likes": r.Likes, "comments": r.Comments, "replies": r.Replies,
		"mentions": r.Mentions, "follows": r.Follows, "clones": r.Clones, "anchorUpdates": r.AnchorUpdates,
	}
}
