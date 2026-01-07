package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repository handles database interactions for the auth feature
type Repository struct {
	collection *mongo.Collection
}

// NewRepository initializes the repository and creates necessary indexes
func NewRepository(db *mongo.Database) *Repository {
	collection := db.Collection("users")

	// Create indexes
	_, _ = collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "googleId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "username", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			// Text index for search
			Keys: bson.D{
				{Key: "username", Value: "text"},
				{Key: "displayName", Value: "text"},
				{Key: "bio", Value: "text"},
			},
			Options: options.Index().
				SetWeights(bson.D{
					{Key: "username", Value: 10},
					{Key: "displayName", Value: 5},
					{Key: "bio", Value: 1},
				}).
				SetName("user_text_search"),
		},
	})

	return &Repository{collection: collection}
}

// CreateUser inserts a new user into the database
func (r *Repository) CreateUser(ctx context.Context, user *User) error {
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	result, err := r.collection.InsertOne(ctx, user)
	if err != nil {
		// Check for duplicate key error (code 11000)
		if mongo.IsDuplicateKeyError(err) {
			// Return the original error wrapped so we can see which key was duplicated in logs
			return fmt.Errorf("user duplicate key error: %w", err)
		}
		return err
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		user.ID = oid
	}

	return nil
}

// GetUserByGoogleID finds a user by their Google ID
func (r *Repository) GetUserByGoogleID(ctx context.Context, googleID string) (*User, error) {
	var user User
	err := r.collection.FindOne(ctx, bson.M{"googleId": googleID}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil // Not found is not an error here
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail finds a user by their email address
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := r.collection.FindOne(ctx, bson.M{"email": email}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByUsername finds a user by their username
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := r.collection.FindOne(ctx, bson.M{"username": username}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByID finds a user by their MongoDB ID
func (r *Repository) GetUserByID(ctx context.Context, userID string) (*User, error) {
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, errors.New("invalid user id format")
	}

	var user User
	err = r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&user)
	if err != nil {
		return nil, err // Return error if not found as per requirement
	}
	return &user, nil
}

// UpdateUser updates specific fields of a user
func (r *Repository) UpdateUser(ctx context.Context, userID string, updates map[string]interface{}) error {
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return errors.New("invalid user id format")
	}

	// Always update UpdatedAt
	updates["updatedAt"] = time.Now()

	filter := bson.M{"_id": oid}
	update := bson.M{"$set": updates}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("user not found")
	}

	return nil
}

// UsernameExists checks if a username is already taken
func (r *Repository) UsernameExists(ctx context.Context, username string) (bool, error) {
	count, err := r.collection.CountDocuments(ctx, bson.M{"username": username})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IncrementAnchorCount increments or decrements the user's anchor count
func (r *Repository) IncrementAnchorCount(ctx context.Context, userID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": userID}
	update := bson.M{
		"$inc": bson.M{"anchorCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("user not found")
	}

	return nil
}

// IncrementFollowerCount increments or decrements a user's follower count
func (r *Repository) IncrementFollowerCount(ctx context.Context, userID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": userID}
	update := bson.M{
		"$inc": bson.M{"followerCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("user not found")
	}

	return nil
}

// IncrementFollowingCount increments or decrements a user's following count
func (r *Repository) IncrementFollowingCount(ctx context.Context, userID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": userID}
	update := bson.M{
		"$inc": bson.M{"followingCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("user not found")
	}

	return nil
}

// GetUsersByIDs fetches multiple users by their IDs (for enriching follow lists)
func (r *Repository) GetUsersByIDs(ctx context.Context, userIDs []primitive.ObjectID) ([]User, error) {
	if len(userIDs) == 0 {
		return []User{}, nil
	}

	filter := bson.M{"_id": bson.M{"$in": userIDs}}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []User
	if err = cursor.All(ctx, &users); err != nil {
		return nil, err
	}

	return users, nil
}

// GetUserByObjectID finds a user by their ObjectID directly
func (r *Repository) GetUserByObjectID(ctx context.Context, userID primitive.ObjectID) (*User, error) {
	var user User
	err := r.collection.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

// GetUsersByUsernames retrieves users by their usernames (for mention validation)
func (r *Repository) GetUsersByUsernames(ctx context.Context, usernames []string) ([]User, error) {
	if len(usernames) == 0 {
		return []User{}, nil
	}

	// Normalize to lowercase
	normalizedUsernames := make([]string, len(usernames))
	for i, u := range usernames {
		normalizedUsernames[i] = strings.ToLower(u)
	}

	filter := bson.M{
		"username": bson.M{"$in": normalizedUsernames},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []User
	if err = cursor.All(ctx, &users); err != nil {
		return nil, err
	}

	return users, nil
}

// GetUserIDsByUsernames returns map of username -> userID for valid usernames
func (r *Repository) GetUserIDsByUsernames(ctx context.Context, usernames []string) (map[string]primitive.ObjectID, error) {
	users, err := r.GetUsersByUsernames(ctx, usernames)
	if err != nil {
		return nil, err
	}

	result := make(map[string]primitive.ObjectID)
	for _, user := range users {
		result[strings.ToLower(user.Username)] = user.ID
	}

	return result, nil
}
