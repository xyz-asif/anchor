package anchors

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/xyz-asif/gotodo/internal/pkg/searchterms"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrAnchorNotFound / ErrItemNotFound mean the document doesn't exist; any other error means
// the lookup itself failed (and must not be reported to the client as "not found")
var (
	ErrAnchorNotFound = errors.New("anchor not found")
	ErrItemNotFound   = errors.New("item not found")
)

// Repository handles database interactions for the anchors feature
type Repository struct {
	anchorsCollection *mongo.Collection
	itemsCollection   *mongo.Collection
	db                *mongo.Database
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
			// Prefix-word index for search (see pkg/searchterms)
			Keys: bson.D{{Key: "search.terms", Value: 1}},
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
		db:                db,
	}
}

// CreateAnchor inserts a new anchor into the database
func (r *Repository) CreateAnchor(ctx context.Context, anchor *Anchor) error {
	anchor.CreatedAt = time.Now()
	anchor.UpdatedAt = time.Now()
	anchor.LastItemAddedAt = time.Now()
	anchor.Search = searchterms.ForAnchor(anchor.Title, anchor.Description, anchor.Tags)

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
			return nil, ErrAnchorNotFound
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
		SetSort(bson.D{{Key: "lastItemAddedAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64(page-1) * int64(limit)).
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

// GetPublicUserAnchors retrieves a user's public anchors for other viewers.
// Unlisted anchors are reachable by link only, so they are not listed here.
func (r *Repository) GetPublicUserAnchors(ctx context.Context, userID primitive.ObjectID, page int, limit int) ([]Anchor, int64, error) {
	filter := bson.M{
		"userId":     userID,
		"deletedAt":  nil,
		"visibility": VisibilityPublic,
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "lastItemAddedAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64(page-1) * int64(limit)).
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

	// If the caller already passed update operators ($set, $inc, ...) use them as-is,
	// otherwise treat the map as plain fields to $set.
	update := bson.M{"$set": updates}
	for key := range updates {
		if strings.HasPrefix(key, "$") {
			update = updates
			break
		}
	}

	result, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("anchor not found")
	}

	if touchesSearchFields(update) {
		if err := r.RefreshAnchorSearch(ctx, anchorID); err != nil {
			log.Printf("anchors: refreshing search terms of %s failed: %v", anchorID.Hex(), err)
		}
	}
	return nil
}

// touchesSearchFields reports whether an update changes text that search indexes
func touchesSearchFields(update bson.M) bool {
	for _, op := range update {
		if fields, ok := op.(bson.M); ok {
			for _, f := range []string{"title", "description", "tags"} {
				if _, ok := fields[f]; ok {
					return true
				}
			}
		}
	}
	return false
}

// RefreshAnchorSearch recomputes an anchor's search terms from its current text. The write only
// applies if the text is still what was read, so a slower refresh can never overwrite the
// terms of a newer edit with stale ones.
func (r *Repository) RefreshAnchorSearch(ctx context.Context, anchorID primitive.ObjectID) error {
	var doc bson.M
	err := r.anchorsCollection.FindOne(ctx, bson.M{"_id": anchorID},
		options.FindOne().SetProjection(bson.M{"title": 1, "description": 1, "tags": 1})).Decode(&doc)
	if err != nil {
		return err
	}
	var current struct {
		Title       string   `bson:"title"`
		Description string   `bson:"description"`
		Tags        []string `bson:"tags"`
	}
	raw, _ := bson.Marshal(doc)
	if err := bson.Unmarshal(raw, &current); err != nil {
		return err
	}
	_, err = r.anchorsCollection.UpdateOne(ctx,
		bson.M{"_id": anchorID, "title": doc["title"], "description": doc["description"], "tags": doc["tags"]},
		bson.M{"$set": bson.M{"search": searchterms.ForAnchor(current.Title, current.Description, current.Tags)}})
	return err
}

// SoftDeleteAnchor marks an anchor as deleted (and unpins it). Only one caller can delete an
// anchor: a second, concurrent delete gets ErrAnchorNotFound, so counters are adjusted once.
func (r *Repository) SoftDeleteAnchor(ctx context.Context, anchorID primitive.ObjectID) error {
	now := time.Now()
	result, err := r.anchorsCollection.UpdateOne(ctx,
		bson.M{"_id": anchorID, "deletedAt": nil},
		bson.M{"$set": bson.M{"deletedAt": now, "updatedAt": now, "isPinned": false}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrAnchorNotFound
	}
	return nil
}

// SetPinned changes isPinned from `from` to `to` atomically. changed is false when the anchor
// was not in the `from` state (e.g. a concurrent request already toggled it).
func (r *Repository) SetPinned(ctx context.Context, anchorID primitive.ObjectID, from, to bool) (changed bool, err error) {
	result, err := r.anchorsCollection.UpdateOne(ctx,
		bson.M{"_id": anchorID, "deletedAt": nil, "isPinned": from},
		bson.M{"$set": bson.M{"isPinned": to, "updatedAt": time.Now()}},
	)
	if err != nil {
		return false, err
	}
	return result.ModifiedCount > 0, nil
}

// ItemIDs returns the IDs of all items in an anchor
func (r *Repository) ItemIDs(ctx context.Context, anchorID primitive.ObjectID) (map[primitive.ObjectID]bool, error) {
	cursor, err := r.itemsCollection.Find(ctx, bson.M{"anchorId": anchorID}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	var docs []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make(map[primitive.ObjectID]bool, len(docs))
	for _, d := range docs {
		ids[d.ID] = true
	}
	return ids, nil
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
	cursor, err := r.itemsCollection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "position", Value: 1}, {Key: "_id", Value: 1}}))
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
		SetSort(bson.D{{Key: "position", Value: 1}, {Key: "_id", Value: 1}}).
		SetSkip(int64(page-1) * int64(limit)).
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
			return nil, ErrItemNotFound
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

	// All positions in one ordered bulk write (one round trip instead of one per item)
	now := time.Now()
	models := make([]mongo.WriteModel, 0, len(itemOIDs))
	for i, itemOID := range itemOIDs {
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": itemOID, "anchorId": anchorOID}).
			SetUpdate(bson.M{"$set": bson.M{"position": i, "updatedAt": now}}))
	}
	if len(models) == 0 {
		return nil
	}
	result, err := r.itemsCollection.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(true))
	if err != nil {
		return err
	}
	if result.MatchedCount != int64(len(models)) {
		return errors.New("item not found or does not belong to anchor")
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

// IncrementLikeCount atomically adjusts an anchor's like count. A decrement only applies while
// the count is at least that high, so it can never go negative (not even briefly).
func (r *Repository) IncrementLikeCount(ctx context.Context, anchorID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": anchorID}
	if delta < 0 {
		filter["likeCount"] = bson.M{"$gte": -delta}
	}
	_, err := r.anchorsCollection.UpdateOne(ctx, filter, bson.M{
		"$inc": bson.M{"likeCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	})
	return err
}

// GetPinnedAnchors retrieves pinned anchors for a specific user
func (r *Repository) GetPinnedAnchors(ctx context.Context, userID primitive.ObjectID, includePrivate bool) ([]Anchor, error) {
	filter := bson.M{
		"userId":    userID,
		"isPinned":  true,
		"deletedAt": nil,
	}

	if !includePrivate {
		filter["visibility"] = VisibilityPublic // unlisted anchors are link-only
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
	// Score = likes*2 + clones*3 + comments*1, computed inside MongoDB in one atomic update
	// (an aggregation-pipeline update), so concurrent likes/comments can't write a stale score.
	pipeline := mongo.Pipeline{
		{{Key: "$set", Value: bson.M{
			"engagementScore": bson.M{"$add": bson.A{
				bson.M{"$multiply": bson.A{bson.M{"$ifNull": bson.A{"$likeCount", 0}}, 2}},
				bson.M{"$multiply": bson.A{bson.M{"$ifNull": bson.A{"$cloneCount", 0}}, 3}},
				bson.M{"$ifNull": bson.A{"$commentCount", 0}},
			}},
		}}},
	}
	result, err := r.anchorsCollection.UpdateOne(ctx, bson.M{"_id": anchorID}, pipeline)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errors.New("anchor not found")
	}
	return nil
}

// IncrementCommentCount atomically adjusts an anchor's comment count; a decrement only applies
// while the count is at least that high, so it never goes negative (not even briefly).
func (r *Repository) IncrementCommentCount(ctx context.Context, anchorID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": anchorID}
	if delta < 0 {
		filter["commentCount"] = bson.M{"$gte": -delta}
	}
	_, err := r.anchorsCollection.UpdateOne(ctx, filter, bson.M{
		"$inc": bson.M{"commentCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	})
	return err
}

// GetAnchorTitles batch fetches titles for a list of anchor IDs
func (r *Repository) GetAnchorTitles(ctx context.Context, anchorIDs []primitive.ObjectID) (map[primitive.ObjectID]string, error) {
	if len(anchorIDs) == 0 {
		return make(map[primitive.ObjectID]string), nil
	}

	// Titles are shown in other users' notifications: skip deleted and private anchors
	filter := bson.M{
		"_id":        bson.M{"$in": anchorIDs},
		"deletedAt":  nil,
		"visibility": bson.M{"$ne": VisibilityPrivate},
	}
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

// GetAllUserAnchors retrieves ALL anchors for a specific user (for deletion)
func (r *Repository) GetAllUserAnchors(ctx context.Context, userID primitive.ObjectID) ([]Anchor, error) {
	filter := bson.M{"userId": userID} // Include deleted ones too to clean up? Yes.

	cursor, err := r.anchorsCollection.Find(ctx, filter)
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

// DeleteAnchor permanently deletes an anchor and its items
func (r *Repository) DeleteAnchor(ctx context.Context, anchorID primitive.ObjectID) error {
	// Delete items first
	_, err := r.itemsCollection.DeleteMany(ctx, bson.M{"anchorId": anchorID})
	if err != nil {
		return err
	}

	// Delete anchor
	_, err = r.anchorsCollection.DeleteOne(ctx, bson.M{"_id": anchorID})
	return err
}

// GetUserClonedAnchors returns anchors cloned by a user.
// When includePrivate is false, private clones are excluded.
func (r *Repository) GetUserClonedAnchors(ctx context.Context, userID primitive.ObjectID, page, limit int, includePrivate bool) ([]Anchor, int64, error) {
	filter := bson.M{
		"userId":             userID,
		"clonedFromAnchorId": bson.M{"$exists": true, "$ne": nil},
		"deletedAt":          nil,
	}
	if !includePrivate {
		filter["visibility"] = VisibilityPublic // others only see public clones
	}

	total, err := r.anchorsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64(page-1) * int64(limit)).
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

	return anchors, total, nil
}

// GetAnchorClones returns clones of an anchor
// Clones by users in hiddenCloners (block relationships with the viewer) are left out.
func (r *Repository) GetAnchorClones(ctx context.Context, originalAnchorID primitive.ObjectID, hiddenCloners []primitive.ObjectID, page, limit int) ([]Anchor, int64, error) {
	// Only public clones are listed (private and unlisted ones are not)
	filter := bson.M{
		"clonedFromAnchorId": originalAnchorID,
		"visibility":         VisibilityPublic,
		"deletedAt":          nil,
	}
	if len(hiddenCloners) > 0 {
		filter["userId"] = bson.M{"$nin": hiddenCloners}
	}

	total, err := r.anchorsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64(page-1) * int64(limit)).
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

	return anchors, total, nil
}

// GetAnchorsByIDs retrieves multiple anchors by their IDs
func (r *Repository) GetAnchorsByIDs(ctx context.Context, anchorIDs []primitive.ObjectID) ([]Anchor, error) {
	if len(anchorIDs) == 0 {
		return []Anchor{}, nil
	}

	filter := bson.M{
		"_id":       bson.M{"$in": anchorIDs},
		"deletedAt": nil,
	}

	cursor, err := r.anchorsCollection.Find(ctx, filter)
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

// IncrementFollowerCount increments or decrements the anchor's follower count
func (r *Repository) IncrementFollowerCount(ctx context.Context, anchorID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": anchorID}
	if delta < 0 {
		filter["followerCount"] = bson.M{"$gte": -delta} // never below zero
	}
	update := bson.M{
		"$inc": bson.M{"followerCount": delta},
		"$set": bson.M{"updatedAt": time.Now()},
	}

	_, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
	return err
}

// IncrementVersion increments the anchor version (called when items added)
func (r *Repository) IncrementVersion(ctx context.Context, anchorID primitive.ObjectID) error {
	now := time.Now()
	filter := bson.M{"_id": anchorID}
	update := bson.M{
		"$inc": bson.M{"version": 1},
		"$set": bson.M{
			"lastItemAddedAt": now,
			"updatedAt":       now,
		},
	}

	_, err := r.anchorsCollection.UpdateOne(ctx, filter, update)
	return err
}

type TagCount struct {
	Name  string `bson:"name"`
	Count int    `bson:"count"`
}

// GetPopularTags returns most used tags from public anchors
func (r *Repository) GetPopularTags(ctx context.Context, limit int) ([]TagCount, error) {
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

	var results []TagCount
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	if results == nil {
		results = []TagCount{}
	}

	return results, nil
}

// FindUserCloneOf returns the user's active clone of the given anchor, or nil if none exists
func (r *Repository) FindUserCloneOf(ctx context.Context, userID, originalAnchorID primitive.ObjectID) (*Anchor, error) {
	var anchor Anchor
	err := r.anchorsCollection.FindOne(ctx, bson.M{
		"userId":             userID,
		"clonedFromAnchorId": originalAnchorID,
		"deletedAt":          nil,
	}).Decode(&anchor)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &anchor, nil
}

// IncrementCloneCount atomically adjusts an anchor's clone count (never below zero)
func (r *Repository) IncrementCloneCount(ctx context.Context, anchorID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": anchorID}
	if delta < 0 {
		filter["cloneCount"] = bson.M{"$gte": -delta}
	}
	_, err := r.anchorsCollection.UpdateOne(ctx, filter, bson.M{"$inc": bson.M{"cloneCount": delta}})
	return err
}

// UpdateItem sets fields on an item and returns the updated item
func (r *Repository) UpdateItem(ctx context.Context, itemID primitive.ObjectID, set bson.M) (*Item, error) {
	set["updatedAt"] = time.Now()
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var item Item
	err := r.itemsCollection.FindOneAndUpdate(ctx, bson.M{"_id": itemID}, bson.M{"$set": set}, opts).Decode(&item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ActiveCloneIDs returns the IDs of the user's non-deleted clones of an anchor, oldest first
func (r *Repository) ActiveCloneIDs(ctx context.Context, userID, originalAnchorID primitive.ObjectID) ([]primitive.ObjectID, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetProjection(bson.M{"_id": 1})
	cursor, err := r.anchorsCollection.Find(ctx, bson.M{
		"userId":             userID,
		"clonedFromAnchorId": originalAnchorID,
		"deletedAt":          nil,
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make([]primitive.ObjectID, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	return ids, nil
}

// RenumberItems rewrites item positions as 0..n-1 (keeping their order) and sets the anchor's
// itemCount to the real number of items. Used after deletes so positions have no gaps.
func (r *Repository) RenumberItems(ctx context.Context, anchorID primitive.ObjectID) error {
	opts := options.Find().
		SetSort(bson.D{{Key: "position", Value: 1}, {Key: "createdAt", Value: 1}}).
		SetProjection(bson.M{"_id": 1, "position": 1})
	cursor, err := r.itemsCollection.Find(ctx, bson.M{"anchorId": anchorID}, opts)
	if err != nil {
		return err
	}
	var items []struct {
		ID       primitive.ObjectID `bson:"_id"`
		Position int                `bson:"position"`
	}
	if err := cursor.All(ctx, &items); err != nil {
		return err
	}

	var models []mongo.WriteModel
	for i, it := range items {
		if it.Position != i {
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": it.ID}).
				SetUpdate(bson.M{"$set": bson.M{"position": i}}))
		}
	}
	if len(models) > 0 {
		if _, err := r.itemsCollection.BulkWrite(ctx, models); err != nil {
			return err
		}
	}

	_, err = r.anchorsCollection.UpdateOne(ctx, bson.M{"_id": anchorID},
		bson.M{"$set": bson.M{"itemCount": len(items), "updatedAt": time.Now()}})
	return err
}

// IsAssetShared reports whether items of other anchors use the same media URL
// (clones point at the original's files), in which case the file must not be deleted.
func (r *Repository) IsAssetShared(ctx context.Context, anchorID primitive.ObjectID, url string) bool {
	if url == "" {
		return false
	}
	n, err := r.itemsCollection.CountDocuments(ctx, bson.M{
		"anchorId": bson.M{"$ne": anchorID},
		"$or": []bson.M{
			{"imageData.cloudinaryUrl": url},
			{"audioData.cloudinaryUrl": url},
			{"fileData.cloudinaryUrl": url},
		},
	}, options.Count().SetLimit(1))
	return err != nil || n > 0 // on error, keep the file
}
