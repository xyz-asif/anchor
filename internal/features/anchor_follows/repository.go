package anchor_follows

import (
	"context"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Repository struct {
	collection *mongo.Collection
}

var (
	repoInstance *Repository
	repoOnce     sync.Once
)

func NewRepository(db *mongo.Database) *Repository {
	collection := db.Collection("anchor_follows")
	return &Repository{collection: collection}
}

func GetRepository(db *mongo.Database) *Repository {
	repoOnce.Do(func() {
		repoInstance = NewRepository(db)
	})
	return repoInstance
}

// GetFollowersWithNotifications returns followers who have enabled notifications for updates
func (r *Repository) GetFollowersWithNotifications(ctx context.Context, anchorID primitive.ObjectID) ([]AnchorFollow, error) {
	filter := bson.M{
		"anchorId":       anchorID,
		"notifyOnUpdate": true,
	}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	var follows []AnchorFollow
	if err = cursor.All(ctx, &follows); err != nil {
		return nil, err
	}
	return follows, nil
}

// UpdateLastSeenVersion updates the last seen version for a user's follow
func (r *Repository) UpdateLastSeenVersion(ctx context.Context, userID, anchorID primitive.ObjectID, version int) error {
	filter := bson.M{
		"userId":   userID,
		"anchorId": anchorID,
	}
	update := bson.M{
		"$set": bson.M{
			"lastSeenVersion": version,
		},
	}
	_, err := r.collection.UpdateOne(ctx, filter, update)
	return err
}

// GetUserFollowingAnchors returns anchors followed by a user
func (r *Repository) GetUserFollowingAnchors(ctx context.Context, userID primitive.ObjectID, page, limit int, sort string) ([]AnchorFollow, int64, error) {
	filter := bson.M{"userId": userID}

	findOptions := options.Find()
	findOptions.SetSkip(int64((page - 1) * limit))
	findOptions.SetLimit(int64(limit))

	// Sort logic could be enhanced but defaulting to createdAt desc for now
	findOptions.SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, 0, err
	}
	var follows []AnchorFollow
	if err = cursor.All(ctx, &follows); err != nil {
		return nil, 0, err
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return follows, total, nil
}
