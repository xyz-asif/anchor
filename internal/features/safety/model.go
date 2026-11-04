package safety

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Report struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ReporterID primitive.ObjectID `bson:"reporterId" json:"reporterId"`
	TargetID   primitive.ObjectID `bson:"targetId" json:"targetId"`
	TargetType string             `bson:"targetType" json:"targetType"` // "anchor", "item", "user"
	Reason     string             `bson:"reason" json:"reason"`
	Status     string             `bson:"status" json:"status"` // see ReportStatus* constants
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time          `bson:"updatedAt" json:"updatedAt"`

	// Moderation (set by an admin)
	ReviewedBy     *primitive.ObjectID `bson:"reviewedBy,omitempty" json:"reviewedBy,omitempty"`
	ReviewedAt     *time.Time          `bson:"reviewedAt,omitempty" json:"reviewedAt,omitempty"`
	ResolutionNote string              `bson:"resolutionNote,omitempty" json:"resolutionNote,omitempty"`

	// Open is true while pending or reviewed; one open report per reporter and target
	Open bool `bson:"open" json:"-"`
}

// Report target types
const (
	TargetAnchor  = "anchor"
	TargetItem    = "item"
	TargetComment = "comment"
	TargetUser    = "user"
)

// Report statuses
const (
	ReportStatusPending   = "pending"   // new, not yet looked at
	ReportStatusReviewed  = "reviewed"  // looked at, decision pending / no action needed yet
	ReportStatusActioned  = "actioned"  // content removed or user dealt with
	ReportStatusDismissed = "dismissed" // report was not valid
)

// UpdateReportRequest is the admin payload for moderating a report (checked by the handler)
type UpdateReportRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

// ReportListQuery holds admin list filters (parsed by parseReportListQuery)
type ReportListQuery struct {
	Status     string
	TargetType string
	Page       int
	Limit      int
}

// CreateReportRequest reports an anchor, item, comment or user (checked by the handler)
type CreateReportRequest struct {
	TargetID   string `json:"targetId"`
	TargetType string `json:"targetType"`
	Reason     string `json:"reason"`
}

type BlockUserRequest struct {
	// User ID to block is passed in path parameter
}

// SetVerificationRequest is the admin payload for granting/removing the verified badge
type SetVerificationRequest struct {
	IsVerified *bool `json:"isVerified" binding:"required"` // true or false
}
