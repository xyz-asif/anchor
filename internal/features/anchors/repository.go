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
	anchor.LastItemAddedAt = time.Now()

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

// GetUserAnchors retrieves all anchors for a specific user
func (r *Repository) GetUserAnchors(ctx context.Context, userID primitive.ObjectID) ([]Anchor, error) {
	filter := bson.M{
		"userId":    userID,
		"deletedAt": nil,
	}

	cursor, err := r.anchorsCollection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "lastItemAddedAt", Value: -1}}))
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

// GetPublicUserAnchors retrieves only public and unlisted anchors for a user
func (r *Repository) GetPublicUserAnchors(ctx context.Context, userID primitive.ObjectID) ([]Anchor, error) {
	filter := bson.M{
		"userId":     userID,
		"deletedAt":  nil,
		"visibility": bson.M{"$in": []string{VisibilityPublic, VisibilityUnlisted}},
	}

	cursor, err := r.anchorsCollection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "lastItemAddedAt", Value: -1}}))
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
