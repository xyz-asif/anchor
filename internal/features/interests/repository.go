package interests

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Repository struct {
	anchorsCollection       *mongo.Collection
	likesCollection         *mongo.Collection
	anchorFollowsCollection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		anchorsCollection:       db.Collection("anchors"),
		likesCollection:         db.Collection("likes"),
		anchorFollowsCollection: db.Collection("anchor_follows"),
	}
}

// GetUserOwnAnchorTags gets tags from user's own anchors
func (r *Repository) GetUserOwnAnchorTags(ctx context.Context, userID primitive.ObjectID) ([]string, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"userId":    userID,
			"deletedAt": nil,
		}}},
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$toLower": "$tags"},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.M{"count": -1}}},
		{{Key: "$limit", Value: 20}},
	}

	cursor, err := r.anchorsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID string `bson:"_id"`
	}
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	tags := make([]string, len(results))
	for i, r := range results {
		tags[i] = r.ID
	}

	return tags, nil
}

// GetUserLikedAnchorTags gets tags from anchors user has liked
func (r *Repository) GetUserLikedAnchorTags(ctx context.Context, userID primitive.ObjectID) ([]string, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": userID}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "anchors",
			"localField":   "anchorId",
			"foreignField": "_id",
			"as":           "anchor",
		}}},
		{{Key: "$unwind", Value: "$anchor"}},
		{{Key: "$match", Value: bson.M{"anchor.deletedAt": nil}}},
		{{Key: "$unwind", Value: "$anchor.tags"}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$toLower": "$anchor.tags"},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.M{"count": -1}}},
		{{Key: "$limit", Value: 20}},
	}

	cursor, err := r.likesCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID string `bson:"_id"`
	}
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	tags := make([]string, len(results))
	for i, r := range results {
		tags[i] = r.ID
	}

	return tags, nil
}

// GetUserFollowedAnchorTags gets tags from anchors user follows
func (r *Repository) GetUserFollowedAnchorTags(ctx context.Context, userID primitive.ObjectID) ([]string, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": userID}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "anchors",
			"localField":   "anchorId",
			"foreignField": "_id",
			"as":           "anchor",
		}}},
		{{Key: "$unwind", Value: "$anchor"}},
		{{Key: "$match", Value: bson.M{"anchor.deletedAt": nil}}},
		{{Key: "$unwind", Value: "$anchor.tags"}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$toLower": "$anchor.tags"},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.M{"count": -1}}},
		{{Key: "$limit", Value: 20}},
	}

	cursor, err := r.anchorFollowsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID string `bson:"_id"`
	}
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	tags := make([]string, len(results))
	for i, r := range results {
		tags[i] = r.ID
	}

	return tags, nil
}

// GetPopularCategories gets most popular tags globally
func (r *Repository) GetPopularCategories(ctx context.Context, limit int) ([]TagCountResult, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"visibility": "public",
			"deletedAt":  nil,
		}}},
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$toLower": "$tags"},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.M{"count": -1}}},
		{{Key: "$limit", Value: limit}},
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

	var results []TagCountResult
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	if results == nil {
		results = []TagCountResult{}
	}

	return results, nil
}

// GetTagAnchorCounts gets anchor counts for specific tags
func (r *Repository) GetTagAnchorCounts(ctx context.Context, tags []string) (map[string]int, error) {
	if len(tags) == 0 {
		return make(map[string]int), nil
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"visibility": "public",
			"deletedAt":  nil,
		}}},
		{{Key: "$unwind", Value: "$tags"}},
		{{Key: "$addFields", Value: bson.M{
			"lowerTag": bson.M{"$toLower": "$tags"},
		}}},
		{{Key: "$match", Value: bson.M{
			"lowerTag": bson.M{"$in": tags},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$lowerTag",
			"count": bson.M{"$sum": 1},
		}}},
	}

	cursor, err := r.anchorsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID    string `bson:"_id"`
		Count int    `bson:"count"`
	}
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.ID] = r.Count
	}

	return counts, nil
}
