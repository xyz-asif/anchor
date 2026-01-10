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

type Repository struct {
	collection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	collection := db.Collection("notifications")

	// Create indexes
	collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
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
	})

	return &Repository{collection: collection}
}

// CreateNotification creates a single notification
func (r *Repository) CreateNotification(ctx context.Context, notification *Notification) error {
	notification.ID = primitive.NewObjectID()
	notification.CreatedAt = time.Now()
	notification.IsRead = false

	_, err := r.collection.InsertOne(ctx, notification)
	return err
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

	_, err := r.collection.InsertMany(ctx, docs)
	return err
}

// CreateNotifications creates multiple notifications at once
func (r *Repository) CreateNotifications(ctx context.Context, notifications []Notification) error {
	if len(notifications) == 0 {
		return nil
	}

	docs := make([]interface{}, len(notifications))
	for i, n := range notifications {
		if n.ID.IsZero() {
			notifications[i].ID = primitive.NewObjectID()
		}
		if n.CreatedAt.IsZero() {
			notifications[i].CreatedAt = time.Now()
		}
		docs[i] = notifications[i]
	}

	_, err := r.collection.InsertMany(ctx, docs)
	return err
}

// GetNotificationByID retrieves a notification by ID
func (r *Repository) GetNotificationByID(ctx context.Context, notificationID primitive.ObjectID) (*Notification, error) {
	var notification Notification
	err := r.collection.FindOne(ctx, bson.M{"_id": notificationID}).Decode(&notification)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, errors.New("notification not found")
		}
		return nil, err
	}
	return &notification, nil
}

// GetUserNotifications retrieves notifications for a user
func (r *Repository) GetUserNotifications(ctx context.Context, userID primitive.ObjectID, unreadOnly bool, page, limit int) ([]Notification, int64, error) {
	filter := bson.M{"recipientId": userID}
	if unreadOnly {
		filter["isRead"] = false
	}

	// Sort: unread first, then by date
	opts := options.Find().
		SetSort(bson.D{
			{Key: "isRead", Value: 1},
			{Key: "createdAt", Value: -1},
		}).
		SetSkip(int64((page - 1) * limit)).
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

// CountUnread counts unread notifications for a user
func (r *Repository) CountUnread(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{
		"recipientId": userID,
		"isRead":      false,
	})
}

// MarkAsRead marks a notification as read
func (r *Repository) MarkAsRead(ctx context.Context, notificationID primitive.ObjectID) error {
	result, err := r.collection.UpdateOne(
		ctx,
		bson.M{"_id": notificationID},
		bson.M{"$set": bson.M{"isRead": true}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errors.New("notification not found")
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
