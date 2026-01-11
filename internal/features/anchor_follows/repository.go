package anchor_follows

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Repository struct {
	collection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	collection := db.Collection("anchor_follows")

	// Create indexes
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "anchorId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "anchorId", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "userId", Value: 1}, {Key: "createdAt", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "anchorId", Value: 1}, {Key: "notifyOnUpdate", Value: 1}},
		},
	}

	collection.Indexes().CreateMany(context.Background(), indexes)

	return &Repository{collection: collection}
}

// CreateFollow creates a new anchor follow
func (r *Repository) CreateFollow(ctx context.Context, follow *AnchorFollow) error {
	follow.ID = primitive.NewObjectID()
	follow.CreatedAt = time.Now()

	_, err := r.collection.InsertOne(ctx, follow)
	return err
}

// DeleteFollow removes a follow relationship
func (r *Repository) DeleteFollow(ctx context.Context, userID, anchorID primitive.ObjectID) error {
	filter := bson.M{
		"userId":   userID,
		"anchorId": anchorID,
	}
	_, err := r.collection.DeleteOne(ctx, filter)
	return err
}

// GetFollow gets a specific follow relationship
func (r *Repository) GetFollow(ctx context.Context, userID, anchorID primitive.ObjectID) (*AnchorFollow, error) {
	filter := bson.M{
		"userId":   userID,
		"anchorId": anchorID,
	}

	var follow AnchorFollow
	err := r.collection.FindOne(ctx, filter).Decode(&follow)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &follow, nil
}

// UpdateNotifyOnUpdate updates notification preference
func (r *Repository) UpdateNotifyOnUpdate(ctx context.Context, userID, anchorID primitive.ObjectID, notify bool) error {
	filter := bson.M{
		"userId":   userID,
		"anchorId": anchorID,
	}
	update := bson.M{
		"$set": bson.M{"notifyOnUpdate": notify},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// UpdateLastSeenVersion updates the last seen version for a follow
func (r *Repository) UpdateLastSeenVersion(ctx context.Context, userID, anchorID primitive.ObjectID, version int) error {
	filter := bson.M{
		"userId":   userID,
		"anchorId": anchorID,
	}
	update := bson.M{
		"$set": bson.M{"lastSeenVersion": version},
	}

	_, err := r.collection.UpdateOne(ctx, filter, update)
	return err
}

// GetUserFollowingAnchors gets all anchors a user is following
func (r *Repository) GetUserFollowingAnchors(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]AnchorFollow, int64, error) {
	filter := bson.M{"userId": userID}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var follows []AnchorFollow
	if err = cursor.All(ctx, &follows); err != nil {
		return nil, 0, err
	}

	return follows, total, nil
}

// GetFollowersWithNotifications gets IDs of followers who want update notifications
func (r *Repository) GetFollowersWithNotifications(ctx context.Context, anchorID primitive.ObjectID) ([]primitive.ObjectID, error) {
	filter := bson.M{
		"anchorId":       anchorID,
		"notifyOnUpdate": true,
	}

	projection := bson.M{"userId": 1}
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var ids []primitive.ObjectID
	for cursor.Next(ctx) {
		var doc struct {
			UserID primitive.ObjectID `bson:"userId"`
		}
		if err := cursor.Decode(&doc); err == nil {
			ids = append(ids, doc.UserID)
		}
	}

	return ids, nil
}

// CountFollowers counts followers for an anchor
func (r *Repository) CountFollowers(ctx context.Context, anchorID primitive.ObjectID) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"anchorId": anchorID})
}
