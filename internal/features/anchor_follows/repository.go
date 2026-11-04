package anchor_follows

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Repository struct {
	collection *mongo.Collection
	db         *mongo.Database
}

func NewRepository(db *mongo.Database) *Repository {
	collection := db.Collection("anchor_follows")

	// Create indexes
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "anchorId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "anchorId", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "userId", Value: 1}, {Key: "createdAt", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "anchorId", Value: 1}, {Key: "notifyOnUpdate", Value: 1}},
		},
	}

	_, _ = collection.Indexes().CreateMany(context.Background(), indexes)

	return &Repository{collection: collection, db: db}
}

// CreateFollow creates a new anchor follow
// CreateFollow inserts a follow. created is false when the user already follows the anchor
// (unique index on userId+anchorId), so concurrent requests can't double-count.
func (r *Repository) CreateFollow(ctx context.Context, follow *AnchorFollow) (created bool, err error) {
	follow.ID = primitive.NewObjectID()
	follow.CreatedAt = time.Now()

	if _, err := r.collection.InsertOne(ctx, follow); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeleteFollow removes a follow relationship
// DeleteFollow removes a follow. deleted is false when there was nothing to remove.
func (r *Repository) DeleteFollow(ctx context.Context, userID, anchorID primitive.ObjectID) (deleted bool, err error) {
	filter := bson.M{
		"userId":   userID,
		"anchorId": anchorID,
	}
	result, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return false, err
	}
	return result.DeletedCount > 0, nil
}

// GetFollow gets a specific follow relationship
func (r *Repository) GetFollow(ctx context.Context, userID, anchorID primitive.ObjectID) (*AnchorFollow, error) {
	filter := bson.M{
		"userId":   userID,
		"anchorId": anchorID,
	}

	var follow AnchorFollow
	err := r.collection.FindOne(ctx, filter).Decode(&follow)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &follow, nil
}

// UpdateNotifyOnUpdate updates notification preference
func (r *Repository) UpdateNotifyOnUpdate(ctx context.Context, userID, anchorID primitive.ObjectID, notify bool) error {
	filter := bson.M{
		"userId":   userID,
		"anchorId": anchorID,
	}
	update := bson.M{
		"$set": bson.M{"notifyOnUpdate": notify},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFollowing
	}
	return nil
}

// ErrNotFollowing is returned when the user doesn't follow the anchor
var ErrNotFollowing = errors.New("not following this anchor")

// UpdateLastSeenVersion records that the user has seen the anchor up to version. It only ever
// moves forward ($max): a slow, older page view finishing late can't bring old updates back.
func (r *Repository) UpdateLastSeenVersion(ctx context.Context, userID, anchorID primitive.ObjectID, version int) error {
	filter := bson.M{
		"userId":   userID,
		"anchorId": anchorID,
	}
	update := bson.M{
		"$max": bson.M{"lastSeenVersion": version},
	}

	_, err := r.collection.UpdateOne(ctx, filter, update)
	return err
}

// GetUserFollowingAnchors gets all anchors a user is following
func (r *Repository) GetUserFollowingAnchors(ctx context.Context, userID primitive.ObjectID, page, limit int) ([]AnchorFollow, int64, error) {
	filter := bson.M{"userId": userID}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
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

	var follows []AnchorFollow
	if err = cursor.All(ctx, &follows); err != nil {
		return nil, 0, err
	}

	return follows, total, nil
}

// GetFollowersWithNotifications returns the followers who want update notifications. Followers
// the anchor's owner has blocked are left out: they can no longer open the anchor.
func (r *Repository) GetFollowersWithNotifications(ctx context.Context, anchorID primitive.ObjectID) ([]primitive.ObjectID, error) {
	filter := bson.M{
		"anchorId":       anchorID,
		"notifyOnUpdate": true,
	}

	var owner struct {
		BlockedUsers []primitive.ObjectID `bson:"blockedUsers"`
	}
	var anchor struct {
		UserID primitive.ObjectID `bson:"userId"`
	}
	if err := r.db.Collection("anchors").FindOne(ctx, bson.M{"_id": anchorID},
		options.FindOne().SetProjection(bson.M{"userId": 1})).Decode(&anchor); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	if err := r.db.Collection("users").FindOne(ctx, bson.M{"_id": anchor.UserID},
		options.FindOne().SetProjection(bson.M{"blockedUsers": 1})).Decode(&owner); err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if len(owner.BlockedUsers) > 0 {
		filter["userId"] = bson.M{"$nin": owner.BlockedUsers}
	}

	projection := bson.M{"userId": 1}
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var ids []primitive.ObjectID
	for cursor.Next(ctx) {
		var doc struct {
			UserID primitive.ObjectID `bson:"userId"`
		}
		if err := cursor.Decode(&doc); err == nil {
			ids = append(ids, doc.UserID)
		}
	}

	return ids, cursor.Err()
}

// CountFollowers counts followers for an anchor
func (r *Repository) CountFollowers(ctx context.Context, anchorID primitive.ObjectID) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"anchorId": anchorID})
}

// FollowingListOptions controls ListFollowing
type FollowingListOptions struct {
	Sort        string               // SortRecent, SortUpdated or SortAlphabetical
	OnlyUpdated bool                 // only anchors with versions the user hasn't seen
	HiddenUsers []primitive.ObjectID // owners in a block relationship with the user
	Page, Limit int
}

// FollowedAnchorRow is one followed anchor with the follow's own fields
type FollowedAnchorRow struct {
	Follow AnchorFollow `bson:"follow"`
	Anchor struct {
		ID              primitive.ObjectID `bson:"_id"`
		UserID          primitive.ObjectID `bson:"userId"`
		Title           string             `bson:"title"`
		Description     string             `bson:"description"`
		CoverMediaType  string             `bson:"coverMediaType"`
		CoverMediaValue string             `bson:"coverMediaValue"`
		Visibility      string             `bson:"visibility"`
		ItemCount       int                `bson:"itemCount"`
		LikeCount       int                `bson:"likeCount"`
		FollowerCount   int                `bson:"followerCount"`
		Tags            []string           `bson:"tags"`
		Version         int                `bson:"version"`
		LastItemAddedAt time.Time          `bson:"lastItemAddedAt"`
	} `bson:"anchor"`
}

// ListFollowing returns a page of the anchors userID follows. Deleted anchors, anchors that went
// private, and anchors by hidden owners are excluded in the query, so totals and pages are exact
// (no matter how many anchors the user follows).
func (r *Repository) ListFollowing(ctx context.Context, userID primitive.ObjectID, o FollowingListOptions) (rows []FollowedAnchorRow, total, totalWithUpdates int64, err error) {
	anchorMatch := bson.M{
		"deletedAt":  nil,
		"visibility": bson.M{"$ne": "private"},
	}
	if len(o.HiddenUsers) > 0 {
		anchorMatch["userId"] = bson.M{"$nin": o.HiddenUsers}
	}

	var sortStage bson.D
	switch o.Sort {
	case "updated":
		sortStage = bson.D{{Key: "anchor.lastItemAddedAt", Value: -1}, {Key: "follow._id", Value: -1}}
	case "alphabetical":
		sortStage = bson.D{{Key: "titleKey", Value: 1}, {Key: "follow._id", Value: 1}}
	default:
		sortStage = bson.D{{Key: "follow.createdAt", Value: -1}, {Key: "follow._id", Value: -1}}
	}

	page := bson.A{}
	if o.OnlyUpdated {
		page = append(page, bson.M{"$match": bson.M{"hasUpdates": true}})
	}
	page = append(page,
		bson.M{"$sort": sortStage},
		bson.M{"$skip": int64(o.Page-1) * int64(o.Limit)},
		bson.M{"$limit": int64(o.Limit)},
	)
	totalStage := bson.A{}
	if o.OnlyUpdated {
		totalStage = append(totalStage, bson.M{"$match": bson.M{"hasUpdates": true}})
	}
	totalStage = append(totalStage, bson.M{"$count": "n"})

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": userID}}},
		{{Key: "$lookup", Value: bson.M{
			"from": "anchors",
			"let":  bson.M{"aid": "$anchorId"},
			"pipeline": bson.A{
				bson.M{"$match": bson.M{"$expr": bson.M{"$eq": bson.A{"$_id", "$$aid"}}}},
				bson.M{"$match": anchorMatch},
			},
			"as": "anchor",
		}}},
		{{Key: "$unwind", Value: "$anchor"}},
		{{Key: "$project", Value: bson.M{
			"anchor": 1,
			"follow": bson.M{
				"_id": "$_id", "userId": "$userId", "anchorId": "$anchorId", "notifyOnUpdate": "$notifyOnUpdate",
				"lastSeenVersion": "$lastSeenVersion", "createdAt": "$createdAt",
			},
			"hasUpdates": bson.M{"$gt": bson.A{"$anchor.version", bson.M{"$ifNull": bson.A{"$lastSeenVersion", 0}}}},
			"titleKey":   bson.M{"$toLower": "$anchor.title"},
		}}},
		{{Key: "$facet", Value: bson.M{
			"rows":        page,
			"total":       totalStage,
			"withUpdates": bson.A{bson.M{"$match": bson.M{"hasUpdates": true}}, bson.M{"$count": "n"}},
		}}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, 0, err
	}
	defer cursor.Close(ctx)

	type count struct {
		N int64 `bson:"n"`
	}
	var result []struct {
		Rows        []FollowedAnchorRow `bson:"rows"`
		Total       []count             `bson:"total"`
		WithUpdates []count             `bson:"withUpdates"`
	}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, 0, 0, err
	}
	if len(result) == 0 {
		return []FollowedAnchorRow{}, 0, 0, nil
	}
	if len(result[0].Total) > 0 {
		total = result[0].Total[0].N
	}
	if len(result[0].WithUpdates) > 0 {
		totalWithUpdates = result[0].WithUpdates[0].N
	}
	return result[0].Rows, total, totalWithUpdates, nil
}
