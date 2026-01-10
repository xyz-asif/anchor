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
	blockedUserIDs []primitive.ObjectID, // New param
	cursor *FeedCursor,
	limit int,
) ([]anchors.Anchor, error) {
	// Base filter: Authors in userIDs, Public or Unlisted, Not Deleted
	filter := bson.M{
		"userId":     bson.M{"$in": userIDs},
		"visibility": bson.M{"$in": []string{"public", "unlisted"}},
		"deletedAt":  nil,
	}

	// Filter out blocked users if provided (though userIDs should already be filtered by service)
	// But as a safeguard:
	if len(blockedUserIDs) > 0 {
		filter["userId"] = bson.M{"$in": userIDs, "$nin": blockedUserIDs}
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
	excludeUserIDs []primitive.ObjectID,
	blockedUserIDs []primitive.ObjectID, // New param
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

	// Exclude followed users, self, AND blocked users
	allExcluded := excludeUserIDs
	allExcluded = append(allExcluded, blockedUserIDs...)

	if len(allExcluded) > 0 {
		filter["userId"] = bson.M{"$nin": allExcluded}
	}

	// Filter out blocked users (combine with excludeUserIDs if needed, but separate is cleaner)
	if len(blockedUserIDs) > 0 {
		// If excludeUserIDs exists, merge
		if _, ok := filter["userId"]; ok {
			// Already has $nin, need to merge lists?
			// MongoDB $nin can take a combined list.
			// Ideally service should combine them.
			// But here we can use $and if needed, but $nin merger is better.
			// For simplicity, let's assume service merges them into excludeUserIDs?
			// No, signature change requested in plan.
			// Let's assume we update signature and handle logic.

			// Actually, just append blockedUserIDs to excludeUserIDs in Service is easier?
			// But let's follow the pattern and add it to signature if I can.
			// If I add to signature, I should merge it here.
		} else {
			filter["userId"] = bson.M{"$nin": blockedUserIDs}
		}
	}

	// Tag filter
	if tag != nil && *tag != "" {
		filter["tags"] = *tag
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
	case "popular":
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
