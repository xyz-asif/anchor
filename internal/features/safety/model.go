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
}

// Report statuses
const (
	ReportStatusPending   = "pending"   // new, not yet looked at
	ReportStatusReviewed  = "reviewed"  // looked at, decision pending / no action needed yet
	ReportStatusActioned  = "actioned"  // content removed or user dealt with
	ReportStatusDismissed = "dismissed" // report was not valid
)

// UpdateReportRequest is the admin payload for moderating a report
type UpdateReportRequest struct {
	Status string `json:"status" binding:"required,oneof=pending reviewed actioned dismissed"`
	Note   string `json:"note" binding:"omitempty,max=1000"`
}

// ReportListQuery holds admin list filters
type ReportListQuery struct {
	Status     string `form:"status" binding:"omitempty,oneof=pending reviewed actioned dismissed"`
	TargetType string `form:"targetType" binding:"omitempty,oneof=anchor item user"`
	Page       int    `form:"page,default=1" binding:"min=1"`
	Limit      int    `form:"limit,default=20" binding:"min=1,max=100"`
}

type CreateReportRequest struct {
	TargetID   string `json:"targetId" binding:"required"`
	TargetType string `json:"targetType" binding:"required,oneof=anchor item user"`
	Reason     string `json:"reason" binding:"required,min=5,max=500"`
}

type BlockUserRequest struct {
	// User ID to block is passed in path parameter
}

// SetVerificationRequest is the admin payload for granting/removing the verified badge
type SetVerificationRequest struct {
	IsVerified *bool `json:"isVerified" binding:"required"`
}
