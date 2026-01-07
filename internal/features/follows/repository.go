package follows

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repository handles database interactions for the follows feature
type Repository struct {
	collection *mongo.Collection
}

// NewRepository creates repository and ensures indexes
func NewRepository(db *mongo.Database) *Repository {
	collection := db.Collection("follows")

	// Create indexes
	_, _ = collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			// Unique compound index - prevents duplicate follows
			Keys: bson.D{
				{Key: "followerId", Value: 1},
				{Key: "followingId", Value: 1},
			},
			Options: options.Index().SetUnique(true),
		},
		{
			// Query followers of a user (sorted by newest first)
			Keys: bson.D{
				{Key: "followingId", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},
		{
			// Query who a user follows (sorted by newest first)
			Keys: bson.D{
				{Key: "followerId", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},
	})

	return &Repository{
		collection: collection,
	}
}

// CreateFollow creates a new follow relationship
func (r *Repository) CreateFollow(ctx context.Context, followerID, followingID primitive.ObjectID) error {
	follow := &Follow{
		ID:          primitive.NewObjectID(),
		FollowerID:  followerID,
		FollowingID: followingID,
		CreatedAt:   time.Now(),
	}

	_, err := r.collection.InsertOne(ctx, follow)
	if err != nil {
		// Check if it's a duplicate key error (already following)
		if mongo.IsDuplicateKeyError(err) {
			return nil // Idempotent - already following
		}
		return err
	}

	return nil
}

// DeleteFollow removes a follow relationship
func (r *Repository) DeleteFollow(ctx context.Context, followerID, followingID primitive.ObjectID) error {
	filter := bson.M{
		"followerId":  followerID,
		"followingId": followingID,
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

// ExistsFollow checks if a follow relationship exists
func (r *Repository) ExistsFollow(ctx context.Context, followerID, followingID primitive.ObjectID) (bool, error) {
	filter := bson.M{
		"followerId":  followerID,
		"followingId": followingID,
	}

	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetFollowStatus checks both directions of follow relationship
func (r *Repository) GetFollowStatus(ctx context.Context, userID, targetID primitive.ObjectID) (isFollowing bool, isFollowedBy bool, err error) {
	// Check if user follows target
	isFollowing, err = r.ExistsFollow(ctx, userID, targetID)
	if err != nil {
		return false, false, err
	}

	// Check if target follows user
	isFollowedBy, err = r.ExistsFollow(ctx, targetID, userID)
	if err != nil {
		return false, false, err
	}

	return isFollowing, isFollowedBy, nil
}

// GetFollowers retrieves followers of a user with pagination
func (r *Repository) GetFollowers(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]Follow, int64, error) {
	filter := bson.M{"followingId": userID}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var follows []Follow
	if err = cursor.All(ctx, &follows); err != nil {
		return nil, 0, err
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return follows, total, nil
}

// GetFollowing retrieves users that a user follows with pagination
func (r *Repository) GetFollowing(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]Follow, int64, error) {
	filter := bson.M{"followerId": userID}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var follows []Follow
	if err = cursor.All(ctx, &follows); err != nil {
		return nil, 0, err
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return follows, total, nil
}

// GetFollowingIDs batch checks if user follows any of the target IDs
func (r *Repository) GetFollowingIDs(ctx context.Context, userID primitive.ObjectID, targetIDs []primitive.ObjectID) (map[primitive.ObjectID]bool, error) {
	if len(targetIDs) == 0 {
		return make(map[primitive.ObjectID]bool), nil
	}

	filter := bson.M{
		"followerId":  userID,
		"followingId": bson.M{"$in": targetIDs},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var follows []Follow
	if err = cursor.All(ctx, &follows); err != nil {
		return nil, err
	}

	// Build map of following IDs
	result := make(map[primitive.ObjectID]bool)
	for _, follow := range follows {
		result[follow.FollowingID] = true
	}

	return result, nil
}

// GetAllFollowingIDs returns all user IDs that the given user follows
func (r *Repository) GetAllFollowingIDs(ctx context.Context, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
	filter := bson.M{"followerId": userID}

	cursor, err := r.collection.Find(ctx, filter, options.Find().SetProjection(bson.M{"followingId": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var follows []Follow
	if err = cursor.All(ctx, &follows); err != nil {
		return nil, err
	}

	ids := make([]primitive.ObjectID, len(follows))
	for i, f := range follows {
		ids[i] = f.FollowingID
	}

	return ids, nil
}

// GetFollowingStatus batch checks if user is following multiple users
func (r *Repository) GetFollowingStatus(ctx context.Context, followerID primitive.ObjectID, followingIDs []primitive.ObjectID) (map[primitive.ObjectID]bool, error) {
	if len(followingIDs) == 0 {
		return make(map[primitive.ObjectID]bool), nil
	}

	cursor, err := r.collection.Find(ctx, bson.M{
		"followerId":  followerID,
		"followingId": bson.M{"$in": followingIDs},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	result := make(map[primitive.ObjectID]bool)
	for cursor.Next(ctx) {
		var follow Follow
		if err := cursor.Decode(&follow); err == nil {
			result[follow.FollowingID] = true
		}
	}

	return result, nil
}
