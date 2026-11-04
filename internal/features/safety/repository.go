package safety

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Repository struct {
	reportsCollection       *mongo.Collection
	usersCollection         *mongo.Collection
	anchorsCollection       *mongo.Collection
	itemsCollection         *mongo.Collection
	commentsCollection      *mongo.Collection
	followsCollection       *mongo.Collection
	anchorFollowsCollection *mongo.Collection
}

var (
	// ErrAlreadyReported: the reporter already has an open report on this target
	ErrAlreadyReported = errors.New("already reported")
	// ErrListFull: the block or mute list reached its maximum size
	ErrListFull = errors.New("list full")
)

const (
	maxBlockedUsers = 10000
	maxMutedUsers   = 10000
	maxHiddenAnchor = 2000 // oldest "not interested" entries are dropped beyond this
)

func NewRepository(db *mongo.Database) *Repository {
	reports := db.Collection("reports")
	ctx := context.Background()

	// "open" marks pending/reviewed reports; set it on reports saved before it existed, so the
	// unique index below covers them too
	_, _ = reports.UpdateMany(ctx, bson.M{"open": bson.M{"$exists": false}, "status": bson.M{"$in": openStatuses}},
		bson.M{"$set": bson.M{"open": true}})
	_, _ = reports.UpdateMany(ctx, bson.M{"open": bson.M{"$exists": false}}, bson.M{"$set": bson.M{"open": false}})

	if _, err := reports.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}},
		{Keys: bson.D{{Key: "reporterId", Value: 1}, {Key: "createdAt", Value: -1}}},
		{
			// One open report per person per target, even when two requests race
			Keys:    bson.D{{Key: "reporterId", Value: 1}, {Key: "targetId", Value: 1}},
			Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"open": true}).SetName("one_open_report_per_target"),
		},
	}); err != nil {
		log.Printf("safety: creating report indexes failed (duplicate open reports?): %v", err)
	}

	return &Repository{
		reportsCollection:       reports,
		usersCollection:         db.Collection("users"),
		anchorsCollection:       db.Collection("anchors"),
		itemsCollection:         db.Collection("items"),
		commentsCollection:      db.Collection("comments"),
		followsCollection:       db.Collection("follows"),
		anchorFollowsCollection: db.Collection("anchor_follows"),
	}
}

var openStatuses = []string{ReportStatusPending, ReportStatusReviewed}

func isOpenStatus(status string) bool {
	return status == ReportStatusPending || status == ReportStatusReviewed
}

// CreateReport saves a new pending report; ErrAlreadyReported if the reporter already has an
// open report on the same target
func (r *Repository) CreateReport(ctx context.Context, report *Report) error {
	report.CreatedAt = time.Now()
	report.UpdatedAt = time.Now()
	report.Status = ReportStatusPending
	report.Open = true

	result, err := r.reportsCollection.InsertOne(ctx, report)
	if mongo.IsDuplicateKeyError(err) {
		return ErrAlreadyReported
	}
	if err != nil {
		return err
	}
	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		report.ID = oid
	}
	return nil
}

// CountRecentReports counts the reports a user filed since the given time
func (r *Repository) CountRecentReports(ctx context.Context, reporterID primitive.ObjectID, since time.Time) (int64, error) {
	return r.reportsCollection.CountDocuments(ctx, bson.M{"reporterId": reporterID, "createdAt": bson.M{"$gte": since}})
}

func (r *Repository) BlockUser(ctx context.Context, blockerID, blockedID primitive.ObjectID) error {
	if blockerID == blockedID {
		return errors.New("cannot block self")
	}

	// Older user documents may hold blockedUsers: null, which $addToSet rejects
	_, _ = r.usersCollection.UpdateOne(ctx, bson.M{"_id": blockerID, "blockedUsers": nil},
		bson.M{"$set": bson.M{"blockedUsers": bson.A{}}})

	return r.addToCappedList(ctx, blockerID, "blockedUsers", blockedID, maxBlockedUsers)
}

// addToCappedList adds id to a user's list (idempotent), refusing with ErrListFull once the
// list has max entries, so a user document can't grow without bound
func (r *Repository) addToCappedList(ctx context.Context, userID primitive.ObjectID, field string, id primitive.ObjectID, max int) error {
	result, err := r.usersCollection.UpdateOne(ctx,
		bson.M{"_id": userID, field + "." + strconv.Itoa(max-1): bson.M{"$exists": false}},
		bson.M{"$addToSet": bson.M{field: id}, "$set": bson.M{"updatedAt": time.Now()}})
	if err != nil {
		return err
	}
	if result.MatchedCount > 0 {
		return nil
	}
	// Full, unless the id is already in it (then there's nothing to do)
	n, err := r.usersCollection.CountDocuments(ctx, bson.M{"_id": userID, field: id}, options.Count().SetLimit(1))
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return ErrListFull
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

// VisibleTarget looks up a report target as the viewer would see it and returns the ID of the
// user it belongs to. found is false when it doesn't exist, was deleted, or the viewer can't
// see it: someone else's private anchor (or an item/comment in one), or content whose anchor
// owner has blocked the viewer. Those all look the same, so reporting can't be used to probe
// for private content.
func (r *Repository) VisibleTarget(ctx context.Context, targetType string, targetID, viewerID primitive.ObjectID) (ownerID primitive.ObjectID, found bool, err error) {
	switch targetType {
	case TargetUser:
		n, err := r.usersCollection.CountDocuments(ctx, bson.M{"_id": targetID}, options.Count().SetLimit(1))
		return targetID, n > 0, err
	case TargetAnchor:
		return r.visibleAnchor(ctx, targetID, viewerID)
	case TargetItem:
		var item struct {
			AnchorID primitive.ObjectID `bson:"anchorId"`
		}
		if err := r.itemsCollection.FindOne(ctx, bson.M{"_id": targetID}).Decode(&item); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return primitive.NilObjectID, false, nil
			}
			return primitive.NilObjectID, false, err
		}
		return r.visibleAnchor(ctx, item.AnchorID, viewerID)
	case TargetComment:
		var comment struct {
			AnchorID primitive.ObjectID `bson:"anchorId"`
			UserID   primitive.ObjectID `bson:"userId"`
		}
		if err := r.commentsCollection.FindOne(ctx, bson.M{"_id": targetID, "deletedAt": nil}).Decode(&comment); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return primitive.NilObjectID, false, nil
			}
			return primitive.NilObjectID, false, err
		}
		if _, ok, err := r.visibleAnchor(ctx, comment.AnchorID, viewerID); err != nil || !ok {
			return primitive.NilObjectID, false, err
		}
		return comment.UserID, true, nil
	}
	return primitive.NilObjectID, false, errors.New("unknown target type")
}

func (r *Repository) visibleAnchor(ctx context.Context, anchorID, viewerID primitive.ObjectID) (primitive.ObjectID, bool, error) {
	var anchor struct {
		UserID     primitive.ObjectID `bson:"userId"`
		Visibility string             `bson:"visibility"`
	}
	err := r.anchorsCollection.FindOne(ctx, bson.M{"_id": anchorID, "deletedAt": nil}).Decode(&anchor)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return primitive.NilObjectID, false, nil
	}
	if err != nil {
		return primitive.NilObjectID, false, err
	}
	if anchor.UserID == viewerID {
		return anchor.UserID, true, nil
	}
	if anchor.Visibility == "private" {
		return primitive.NilObjectID, false, nil
	}
	blocked, err := r.usersCollection.CountDocuments(ctx, bson.M{"_id": anchor.UserID, "blockedUsers": viewerID}, options.Count().SetLimit(1))
	if err != nil {
		return primitive.NilObjectID, false, err
	}
	return anchor.UserID, blocked == 0, nil
}

// HasOpenReport reports whether the reporter already has an unresolved report on the target
func (r *Repository) HasOpenReport(ctx context.Context, reporterID, targetID primitive.ObjectID) (bool, error) {
	n, err := r.reportsCollection.CountDocuments(ctx, bson.M{
		"reporterId": reporterID,
		"targetId":   targetID,
		"open":       true,
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
		SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64(page-1) * int64(limit)).
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

// UpdateReportStatus records an admin's moderation decision and returns the updated report.
// Reopening a closed report fails with ErrAlreadyReported if the same person has since filed a
// new open report on the same target.
func (r *Repository) UpdateReportStatus(ctx context.Context, id, adminID primitive.ObjectID, status, note string) (*Report, error) {
	now := time.Now()
	update := bson.M{"$set": bson.M{
		"status":         status,
		"open":           isOpenStatus(status),
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
	if mongo.IsDuplicateKeyError(err) {
		return nil, ErrAlreadyReported
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
	case "comment":
		coll, projection = r.commentsCollection, bson.M{"anchorId": 1, "userId": 1, "content": 1, "parentId": 1, "deletedAt": 1}
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

// MuteUser adds mutedID to the muter's mutedUsers list (idempotent); ErrListFull at the cap
func (r *Repository) MuteUser(ctx context.Context, muterID, mutedID primitive.ObjectID) error {
	return r.addToCappedList(ctx, muterID, "mutedUsers", mutedID, maxMutedUsers)
}

// UnmuteUser removes mutedID from the muter's mutedUsers list (idempotent)
func (r *Repository) UnmuteUser(ctx context.Context, muterID, mutedID primitive.ObjectID) error {
	_, err := r.usersCollection.UpdateOne(ctx, bson.M{"_id": muterID, "mutedUsers": mutedID}, bson.M{
		"$pull": bson.M{"mutedUsers": mutedID},
		"$set":  bson.M{"updatedAt": time.Now()},
	})
	return err
}

// HideAnchor marks an anchor as "not interested" for the user (idempotent). Only the newest
// 2,000 are kept, so the list can't grow without bound.
func (r *Repository) HideAnchor(ctx context.Context, userID, anchorID primitive.ObjectID) error {
	_, err := r.usersCollection.UpdateOne(ctx, bson.M{"_id": userID, "hiddenAnchors": bson.M{"$ne": anchorID}}, bson.M{
		"$push": bson.M{"hiddenAnchors": bson.M{"$each": bson.A{anchorID}, "$slice": -maxHiddenAnchor}},
		"$set":  bson.M{"updatedAt": time.Now()},
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

// RemoveAnchorFollowsBetween removes follows of each user's anchors by the other user, and
// lowers those anchors' follower counts (never below zero)
func (r *Repository) RemoveAnchorFollowsBetween(ctx context.Context, a, b primitive.ObjectID) error {
	for _, pair := range [][2]primitive.ObjectID{{a, b}, {b, a}} {
		follower, owner := pair[0], pair[1]
		cursor, err := r.anchorFollowsCollection.Aggregate(ctx, mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"userId": follower}}},
			{{Key: "$lookup", Value: bson.M{
				"from": "anchors",
				"let":  bson.M{"aid": "$anchorId"},
				"pipeline": bson.A{
					bson.M{"$match": bson.M{"$expr": bson.M{"$and": bson.A{
						bson.M{"$eq": bson.A{"$_id", "$$aid"}},
						bson.M{"$eq": bson.A{"$userId", owner}},
					}}}},
					bson.M{"$project": bson.M{"_id": 1}},
				},
				"as": "anchor",
			}}},
			{{Key: "$match", Value: bson.M{"anchor": bson.M{"$ne": bson.A{}}}}},
			{{Key: "$project", Value: bson.M{"anchorId": 1}}},
		})
		if err != nil {
			return err
		}
		var rows []struct {
			ID       primitive.ObjectID `bson:"_id"`
			AnchorID primitive.ObjectID `bson:"anchorId"`
		}
		if err := cursor.All(ctx, &rows); err != nil {
			return err
		}
		for _, row := range rows {
			res, err := r.anchorFollowsCollection.DeleteOne(ctx, bson.M{"_id": row.ID})
			if err != nil {
				return err
			}
			if res.DeletedCount == 1 {
				_, _ = r.anchorsCollection.UpdateOne(ctx,
					bson.M{"_id": row.AnchorID, "followerCount": bson.M{"$gte": 1}},
					bson.M{"$inc": bson.M{"followerCount": -1}})
			}
		}
	}
	return nil
}
