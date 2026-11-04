package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/xyz-asif/gotodo/internal/pkg/searchterms"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repository handles database interactions for the auth feature
// cleanupLegacySessions makes the legacy-session cleanup run once per process
var cleanupLegacySessions sync.Once

// ErrUserNotFound is returned when no user matches; other errors mean the lookup itself failed
var ErrUserNotFound = errors.New("user not found")

// ErrUsernameAlreadyChanged is returned when a user who already used their one username change tries again
var ErrUsernameAlreadyChanged = errors.New("username can only be changed once")

type Repository struct {
	collection              *mongo.Collection
	refreshTokensCollection *mongo.Collection
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
		{
			// Prefix-word index for search (see pkg/searchterms)
			Keys: bson.D{{Key: "search.terms", Value: 1}},
		},
		{
			// "who has blocked me" lookups (multikey index over the array)
			Keys: bson.D{{Key: "blockedUsers", Value: 1}},
		},
	})

	refreshTokensCollection := db.Collection("refresh_tokens")
	// Create indexes for refresh tokens
	_, _ = refreshTokensCollection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tokenId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "userId", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "expiresAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0), // TTL index
		},
	})

	// Sessions saved before tokens carried a unique ID all have tokenId "" and can never be
	// matched to a token again. Remove them once per process so they stop blocking the unique index.
	cleanupLegacySessions.Do(func() {
		// Users created before this fix stored blockedUsers as null, and MongoDB can't
		// $addToSet/$pull on null. Normalise them to empty lists once per process.
		_, _ = collection.UpdateMany(context.Background(), bson.M{"blockedUsers": nil}, bson.M{"$set": bson.M{"blockedUsers": bson.A{}}})

		res, err := refreshTokensCollection.DeleteMany(context.Background(), bson.M{"tokenId": bson.M{"$in": []interface{}{"", nil}}})
		if err == nil && res.DeletedCount > 0 {
			log.Printf("Removed %d legacy refresh token sessions without a token ID", res.DeletedCount)
		}
	})

	return &Repository{
		collection:              collection,
		refreshTokensCollection: refreshTokensCollection,
	}
}

// CreateUser inserts a new user into the database
func (r *Repository) CreateUser(ctx context.Context, user *User) error {
	if user.BlockedUsers == nil {
		user.BlockedUsers = []primitive.ObjectID{} // stored as [] so $addToSet/$pull work
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	user.Search = searchterms.ForUser(user.Username, user.DisplayName)

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

	return r.GetUserByObjectID(ctx, oid)
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

	_, name := updates["displayName"]
	_, username := updates["username"]
	if name || username {
		r.refreshUserSearch(ctx, oid)
	}
	return nil
}

// refreshUserSearch recomputes a user's search terms from their current names. The write only
// applies if the names are still what was read, so it can't overwrite a newer edit's terms.
// Best effort: a failure only means search lags until the next edit or the startup backfill.
func (r *Repository) refreshUserSearch(ctx context.Context, userID primitive.ObjectID) {
	var u struct {
		Username    string `bson:"username"`
		DisplayName string `bson:"displayName"`
	}
	err := r.collection.FindOne(ctx, bson.M{"_id": userID},
		options.FindOne().SetProjection(bson.M{"username": 1, "displayName": 1})).Decode(&u)
	if err == nil {
		_, err = r.collection.UpdateOne(ctx,
			bson.M{"_id": userID, "username": u.Username, "displayName": u.DisplayName},
			bson.M{"$set": bson.M{"search": searchterms.ForUser(u.Username, u.DisplayName)}})
	}
	if err != nil {
		log.Printf("auth: refreshing search terms of %s failed: %v", userID.Hex(), err)
	}
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
	if delta < 0 {
		// never let the counter go below zero
		filter["anchorCount"] = bson.M{"$gte": -delta}
	}
	update := bson.M{
		"$inc": bson.M{"anchorCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 && delta > 0 {
		return errors.New("user not found")
	}

	return nil
}

// IncrementFollowerCount increments or decrements a user's follower count
func (r *Repository) IncrementFollowerCount(ctx context.Context, userID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": userID}
	if delta < 0 {
		// never let the counter go below zero
		filter["followerCount"] = bson.M{"$gte": -delta}
	}
	update := bson.M{
		"$inc": bson.M{"followerCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 && delta > 0 {
		return errors.New("user not found")
	}

	return nil
}

// IncrementFollowingCount increments or decrements a user's following count
func (r *Repository) IncrementFollowingCount(ctx context.Context, userID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": userID}
	if delta < 0 {
		// never let the counter go below zero
		filter["followingCount"] = bson.M{"$gte": -delta}
	}
	update := bson.M{
		"$inc": bson.M{"followingCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 && delta > 0 {
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
			return nil, ErrUserNotFound
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

// SetUsernameOnce sets a new username only if the user hasn't used their one change yet.
// The check and the update are one atomic operation, so two concurrent requests can't both succeed.
func (r *Repository) SetUsernameOnce(ctx context.Context, userID primitive.ObjectID, username string) error {
	now := time.Now()
	result, err := r.collection.UpdateOne(ctx,
		bson.M{"_id": userID, "usernameChanged": bson.M{"$ne": true}},
		bson.M{"$set": bson.M{
			"username":          username,
			"usernameChanged":   true,
			"usernameChangedAt": now,
			"updatedAt":         now,
		}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrUsernameAlreadyChanged
	}
	r.refreshUserSearch(ctx, userID)
	return nil
}

// SwapUserImage atomically sets an image URL/public-ID pair on the user and returns the public ID
// it replaced, so the caller deletes exactly the asset that is no longer referenced (even when
// two uploads race). Pass empty strings to clear the image.
func (r *Repository) SwapUserImage(ctx context.Context, userID primitive.ObjectID, urlField, publicIDField, url, publicID string) (string, error) {
	var before User
	err := r.collection.FindOneAndUpdate(ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{urlField: url, publicIDField: publicID, "updatedAt": time.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.Before),
	).Decode(&before)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", ErrUserNotFound
		}
		return "", err
	}
	switch publicIDField {
	case "profilePicturePublicId":
		return before.ProfilePicturePublicID, nil
	case "coverImagePublicId":
		return before.CoverImagePublicID, nil
	}
	return "", nil
}

// SessionActive reports whether the refresh session with this token ID still exists and hasn't
// expired. Logout, revoke-all and account deletion remove sessions.
func (r *Repository) SessionActive(ctx context.Context, tokenID string) (bool, error) {
	n, err := r.refreshTokensCollection.CountDocuments(ctx, bson.M{
		"tokenId":   tokenID,
		"revoked":   bson.M{"$ne": true},
		"expiresAt": bson.M{"$gt": time.Now()}, // the TTL monitor can lag by up to a minute
	}, options.Count().SetLimit(1))
	return n > 0, err
}

// SaveRefreshToken stores a new refresh token session
func (r *Repository) SaveRefreshToken(ctx context.Context, session *RefreshTokenSession) error {
	_, err := r.refreshTokensCollection.InsertOne(ctx, session)
	return err
}

// GetRefreshToken retrieves a refresh token session by its TokenID (JTI)
func (r *Repository) GetRefreshToken(ctx context.Context, tokenID string) (*RefreshTokenSession, error) {
	var session RefreshTokenSession
	err := r.refreshTokensCollection.FindOne(ctx, bson.M{"tokenId": tokenID}).Decode(&session)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// RevokeRefreshToken revokes a specific refresh token (marks as revoked or deletes)
// We'll delete it since we have revocation logs if needed, or just standard logout behavior.
// If we want to keep history, we'd mark revoked=true. But TTL will kill it anyway.
// Let's delete it for "Logout".
func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenID string) error {
	if _, err := r.refreshTokensCollection.DeleteOne(ctx, bson.M{"tokenId": tokenID}); err != nil {
		return err
	}
	// This phone was signed out: stop its pushes ("devices" is the push package's collection;
	// devices are tied to the session that registered them)
	_, err := r.collection.Database().Collection("devices").DeleteMany(ctx, bson.M{"sessionId": tokenID})
	return err
}

// RevokeAllUserTokens revokes all refresh tokens for a user (Logout All Devices)
func (r *Repository) RevokeAllUserTokens(ctx context.Context, userID primitive.ObjectID) error {
	if _, err := r.refreshTokensCollection.DeleteMany(ctx, bson.M{"userId": userID}); err != nil {
		return err
	}
	// Signed out everywhere: no more pushes to any of the user's devices
	// ("devices" is the push package's collection; it can't be imported here)
	if _, err := r.collection.Database().Collection("devices").DeleteMany(ctx, bson.M{"userId": userID}); err != nil {
		return err
	}
	// Also invalidate already-issued access tokens (checked by the auth middleware)
	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{"$set": bson.M{"tokensRevokedAt": time.Now()}})
	return err
}

// DeleteUser permanently removes a user from the database
func (r *Repository) DeleteUser(ctx context.Context, userID primitive.ObjectID) error {
	// Also delete all refresh tokens
	_ = r.RevokeAllUserTokens(ctx, userID)
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": userID})
	return err
}

// IsBlockedBetween reports whether either user has blocked the other.
// Used to stop blocked users from following, liking, commenting on or cloning each other's content.
func (r *Repository) IsBlockedBetween(ctx context.Context, me *User, otherID primitive.ObjectID) (bool, error) {
	if me.HasBlocked(otherID) {
		return true, nil
	}
	n, err := r.collection.CountDocuments(ctx, bson.M{"_id": otherID, "blockedUsers": me.ID}, options.Count().SetLimit(1))
	return n > 0, err
}

// GetBlockerIDs returns the IDs of users who have blocked userID
func (r *Repository) GetBlockerIDs(ctx context.Context, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
	values, err := r.collection.Distinct(ctx, "_id", bson.M{"blockedUsers": userID})
	if err != nil {
		return nil, err
	}
	ids := make([]primitive.ObjectID, 0, len(values))
	for _, v := range values {
		if id, ok := v.(primitive.ObjectID); ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// HiddenUserIDs returns the users whose content must be hidden from viewer: those the viewer
// blocked and those who blocked the viewer. A nil viewer (anonymous) hides nobody.
func (r *Repository) HiddenUserIDs(ctx context.Context, viewer *User) ([]primitive.ObjectID, error) {
	if viewer == nil {
		return nil, nil
	}
	blockers, err := r.GetBlockerIDs(ctx, viewer.ID)
	if err != nil {
		return nil, err
	}
	return append(append([]primitive.ObjectID{}, viewer.BlockedUsers...), blockers...), nil
}

// CanViewProfile reports whether viewer (nil = anonymous) may see target's profile and lists.
// A user who blocked the viewer is invisible to them; the viewer can still see users they
// blocked themselves (so they can find and unblock them).
func CanViewProfile(target, viewer *User) bool {
	return viewer == nil || viewer.ID == target.ID || !target.HasBlocked(viewer.ID)
}

// UserHasBlocked reports whether blockerID has blocked blockedID (one direction only)
func (r *Repository) UserHasBlocked(ctx context.Context, blockerID, blockedID primitive.ObjectID) (bool, error) {
	n, err := r.collection.CountDocuments(ctx, bson.M{"_id": blockerID, "blockedUsers": blockedID}, options.Count().SetLimit(1))
	return n > 0, err
}
