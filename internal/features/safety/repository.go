package safety

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
	reportsCollection *mongo.Collection
	usersCollection   *mongo.Collection
	anchorsCollection *mongo.Collection
	itemsCollection   *mongo.Collection
	followsCollection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	reports := db.Collection("reports")
	_, _ = reports.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}},
		{Keys: bson.D{{Key: "reporterId", Value: 1}, {Key: "targetId", Value: 1}, {Key: "status", Value: 1}}},
	})

	return &Repository{
		reportsCollection: reports,
		usersCollection:   db.Collection("users"),
		anchorsCollection: db.Collection("anchors"),
		itemsCollection:   db.Collection("items"),
		followsCollection: db.Collection("follows"),
	}
}

func (r *Repository) CreateReport(ctx context.Context, report *Report) error {
	report.CreatedAt = time.Now()
	report.UpdatedAt = time.Now()
	report.Status = ReportStatusPending

	result, err := r.reportsCollection.InsertOne(ctx, report)
	if err != nil {
		return err
	}
	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		report.ID = oid
	}
	return nil
}

func (r *Repository) BlockUser(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
	if blockerID == blockedID {
		return errors.New("cannot block self")
	}

	// Older user documents may hold blockedUsers: null, which $addToSet rejects
	_, _ = r.usersCollection.UpdateOne(ctx, bson.M{"_id": blockerID, "blockedUsers": nil},
		bson.M{"$set": bson.M{"blockedUsers": bson.A{}}})

	filter := bson.M{"_id": blockerID}
	update := bson.M{
		"$addToSet": bson.M{"blockedUsers": blockedID},
		"$set":      bson.M{"updatedAt": time.Now()},
	}

	_, err := r.usersCollection.UpdateOne(ctx, filter, update)
	return err
}

func (r *Repository) RemoveBlock(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
	// Only match when the id is in the list ($pull fails on a null field)
	filter := bson.M{"_id": blockerID, "blockedUsers": blockedID}
	update := bson.M{
		"$pull": bson.M{"blockedUsers": blockedID},
		"$set":  bson.M{"updatedAt": time.Now()},
	}

	_, err := r.usersCollection.UpdateOne(ctx, filter, update)
	return err
}

// TargetExists reports whether the reported anchor, item or user exists
// (soft-deleted anchors count as missing).
func (r *Repository) TargetExists(ctx context.Context, targetType string, targetID primitive.ObjectID) (bool, error) {
	var coll *mongo.Collection
	filter := bson.M{"_id": targetID}
	switch targetType {
	case "user":
		coll = r.usersCollection
	case "anchor":
		coll = r.anchorsCollection
		filter["deletedAt"] = nil
	case "item":
		coll = r.itemsCollection
	default:
		return false, errors.New("unknown target type")
	}
	n, err := coll.CountDocuments(ctx, filter, options.Count().SetLimit(1))
	return n > 0, err
}

// HasOpenReport reports whether the reporter already has an unresolved report on the target
func (r *Repository) HasOpenReport(ctx context.Context, reporterID, targetID primitive.ObjectID) (bool, error) {
	n, err := r.reportsCollection.CountDocuments(ctx, bson.M{
		"reporterId": reporterID,
		"targetId":   targetID,
		"status":     bson.M{"$in": []string{ReportStatusPending, ReportStatusReviewed}},
	}, options.Count().SetLimit(1))
	return n > 0, err
}

// ListReports returns reports, newest first, optionally filtered by status and target type
func (r *Repository) ListReports(ctx context.Context, status, targetType string, page, limit int) ([]Report, int64, error) {
	filter := bson.M{}
	if status != "" {
		filter["status"] = status
	}
	if targetType != "" {
		filter["targetType"] = targetType
	}

	total, err := r.reportsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))
	cursor, err := r.reportsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	reports := []Report{}
	if err := cursor.All(ctx, &reports); err != nil {
		return nil, 0, err
	}
	return reports, total, nil
}

// GetReportByID returns a report, or nil if it doesn't exist
func (r *Repository) GetReportByID(ctx context.Context, id primitive.ObjectID) (*Report, error) {
	var report Report
	err := r.reportsCollection.FindOne(ctx, bson.M{"_id": id}).Decode(&report)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// UpdateReportStatus records an admin's moderation decision and returns the updated report
func (r *Repository) UpdateReportStatus(ctx context.Context, id, adminID primitive.ObjectID, status, note string) (*Report, error) {
	now := time.Now()
	update := bson.M{"$set": bson.M{
		"status":         status,
		"resolutionNote": note,
		"reviewedBy":     adminID,
		"reviewedAt":     now,
		"updatedAt":      now,
	}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var report Report
	err := r.reportsCollection.FindOneAndUpdate(ctx, bson.M{"_id": id}, update, opts).Decode(&report)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// TargetSummary returns a few identifying fields of the reported target for moderators
func (r *Repository) TargetSummary(ctx context.Context, targetType string, targetID primitive.ObjectID) (bson.M, error) {
	var coll *mongo.Collection
	var projection bson.M
	switch targetType {
	case "user":
		coll, projection = r.usersCollection, bson.M{"username": 1, "displayName": 1, "profilePictureUrl": 1, "createdAt": 1}
	case "anchor":
		coll, projection = r.anchorsCollection, bson.M{"title": 1, "description": 1, "userId": 1, "visibility": 1, "deletedAt": 1}
	case "item":
		coll, projection = r.itemsCollection, bson.M{"anchorId": 1, "type": 1, "textData": 1, "urlData": 1, "imageData.cloudinaryUrl": 1}
	default:
		return nil, nil
	}
	var doc bson.M
	err := coll.FindOne(ctx, bson.M{"_id": targetID}, options.FindOne().SetProjection(projection)).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return doc, err
}

// MuteUser adds mutedID to the muter's mutedUsers list (idempotent)
func (r *Repository) MuteUser(ctx context.Context, muterID, mutedID primitive.ObjectID) error {
	_, err := r.usersCollection.UpdateOne(ctx, bson.M{"_id": muterID}, bson.M{
		"$addToSet": bson.M{"mutedUsers": mutedID},
		"$set":      bson.M{"updatedAt": time.Now()},
	})
	return err
}

// UnmuteUser removes mutedID from the muter's mutedUsers list (idempotent)
func (r *Repository) UnmuteUser(ctx context.Context, muterID, mutedID primitive.ObjectID) error {
	_, err := r.usersCollection.UpdateOne(ctx, bson.M{"_id": muterID, "mutedUsers": mutedID}, bson.M{
		"$pull": bson.M{"mutedUsers": mutedID},
		"$set":  bson.M{"updatedAt": time.Now()},
	})
	return err
}

// HideAnchor marks an anchor as "not interested" for the user (idempotent)
func (r *Repository) HideAnchor(ctx context.Context, userID, anchorID primitive.ObjectID) error {
	_, err := r.usersCollection.UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$addToSet": bson.M{"hiddenAnchors": anchorID},
		"$set":      bson.M{"updatedAt": time.Now()},
	})
	return err
}

// UnhideAnchor removes an anchor from the user's "not interested" list (idempotent)
func (r *Repository) UnhideAnchor(ctx context.Context, userID, anchorID primitive.ObjectID) error {
	_, err := r.usersCollection.UpdateOne(ctx, bson.M{"_id": userID, "hiddenAnchors": anchorID}, bson.M{
		"$pull": bson.M{"hiddenAnchors": anchorID},
		"$set":  bson.M{"updatedAt": time.Now()},
	})
	return err
}

// RemoveFollowsBetween deletes follows in both directions between two users and adjusts
// their follower/following counts (only for follows that actually existed, never below zero).
func (r *Repository) RemoveFollowsBetween(ctx context.Context, a, b primitive.ObjectID) error {
	for _, pair := range [][2]primitive.ObjectID{{a, b}, {b, a}} {
		follower, following := pair[0], pair[1]
		res, err := r.followsCollection.DeleteOne(ctx, bson.M{"followerId": follower, "followingId": following})
		if err != nil {
			return err
		}
		if res.DeletedCount == 0 {
			continue
		}
		_, _ = r.usersCollection.UpdateOne(ctx,
			bson.M{"_id": follower, "followingCount": bson.M{"$gte": 1}},
			bson.M{"$inc": bson.M{"followingCount": -1}})
		_, _ = r.usersCollection.UpdateOne(ctx,
			bson.M{"_id": following, "followerCount": bson.M{"$gte": 1}},
			bson.M{"$inc": bson.M{"followerCount": -1}})
	}
	return nil
}
