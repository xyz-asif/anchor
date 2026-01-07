package search

import (
	"context"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Repository struct {
	anchorsCollection *mongo.Collection
	usersCollection   *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		anchorsCollection: db.Collection("anchors"),
		usersCollection:   db.Collection("users"),
	}
}

// SearchAnchors performs text search on anchors
func (r *Repository) SearchAnchors(ctx context.Context, query string, tag *string, sort string, page, limit int) ([]AnchorSearchDoc, int64, error) {
	// Build filter
	filter := bson.M{
		"$text":      bson.M{"$search": query},
		"visibility": "public",
		"deletedAt":  nil,
	}

	// Add tag filter if provided
	if tag != nil && *tag != "" {
		filter["tags"] = bson.M{
			"$regex": primitive.Regex{
				Pattern: "^" + strings.ToLower(*tag) + "$",
				Options: "i",
			},
		}
	}

	// Build find options
	opts := options.Find().
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	// Set sort and projection based on sort type
	switch sort {
	case SortRecent:
		opts.SetSort(bson.D{{Key: "createdAt", Value: -1}})
	case SortPopular:
		opts.SetSort(bson.D{{Key: "engagementScore", Value: -1}, {Key: "createdAt", Value: -1}})
	default: // relevant
		opts.SetSort(bson.D{{Key: "score", Value: bson.M{"$meta": "textScore"}}})
		opts.SetProjection(bson.M{"score": bson.M{"$meta": "textScore"}})
	}

	// Execute search
	cursor, err := r.anchorsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var anchors []AnchorSearchDoc
	if err = cursor.All(ctx, &anchors); err != nil {
		return nil, 0, err
	}

	// Get total count
	total, err := r.anchorsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return anchors, total, nil
}

// SearchUsers performs text search on users
func (r *Repository) SearchUsers(ctx context.Context, query string, page, limit int) ([]UserSearchDoc, int64, error) {
	filter := bson.M{
		"$text": bson.M{"$search": query},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "score", Value: bson.M{"$meta": "textScore"}}}).
		SetProjection(bson.M{"score": bson.M{"$meta": "textScore"}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.usersCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var users []UserSearchDoc
	if err = cursor.All(ctx, &users); err != nil {
		return nil, 0, err
	}

	total, err := r.usersCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// SearchTags returns tags matching prefix with usage counts
func (r *Repository) SearchTags(ctx context.Context, prefix string, limit int) ([]TagResult, error) {
	prefixLower := strings.ToLower(prefix)

	pipeline := mongo.Pipeline{
		// Match public, non-deleted anchors with matching tag prefix
		{{Key: "$match", Value: bson.M{
			"visibility": "public",
			"deletedAt":  nil,
			"tags": bson.M{
				"$elemMatch": bson.M{
					"$regex": primitive.Regex{
						Pattern: "^" + prefixLower,
						Options: "i",
					},
				},
			},
		}}},
		// Unwind tags array
		{{Key: "$unwind", Value: "$tags"}},
		// Convert tag to lowercase for grouping
		{{Key: "$addFields", Value: bson.M{
			"lowerTag": bson.M{"$toLower": "$tags"},
		}}},
		// Filter tags by prefix (after unwind)
		{{Key: "$match", Value: bson.M{
			"lowerTag": bson.M{
				"$regex": primitive.Regex{
					Pattern: "^" + prefixLower,
					Options: "i",
				},
			},
		}}},
		// Group by lowercase tag and count
		{{Key: "$group", Value: bson.M{
			"_id":   "$lowerTag",
			"count": bson.M{"$sum": 1},
		}}},
		// Sort by count DESC
		{{Key: "$sort", Value: bson.M{"count": -1}}},
		// Limit results
		{{Key: "$limit", Value: limit}},
		// Project final shape
		{{Key: "$project", Value: bson.M{
			"_id":   0,
			"name":  "$_id",
			"count": 1,
		}}},
	}

	cursor, err := r.anchorsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []TagResult
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	if results == nil {
		results = []TagResult{}
	}

	return results, nil
}
