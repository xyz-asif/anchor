package anchors

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repository handles database interactions for the anchors feature
type Repository struct {
	anchorsCollection *mongo.Collection
	itemsCollection   *mongo.Collection
}

// NewRepository initializes the repository and creates necessary indexes
func NewRepository(db *mongo.Database) *Repository {
	anchorsCollection := db.Collection("anchors")
	itemsCollection := db.Collection("items")

	// Create indexes for anchors collection
	_, _ = anchorsCollection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "userId", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "visibility", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "tags", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "lastItemAddedAt", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "deletedAt", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
		{
			// Discovery feed index
			Keys: bson.D{
				{Key: "visibility", Value: 1},
				{Key: "deletedAt", Value: 1},
				{Key: "engagementScore", Value: -1},
				{Key: "createdAt", Value: -1},
				{Key: "_id", Value: -1},
			},
		},
		{
			// Discovery with tag filter
			Keys: bson.D{
				{Key: "visibility", Value: 1},
				{Key: "deletedAt", Value: 1},
				{Key: "tags", Value: 1},
				{Key: "engagementScore", Value: -1},
			},
		},
		{
			// Text index for search
			Keys: bson.D{
				{Key: "title", Value: "text"},
				{Key: "description", Value: "text"},
				{Key: "tags", Value: "text"},
			},
			Options: options.Index().
				SetWeights(bson.D{
					{Key: "title", Value: 10},
					{Key: "tags", Value: 5},
					{Key: "description", Value: 1},
				}).
				SetName("anchor_text_search"),
		},
	})

	// Create indexes for items collection
	_, _ = itemsCollection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "anchorId", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "position", Value: 1}},
		},
	})

	return &Repository{
		anchorsCollection: anchorsCollection,
		itemsCollection:   itemsCollection,
	}
}

// CreateAnchor inserts a new anchor into the database
func (r *Repository) CreateAnchor(ctx context.Context, anchor *Anchor) error {
	anchor.CreatedAt = time.Now()
	anchor.UpdatedAt = time.Now()
	now := time.Now()
	anchor.LastItemAddedAt = &now

	result, err := r.anchorsCollection.InsertOne(ctx, anchor)
	if err != nil {
		return err
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		anchor.ID = oid
	}

	return nil
}

// GetAnchorByID finds an anchor by its ID
func (r *Repository) GetAnchorByID(ctx context.Context, anchorID primitive.ObjectID) (*Anchor, error) {
	var anchor Anchor
	err := r.anchorsCollection.FindOne(ctx, bson.M{"_id": anchorID}).Decode(&anchor)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errors.New("anchor not found")
		}
		return nil, err
	}

	return &anchor, nil
}

// GetUserAnchors retrieves all anchors for a specific user with pagination
func (r *Repository) GetUserAnchors(ctx context.Context, userID primitive.ObjectID, page int, limit int) ([]Anchor, int64, error) {
	filter := bson.M{
		"userId":    userID,
		"deletedAt": nil,
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "lastItemAddedAt", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.anchorsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var anchors []Anchor
	if err = cursor.All(ctx, &anchors); err != nil {
		return nil, 0, err
	}

	total, err := r.anchorsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return anchors, total, nil
}

// GetPublicUserAnchors retrieves only public and unlisted anchors for a user with pagination
func (r *Repository) GetPublicUserAnchors(ctx context.Context, userID primitive.ObjectID, page int, limit int) ([]Anchor, int64, error) {
	filter := bson.M{
		"userId":     userID,
		"deletedAt":  nil,
		"visibility": bson.M{"$in": []string{VisibilityPublic, VisibilityUnlisted}},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "lastItemAddedAt", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.anchorsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var anchors []Anchor
	if err = cursor.All(ctx, &anchors); err != nil {
		return nil, 0, err
	}

	total, err := r.anchorsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return anchors, total, nil
}

// UpdateAnchor updates specific fields of an anchor
func (r *Repository) UpdateAnchor(ctx context.Context, anchorID primitive.ObjectID, updates bson.M) error {
	filter := bson.M{"_id": anchorID}

	// Handle $inc and $set operations
	var update bson.M
	if _, hasInc := updates["$inc"]; hasInc {
		update = updates
	} else {
		update = bson.M{"$set": updates}
	}

	result, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("anchor not found")
	}

	return nil
}

// SoftDeleteAnchor marks an anchor as deleted
func (r *Repository) SoftDeleteAnchor(ctx context.Context, anchorID primitive.ObjectID) error {
	now := time.Now()
	filter := bson.M{"_id": anchorID}
	update := bson.M{
		"$set": bson.M{
			"deletedAt": now,
			"updatedAt": now,
		},
	}

	result, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("anchor not found")
	}

	return nil
}

// CountUserAnchors counts the total number of non-deleted anchors for a user
func (r *Repository) CountUserAnchors(ctx context.Context, userID string) (int64, error) {
	filter := bson.M{
		"userId":    userID,
		"deletedAt": nil,
	}

	count, err := r.anchorsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// CountPinnedAnchors counts the number of pinned anchors for a user
func (r *Repository) CountPinnedAnchors(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	filter := bson.M{
		"userId":    userID,
		"isPinned":  true,
		"deletedAt": nil,
	}

	count, err := r.anchorsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// CreateItem inserts a new item into an anchor
func (r *Repository) CreateItem(ctx context.Context, item *Item) error {
	item.CreatedAt = time.Now()
	item.UpdatedAt = time.Now()

	result, err := r.itemsCollection.InsertOne(ctx, item)
	if err != nil {
		return err
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		item.ID = oid
	}

	return nil
}

// CreateItems inserts multiple items into the database
func (r *Repository) CreateItems(ctx context.Context, items []interface{}) error {
	if len(items) == 0 {
		return nil
	}
	_, err := r.itemsCollection.InsertMany(ctx, items)
	return err
}

// GetAnchorItems retrieves all items for a specific anchor, ordered by position
func (r *Repository) GetAnchorItems(ctx context.Context, anchorID primitive.ObjectID) ([]Item, error) {
	filter := bson.M{"anchorId": anchorID}
	cursor, err := r.itemsCollection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "position", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var items []Item
	if err = cursor.All(ctx, &items); err != nil {
		return nil, err
	}

	return items, nil
}

// GetAnchorItemsPaginated retrieves items for an anchor with pagination
func (r *Repository) GetAnchorItemsPaginated(ctx context.Context, anchorID primitive.ObjectID, page int, limit int) ([]Item, int64, error) {
	filter := bson.M{"anchorId": anchorID}

	opts := options.Find().
		SetSort(bson.D{{Key: "position", Value: 1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.itemsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var items []Item
	if err = cursor.All(ctx, &items); err != nil {
		return nil, 0, err
	}

	total, err := r.itemsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// GetItemByID finds an item by its ID
func (r *Repository) GetItemByID(ctx context.Context, itemID primitive.ObjectID) (*Item, error) {
	var item Item
	err := r.itemsCollection.FindOne(ctx, bson.M{"_id": itemID}).Decode(&item)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errors.New("item not found")
		}
		return nil, err
	}

	return &item, nil
}

// DeleteItem removes an item from the database
func (r *Repository) DeleteItem(ctx context.Context, itemID primitive.ObjectID) error {
	result, err := r.itemsCollection.DeleteOne(ctx, bson.M{"_id": itemID})
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return errors.New("item not found")
	}

	return nil
}

// ReorderItems updates the position field for multiple items
func (r *Repository) ReorderItems(ctx context.Context, anchorID string, itemIDs []string) error {
	anchorOID, err := primitive.ObjectIDFromHex(anchorID)
	if err != nil {
		return errors.New("invalid anchor id format")
	}

	// Convert all item IDs to ObjectIDs
	var itemOIDs []primitive.ObjectID
	for _, itemID := range itemIDs {
		oid, err := primitive.ObjectIDFromHex(itemID)
		if err != nil {
			return errors.New("invalid item id format")
		}
		itemOIDs = append(itemOIDs, oid)
	}

	// Update each item's position
	for i, itemOID := range itemOIDs {
		filter := bson.M{
			"_id":      itemOID,
			"anchorId": anchorOID,
		}
		update := bson.M{
			"$set": bson.M{
				"position":  i,
				"updatedAt": time.Now(),
			},
		}

		result, err := r.itemsCollection.UpdateOne(ctx, filter, update)
		if err != nil {
			return err
		}

		if result.MatchedCount == 0 {
			return errors.New("item not found or does not belong to anchor")
		}
	}

	return nil
}

// CountAnchorItems counts the total number of items in an anchor
func (r *Repository) CountAnchorItems(ctx context.Context, anchorID primitive.ObjectID) (int64, error) {
	filter := bson.M{"anchorId": anchorID}
	count, err := r.itemsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// IncrementLikeCount increments or decrements an anchor's like count
func (r *Repository) IncrementLikeCount(ctx context.Context, anchorID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": anchorID}
	update := bson.M{
		"$inc": bson.M{"likeCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	_, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	// If decrementing, ensure count doesn't go negative
	if delta < 0 {
		// Fix negative count if it occurred
		_, _ = r.anchorsCollection.UpdateOne(ctx,
			bson.M{"_id": anchorID, "likeCount": bson.M{"$lt": 0}},
			bson.M{"$set": bson.M{"likeCount": 0}},
		)
	}

	return nil
}

// GetPinnedAnchors retrieves pinned anchors for a specific user
func (r *Repository) GetPinnedAnchors(ctx context.Context, userID primitive.ObjectID, includePrivate bool) ([]Anchor, error) {
	filter := bson.M{
		"userId":    userID,
		"isPinned":  true,
		"deletedAt": nil,
	}

	if !includePrivate {
		filter["visibility"] = bson.M{"$in": []string{VisibilityPublic, VisibilityUnlisted}}
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetLimit(3)

	cursor, err := r.anchorsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var anchors []Anchor
	if err = cursor.All(ctx, &anchors); err != nil {
		return nil, err
	}

	return anchors, nil
}

// UpdateEngagementScore recalculates and updates the engagement score of an anchor
func (r *Repository) UpdateEngagementScore(ctx context.Context, anchorID primitive.ObjectID) error {
	anchor, err := r.GetAnchorByID(ctx, anchorID)
	if err != nil {
		return err
	}

	// Calculate: (likes * 2) + (clones * 3) + (comments * 1)
	score := (anchor.LikeCount * 2) + (anchor.CloneCount * 3) + (anchor.CommentCount * 1)

	return r.UpdateAnchor(ctx, anchorID, bson.M{
		"engagementScore": score,
		"updatedAt":       time.Now(),
	})
}

// IncrementCommentCount increments/decrements anchor's comment count
func (r *Repository) IncrementCommentCount(ctx context.Context, anchorID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": anchorID}
	update := bson.M{
		"$inc": bson.M{"commentCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	_, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	// Ensure count doesn't go negative
	if delta < 0 {
		_, _ = r.anchorsCollection.UpdateOne(ctx,
			bson.M{"_id": anchorID, "commentCount": bson.M{"$lt": 0}},
			bson.M{"$set": bson.M{"commentCount": 0}},
		)
	}

	return nil
}

// GetAnchorTitles batch fetches titles for a list of anchor IDs
func (r *Repository) GetAnchorTitles(ctx context.Context, anchorIDs []primitive.ObjectID) (map[primitive.ObjectID]string, error) {
	if len(anchorIDs) == 0 {
		return make(map[primitive.ObjectID]string), nil
	}

	filter := bson.M{"_id": bson.M{"$in": anchorIDs}}
	projection := bson.M{"title": 1}

	cursor, err := r.anchorsCollection.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	result := make(map[primitive.ObjectID]string)
	for cursor.Next(ctx) {
		var doc struct {
			ID    primitive.ObjectID `bson:"_id"`
			Title string             `bson:"title"`
		}
		if err := cursor.Decode(&doc); err == nil {
			result[doc.ID] = doc.Title
		}
	}

	return result, nil
}

// IncrementVersion increments the version of an anchor
func (r *Repository) IncrementVersion(ctx context.Context, anchorID primitive.ObjectID) error {
	filter := bson.M{"_id": anchorID}
	update := bson.M{
		"$inc": bson.M{"version": 1},
	}
	_, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
	return err
}

// TagCount represents a tag with its usage count
type TagCount struct {
	Name  string `bson:"name"`
	Count int    `bson:"count"`
}

// GetPopularTags returns most used tags from public anchors
func (r *Repository) GetPopularTags(ctx context.Context, limit int) ([]TagCount, error) {
	pipeline := mongo.Pipeline{
		// Match public, non-deleted anchors
		{{Key: "$match", Value: bson.M{
			"visibility": VisibilityPublic,
			"deletedAt":  nil,
		}}},
		// Unwind tags array
		{{Key: "$unwind", Value: "$tags"}},
		// Group by tag and count
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$toLower": "$tags"},
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

	var results []TagCount
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	if results == nil {
		results = []TagCount{}
	}

	return results, nil
}
