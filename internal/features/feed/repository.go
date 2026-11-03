package feed

import (
	"context"
	"time"

	"github.com/xyz-asif/gotodo/internal/features/anchors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Repository struct {
	anchorsCollection *mongo.Collection
	itemsCollection   *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	repo := &Repository{
		anchorsCollection: db.Collection("anchors"),
		itemsCollection:   db.Collection("items"),
	}
	repo.ensureIndexes()
	return repo
}

func (r *Repository) ensureIndexes() {
	// Compound index for feed query optimization
	indexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "userId", Value: 1},
			{Key: "visibility", Value: 1},
			{Key: "deletedAt", Value: 1},
			{Key: "lastItemAddedAt", Value: -1},
			{Key: "_id", Value: -1},
		},
	}
	r.anchorsCollection.Indexes().CreateOne(context.Background(), indexModel)
}

// GetFeedAnchors retrieves anchors for the feed with pagination
func (r *Repository) GetFeedAnchors(
	ctx context.Context,
	userIDs []primitive.ObjectID,
	excludedUserIDs []primitive.ObjectID, // blocked and muted users
	cursor *FeedCursor,
	limit int,
) ([]anchors.Anchor, error) {
	// Base filter: Authors in userIDs, public only (unlisted anchors are link-only), not deleted
	filter := bson.M{
		"userId":     bson.M{"$in": userIDs},
		"visibility": "public",
		"deletedAt":  nil,
	}

	// Drop blocked and muted authors
	if len(excludedUserIDs) > 0 {
		filter["userId"] = bson.M{"$in": userIDs, "$nin": excludedUserIDs}
	}

	// Apply cursor pagination if present
	if cursor != nil {
		filter["$or"] = []bson.M{
			{"lastItemAddedAt": bson.M{"$lt": cursor.Timestamp}},
			{
				"lastItemAddedAt": cursor.Timestamp,
				"_id":             bson.M{"$lt": cursor.AnchorID},
			},
		}
	}

	// Sort by lastItemAddedAt DESC, _id DESC
	opts := options.Find().
		SetSort(bson.D{
			{Key: "lastItemAddedAt", Value: -1},
			{Key: "_id", Value: -1},
		}).
		SetLimit(int64(limit))

	mongoCursor, err := r.anchorsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer mongoCursor.Close(ctx)

	var results []anchors.Anchor
	if err = mongoCursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// GetPreviewItems retrieves the first N items for an anchor for preview
func (r *Repository) GetPreviewItems(
	ctx context.Context,
	anchorID primitive.ObjectID,
	limit int,
) ([]anchors.Item, error) {
	filter := bson.M{
		"anchorId":  anchorID,
		"deletedAt": nil,
	}

	opts := options.Find().
		SetSort(bson.M{"position": 1}).
		SetLimit(int64(limit))

	cursor, err := r.itemsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []anchors.Item
	if err = cursor.All(ctx, &items); err != nil {
		return nil, err
	}

	return items, nil
}

// GetUserClonedAnchors returns a map of anchor IDs that the user has cloned from the specific list
func (r *Repository) GetUserClonedAnchors(
	ctx context.Context,
	userID primitive.ObjectID,
	anchorIDs []primitive.ObjectID,
) (map[primitive.ObjectID]bool, error) {
	filter := bson.M{
		"userId":             userID,
		"clonedFromAnchorId": bson.M{"$in": anchorIDs},
		"deletedAt":          nil,
	}

	cursor, err := r.anchorsCollection.Find(ctx, filter, options.Find().SetProjection(bson.M{"clonedFromAnchorId": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	clonedMap := make(map[primitive.ObjectID]bool)
	for cursor.Next(ctx) {
		var result struct {
			ClonedFromAnchorID primitive.ObjectID `bson:"clonedFromAnchorId"`
		}
		if err := cursor.Decode(&result); err == nil {
			clonedMap[result.ClonedFromAnchorID] = true
		}
	}

	return clonedMap, nil
}

// GetDiscoverAnchors retrieves anchors for discovery feed
func (r *Repository) GetDiscoverAnchors(
	ctx context.Context,
	excludeUserIDs []primitive.ObjectID, // followed users, self, blocked and muted users
	hiddenAnchorIDs []primitive.ObjectID, // anchors the viewer marked "not interested"
	interestTags []string, // for_you: only anchors with at least one of these tags
	category string,
	tag *string,
	cursor *DiscoverCursor,
	limit int,
) ([]anchors.Anchor, error) {
	// Base filter: public only (NOT unlisted), not deleted
	filter := bson.M{
		"visibility": "public",
		"deletedAt":  nil,
	}

	if len(excludeUserIDs) > 0 {
		filter["userId"] = bson.M{"$nin": excludeUserIDs}
	}
	if len(hiddenAnchorIDs) > 0 {
		filter["_id"] = bson.M{"$nin": hiddenAnchorIDs}
	}

	// Tag filter (exact tag) and interest filter (any of the tags) can both apply
	tagConditions := bson.M{}
	if tag != nil && *tag != "" {
		tagConditions["$all"] = []string{*tag}
	}
	if len(interestTags) > 0 {
		tagConditions["$in"] = interestTags
	}
	if len(tagConditions) > 0 {
		filter["tags"] = tagConditions
	}

	// Category-specific logic
	var sort bson.D

	switch category {
	case "trending":
		// Only last 48 hours
		cutoff := time.Now().Add(-48 * time.Hour)
		filter["createdAt"] = bson.M{"$gte": cutoff}
		sort = bson.D{
			{Key: "engagementScore", Value: -1},
			{Key: "createdAt", Value: -1},
			{Key: "_id", Value: -1},
		}
	case "popular", "for_you":
		sort = bson.D{
			{Key: "engagementScore", Value: -1},
			{Key: "createdAt", Value: -1},
			{Key: "_id", Value: -1},
		}
	case "recent":
		sort = bson.D{
			{Key: "createdAt", Value: -1},
			{Key: "_id", Value: -1},
		}
	}

	// Apply cursor pagination
	if cursor != nil {
		if category == "recent" {
			filter["$or"] = []bson.M{
				{"createdAt": bson.M{"$lt": cursor.CreatedAt}},
				{
					"createdAt": cursor.CreatedAt,
					"_id":       bson.M{"$lt": cursor.AnchorID},
				},
			}
		} else {
			// trending/popular - use score
			if cursor.Score != nil {
				filter["$or"] = []bson.M{
					{"engagementScore": bson.M{"$lt": *cursor.Score}},
					{
						"engagementScore": *cursor.Score,
						"createdAt":       bson.M{"$lt": cursor.CreatedAt},
					},
					{
						"engagementScore": *cursor.Score,
						"createdAt":       cursor.CreatedAt,
						"_id":             bson.M{"$lt": cursor.AnchorID},
					},
				}
			}
		}
	}

	opts := options.Find().SetSort(sort).SetLimit(int64(limit))

	mongoCursor, err := r.anchorsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer mongoCursor.Close(ctx)

	var results []anchors.Anchor
	if err = mongoCursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// GetPreviewItemsForAnchors returns the first `limit` items (by position) of each anchor in a
// single aggregation, keyed by anchor ID. Requires MongoDB 5.2+ ($topN).
func (r *Repository) GetPreviewItemsForAnchors(
	ctx context.Context,
	anchorIDs []primitive.ObjectID,
	limit int,
) (map[primitive.ObjectID][]anchors.Item, error) {
	result := make(map[primitive.ObjectID][]anchors.Item, len(anchorIDs))
	if len(anchorIDs) == 0 {
		return result, nil
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"anchorId": bson.M{"$in": anchorIDs}, "deletedAt": nil}}},
		{{Key: "$group", Value: bson.M{
			"_id": "$anchorId",
			"items": bson.M{"$topN": bson.M{
				"n":      limit,
				"sortBy": bson.M{"position": 1},
				"output": "$$ROOT",
			}},
		}}},
	}
	cursor, err := r.itemsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var groups []struct {
		AnchorID primitive.ObjectID `bson:"_id"`
		Items    []anchors.Item     `bson:"items"`
	}
	if err := cursor.All(ctx, &groups); err != nil {
		return nil, err
	}
	for _, g := range groups {
		result[g.AnchorID] = g.Items
	}
	return result, nil
}

// AnyPublicAnchorWithTags reports whether at least one public, non-deleted anchor has any of the tags
func (r *Repository) AnyPublicAnchorWithTags(ctx context.Context, tagList []string) (bool, error) {
	n, err := r.anchorsCollection.CountDocuments(ctx, bson.M{
		"visibility": "public",
		"deletedAt":  nil,
		"tags":       bson.M{"$in": tagList},
	}, options.Count().SetLimit(1))
	return n > 0, err
}
