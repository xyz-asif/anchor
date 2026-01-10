package anchor_follows

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AnchorFollow struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID          primitive.ObjectID `bson:"userId" json:"userId"`
	AnchorID        primitive.ObjectID `bson:"anchorId" json:"anchorId"`
	NotifyOnUpdate  bool               `bson:"notifyOnUpdate" json:"notifyOnUpdate"`
	LastSeenVersion int                `bson:"lastSeenVersion" json:"lastSeenVersion"`
	CreatedAt       time.Time          `bson:"createdAt" json:"createdAt"`
}
