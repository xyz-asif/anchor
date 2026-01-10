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
	Status     string             `bson:"status" json:"status"` // "pending", "reviewed"
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time          `bson:"updatedAt" json:"updatedAt"`
}

type CreateReportRequest struct {
	TargetID   string `json:"targetId" binding:"required"`
	TargetType string `json:"targetType" binding:"required,oneof=anchor item user"`
	Reason     string `json:"reason" binding:"required,min=5,max=500"`
}

type BlockUserRequest struct {
	// User ID to block is passed in path parameter
}
