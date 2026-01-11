package likes

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repository handles database interactions for the likes feature
type Repository struct {
	collection *mongo.Collection
}

// NewRepository creates repository and ensures indexes
func NewRepository(db *mongo.Database) *Repository {
	collection := db.Collection("likes")

	// Create indexes
	_, _ = collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			// Unique compound index - prevents duplicate likes
			Keys: bson.D{
				{Key: "anchorId", Value: 1},
				{Key: "userId", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			// Query likes for an anchor (sorted by recent first)
			Keys: bson.D{
				{Key: "anchorId", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},
		{
			// Query anchors a user has liked (for "liked anchors" page - future)
			Keys: bson.D{
				{Key: "userId", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},
	})

	return &Repository{
		collection: collection,
	}
}

// CreateLike creates a new like relationship
func (r *Repository) CreateLike(ctx context.Context, anchorID, userID primitive.ObjectID) error {
	like := &Like{
		ID:        primitive.NewObjectID(),
		AnchorID:  anchorID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}

	_, err := r.collection.InsertOne(ctx, like)
	if err != nil {
		// Check if it's a duplicate key error (already liked)
		if mongo.IsDuplicateKeyError(err) {
			return nil // Idempotent - already liked
		}
		return err
	}

	return nil
}

// DeleteLike removes a like relationship
func (r *Repository) DeleteLike(ctx context.Context, anchorID, userID primitive.ObjectID) error {
	filter := bson.M{
		"anchorId": anchorID,
		"userId":   userID,
	}

	result, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	// Idempotent - if not found, still return success
	if result.DeletedCount == 0 {
		return nil
	}

	return nil
}

// ExistsLike checks if a like relationship exists
func (r *Repository) ExistsLike(ctx context.Context, anchorID, userID primitive.ObjectID) (bool, error) {
	filter := bson.M{
		"anchorId": anchorID,
		"userId":   userID,
	}

	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetLikers retrieves users who liked an anchor with pagination
func (r *Repository) GetLikers(ctx context.Context, anchorID primitive.ObjectID, page, limit int) ([]Like, int64, error) {
	filter := bson.M{"anchorId": anchorID}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var likes []Like
	if err = cursor.All(ctx, &likes); err != nil {
		return nil, 0, err
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return likes, total, nil
}

// GetRecentLikers retrieves recent likers (for like summary)
func (r *Repository) GetRecentLikers(ctx context.Context, anchorID primitive.ObjectID, limit int) ([]Like, error) {
	filter := bson.M{"anchorId": anchorID}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var likes []Like
	if err = cursor.All(ctx, &likes); err != nil {
		return nil, err
	}

	return likes, nil
}

// GetUserLikedAnchors batch checks if user liked any of the anchor IDs
func (r *Repository) GetUserLikedAnchors(ctx context.Context, userID primitive.ObjectID, anchorIDs []primitive.ObjectID) (map[primitive.ObjectID]bool, error) {
	if len(anchorIDs) == 0 {
		return make(map[primitive.ObjectID]bool), nil
	}

	filter := bson.M{
		"userId":   userID,
		"anchorId": bson.M{"$in": anchorIDs},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var likes []Like
	if err = cursor.All(ctx, &likes); err != nil {
		return nil, err
	}

	// Build map of liked anchor IDs
	result := make(map[primitive.ObjectID]bool)
	for _, like := range likes {
		result[like.AnchorID] = true
	}

	return result, nil
}

// GetUserLikedAnchorsPaginated returns anchors liked by a user with pagination
func (r *Repository) GetUserLikedAnchorsPaginated(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]Like, int64, error) {
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

	var likes []Like
	if err = cursor.All(ctx, &likes); err != nil {
		return nil, 0, err
	}

	return likes, total, nil
}
