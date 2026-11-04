package notifications

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotificationNotFound means no notification with that ID belongs to the user
var ErrNotificationNotFound = errors.New("notification not found")

type Repository struct {
	collection *mongo.Collection
	users      *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	collection := db.Collection("notifications")

	// Create indexes
	_, _ = collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "recipientId", Value: 1},
				{Key: "isRead", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "recipientId", Value: 1},
				{Key: "isRead", Value: 1},
			},
		},
		{
			Keys: bson.D{{Key: "createdAt", Value: 1}},
		},
		{
			// duplicate checks: same recipient, actor, type and resource
			Keys: bson.D{{Key: "recipientId", Value: 1}, {Key: "type", Value: 1}, {Key: "resourceId", Value: 1}},
		},
	})

	return &Repository{collection: collection, users: db.Collection("users")}
}

// CreateNotification creates a single notification
func (r *Repository) CreateNotification(ctx context.Context, notification *Notification) error {
	notification.ID = primitive.NewObjectID()
	notification.CreatedAt = time.Now()
	notification.IsRead = false

	if _, err := r.collection.InsertOne(ctx, notification); err != nil {
		return err
	}
	dispatchPush([]Notification{*notification})
	return nil
}

// CreateMany creates multiple notifications
func (r *Repository) CreateMany(ctx context.Context, notifications []Notification) error {
	if len(notifications) == 0 {
		return nil
	}

	docs := make([]interface{}, len(notifications))
	for i := range notifications {
		notifications[i].ID = primitive.NewObjectID()
		notifications[i].CreatedAt = time.Now()
		notifications[i].IsRead = false
		docs[i] = notifications[i]
	}

	if _, err := r.collection.InsertMany(ctx, docs); err != nil {
		return err
	}
	dispatchPush(notifications)
	return nil
}

// CreateNotifications creates multiple notifications at once
func (r *Repository) CreateNotifications(ctx context.Context, notifications []Notification) error {
	if len(notifications) == 0 {
		return nil
	}

	docs := make([]interface{}, len(notifications))
	for i, n := range notifications {
		docs[i] = n
	}

	if _, err := r.collection.InsertMany(ctx, docs); err != nil {
		return err
	}
	dispatchPush(notifications)
	return nil
}

// GetNotificationByID retrieves a notification by ID
func (r *Repository) GetNotificationByID(ctx context.Context, notificationID primitive.ObjectID) (*Notification, error) {
	var notification Notification
	err := r.collection.FindOne(ctx, bson.M{"_id": notificationID}).Decode(&notification)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotificationNotFound
		}
		return nil, err
	}
	return &notification, nil
}

// recipientFilter matches the user's notifications, leaving out those from people the user has
// blocked or muted since (they used to keep showing up, with previews, after a block)
func (r *Repository) recipientFilter(ctx context.Context, userID primitive.ObjectID) (bson.M, error) {
	filter := bson.M{"recipientId": userID}
	var u struct {
		BlockedUsers []primitive.ObjectID `bson:"blockedUsers"`
		MutedUsers   []primitive.ObjectID `bson:"mutedUsers"`
	}
	err := r.users.FindOne(ctx, bson.M{"_id": userID},
		options.FindOne().SetProjection(bson.M{"blockedUsers": 1, "mutedUsers": 1})).Decode(&u)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	if hidden := append(u.BlockedUsers, u.MutedUsers...); len(hidden) > 0 {
		filter["actorId"] = bson.M{"$nin": hidden}
	}
	return filter, nil
}

// GetUserNotifications retrieves notifications for a user
func (r *Repository) GetUserNotifications(ctx context.Context, userID primitive.ObjectID, unreadOnly bool, page, limit int) ([]Notification, int64, error) {
	filter, err := r.recipientFilter(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	if unreadOnly {
		filter["isRead"] = false
	}

	// Sort: unread first, then by date (_id keeps equal times in a stable order)
	opts := options.Find().
		SetSort(bson.D{
			{Key: "isRead", Value: 1},
			{Key: "createdAt", Value: -1},
			{Key: "_id", Value: -1},
		}).
		SetSkip(int64(page-1) * int64(limit)).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var notifications []Notification
	if err = cursor.All(ctx, &notifications); err != nil {
		return nil, 0, err
	}

	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return notifications, total, nil
}

// CountUnread counts unread notifications for a user (the same ones the list shows, so the
// badge and the list always agree)
func (r *Repository) CountUnread(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	filter, err := r.recipientFilter(ctx, userID)
	if err != nil {
		return 0, err
	}
	filter["isRead"] = false
	return r.collection.CountDocuments(ctx, filter)
}

// MarkAsRead marks the user's notification as read. Ownership is part of the query, so someone
// else's notification is simply "not found".
func (r *Repository) MarkAsRead(ctx context.Context, notificationID, userID primitive.ObjectID) error {
	result, err := r.collection.UpdateOne(
		ctx,
		bson.M{"_id": notificationID, "recipientId": userID},
		bson.M{"$set": bson.M{"isRead": true}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

// MarkAllAsRead marks all notifications as read for a user
func (r *Repository) MarkAllAsRead(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	result, err := r.collection.UpdateMany(
		ctx,
		bson.M{"recipientId": userID, "isRead": false},
		bson.M{"$set": bson.M{"isRead": true}},
	)
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

// DeleteForRecipient deletes one notification if it belongs to the user; returns false if not found
func (r *Repository) DeleteForRecipient(ctx context.Context, notificationID, userID primitive.ObjectID) (bool, error) {
	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": notificationID, "recipientId": userID})
	if err != nil {
		return false, err
	}
	return result.DeletedCount > 0, nil
}

// DeleteAllForRecipient deletes the user's notifications (only read ones if readOnly)
func (r *Repository) DeleteAllForRecipient(ctx context.Context, userID primitive.ObjectID, readOnly bool) (int64, error) {
	filter := bson.M{"recipientId": userID}
	if readOnly {
		filter["isRead"] = true
	}
	result, err := r.collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

// Exists reports whether a notification of this type from actor to recipient about resource exists
func (r *Repository) Exists(ctx context.Context, recipientID, actorID primitive.ObjectID, notifType string, resourceID primitive.ObjectID) (bool, error) {
	n, err := r.collection.CountDocuments(ctx, bson.M{
		"recipientId": recipientID,
		"actorId":     actorID,
		"type":        notifType,
		"resourceId":  resourceID,
	}, options.Count().SetLimit(1))
	return n > 0, err
}

// ExistsForAnchor reports whether a notification of this type from actor to recipient about the
// anchor exists (for clones, whose resource is a new anchor every time)
func (r *Repository) ExistsForAnchor(ctx context.Context, recipientID, actorID primitive.ObjectID, notifType string, anchorID primitive.ObjectID) (bool, error) {
	n, err := r.collection.CountDocuments(ctx, bson.M{
		"recipientId": recipientID,
		"actorId":     actorID,
		"type":        notifType,
		"anchorId":    anchorID,
	}, options.Count().SetLimit(1))
	return n > 0, err
}

// RefreshUnread moves the recipients' unread notifications of this type about this resource to
// the top again (new time and preview) and returns the recipients that had one. Used so that
// repeated anchor updates collapse into one unread notification instead of piling up.
func (r *Repository) RefreshUnread(ctx context.Context, recipientIDs []primitive.ObjectID, notifType string, resourceID primitive.ObjectID, preview string) (map[primitive.ObjectID]bool, error) {
	refreshed := map[primitive.ObjectID]bool{}
	if len(recipientIDs) == 0 {
		return refreshed, nil
	}
	filter := bson.M{
		"recipientId": bson.M{"$in": recipientIDs},
		"type":        notifType,
		"resourceId":  resourceID,
		"isRead":      false,
	}
	ids, err := r.collection.Distinct(ctx, "recipientId", filter)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return refreshed, nil
	}
	if _, err := r.collection.UpdateMany(ctx, filter,
		bson.M{"$set": bson.M{"createdAt": time.Now(), "preview": preview}}); err != nil {
		return nil, err
	}
	for _, v := range ids {
		if id, ok := v.(primitive.ObjectID); ok {
			refreshed[id] = true
		}
	}
	return refreshed, nil
}

// UsersBlockingOrMuting returns which of userIDs have blocked or muted actorID (one query,
// instead of one lookup per recipient)
func (r *Repository) UsersBlockingOrMuting(ctx context.Context, userIDs []primitive.ObjectID, actorID primitive.ObjectID) (map[primitive.ObjectID]bool, error) {
	out := map[primitive.ObjectID]bool{}
	if len(userIDs) == 0 {
		return out, nil
	}
	values, err := r.users.Distinct(ctx, "_id", bson.M{
		"_id": bson.M{"$in": userIDs},
		"$or": bson.A{bson.M{"blockedUsers": actorID}, bson.M{"mutedUsers": actorID}},
	})
	if err != nil {
		return nil, err
	}
	for _, v := range values {
		if id, ok := v.(primitive.ObjectID); ok {
			out[id] = true
		}
	}
	return out, nil
}
