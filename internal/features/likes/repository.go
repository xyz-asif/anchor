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
// CreateLike inserts a like. created is false when the user had already liked the anchor.
func (r *Repository) CreateLike(ctx context.Context, anchorID, userID primitive.ObjectID) (created bool, err error) {
	like := &Like{
		ID:        primitive.NewObjectID(),
		AnchorID:  anchorID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}

	if _, err := r.collection.InsertOne(ctx, like); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return false, nil // already liked (unique index on anchorId+userId)
		}
		return false, err
	}
	return true, nil
}

// DeleteLike removes a like relationship
// DeleteLike removes a like. deleted is false when there was no like to remove.
func (r *Repository) DeleteLike(ctx context.Context, anchorID, userID primitive.ObjectID) (deleted bool, err error) {
	filter := bson.M{
		"anchorId": anchorID,
		"userId":   userID,
	}

	result, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return false, err
	}
	return result.DeletedCount > 0, nil
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

// GetLikers retrieves users who liked an anchor with pagination. Likers in hidden (block
// relationships with the viewer) are excluded in the query, so totals and pages match.
func (r *Repository) GetLikers(ctx context.Context, anchorID primitive.ObjectID, hidden []primitive.ObjectID, page, limit int) ([]Like, int64, error) {
	filter := bson.M{"anchorId": anchorID}
	if len(hidden) > 0 {
		filter["userId"] = bson.M{"$nin": hidden}
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64(page-1) * int64(limit)).
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
func (r *Repository) GetRecentLikers(ctx context.Context, anchorID primitive.ObjectID, hidden []primitive.ObjectID, limit int) ([]Like, error) {
	filter := bson.M{"anchorId": anchorID}
	if len(hidden) > 0 {
		filter["userId"] = bson.M{"$nin": hidden}
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
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
// GetVisibleUserLikes returns a page of likerID's likes, newest first, counting and paging only
// likes whose anchor the viewer may see: not deleted, public or owned by the viewer, and not by
// an author in hiddenAuthors. Filtering in the query (not after paging) keeps pages full and
// keeps the total from revealing how many hidden anchors the user liked.
func (r *Repository) GetVisibleUserLikes(ctx context.Context, likerID primitive.ObjectID, viewerID *primitive.ObjectID, hiddenAuthors []primitive.ObjectID, page, limit int) ([]Like, int64, error) {
	visible := bson.A{bson.M{"visibility": "public"}}
	if viewerID != nil {
		visible = append(visible, bson.M{"userId": *viewerID})
	}
	anchorFilter := bson.M{"deletedAt": nil, "$or": visible}
	if len(hiddenAuthors) > 0 {
		anchorFilter["userId"] = bson.M{"$nin": hiddenAuthors}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": likerID}}},
		{{Key: "$sort", Value: bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}}},
		{{Key: "$lookup", Value: bson.M{
			"from": "anchors",
			"let":  bson.M{"aid": "$anchorId"},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{"$expr": bson.M{"$eq": bson.A{"$_id", "$$aid"}}}},
				bson.M{"$match": anchorFilter},
				bson.M{"$project": bson.M{"_id": 1}},
			},
			"as": "visibleAnchor",
		}}},
		{{Key: "$match", Value: bson.M{"visibleAnchor.0": bson.M{"$exists": true}}}},
		{{Key: "$project", Value: bson.M{"visibleAnchor": 0}}},
		{{Key: "$facet", Value: bson.M{
			"items": bson.A{bson.M{"$skip": int64(page-1) * int64(limit)}, bson.M{"$limit": int64(limit)}},
			"total": bson.A{bson.M{"$count": "n"}},
		}}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var result []struct {
		Items []Like `bson:"items"`
		Total []struct {
			N int64 `bson:"n"`
		} `bson:"total"`
	}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, 0, err
	}
	if len(result) == 0 {
		return []Like{}, 0, nil
	}
	var total int64
	if len(result[0].Total) > 0 {
		total = result[0].Total[0].N
	}
	return result[0].Items, total, nil
}

func (r *Repository) GetUserLikedAnchorsPaginated(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]Like, int64, error) {
	filter := bson.M{"userId": userID}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64(page-1) * int64(limit)).
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

// GetLikersAmong returns, for each anchor, up to perAnchor of its most recent likers who are in
// likerIDs (for "liked by people you follow"), newest first, in one query for the whole page.
// Every like is considered, not just the latest few, so an older like by a friend still shows.
func (r *Repository) GetLikersAmong(ctx context.Context, anchorIDs, likerIDs []primitive.ObjectID, perAnchor int) (map[primitive.ObjectID][]primitive.ObjectID, error) {
	result := make(map[primitive.ObjectID][]primitive.ObjectID)
	if len(anchorIDs) == 0 || len(likerIDs) == 0 || perAnchor < 1 {
		return result, nil
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"anchorId": bson.M{"$in": anchorIDs}, "userId": bson.M{"$in": likerIDs}}}},
		{{Key: "$group", Value: bson.M{
			"_id": "$anchorId",
			"users": bson.M{"$topN": bson.M{
				"n":      perAnchor,
				"sortBy": bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}},
				"output": "$userId",
			}},
		}}},
	}
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var groups []struct {
		AnchorID primitive.ObjectID   `bson:"_id"`
		Users    []primitive.ObjectID `bson:"users"`
	}
	if err := cursor.All(ctx, &groups); err != nil {
		return nil, err
	}
	for _, g := range groups {
		result[g.AnchorID] = g.Users
	}
	return result, nil
}
