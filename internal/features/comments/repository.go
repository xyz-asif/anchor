package comments

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrCommentNotFound means the comment doesn't exist (or was deleted); other errors mean the
// lookup itself failed
var ErrCommentNotFound = errors.New("comment not found")

type Repository struct {
	commentsCollection     *mongo.Collection
	commentLikesCollection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	commentsCollection := db.Collection("comments")
	commentLikesCollection := db.Collection("commentLikes")

	// Create indexes
	_, _ = commentsCollection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "anchorId", Value: 1},
				{Key: "deletedAt", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "anchorId", Value: 1},
				{Key: "deletedAt", Value: 1},
				{Key: "likeCount", Value: -1},
				{Key: "createdAt", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "userId", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},
		{
			Keys: bson.D{
				{Key: "parentId", Value: 1},
				{Key: "deletedAt", Value: 1},
				{Key: "createdAt", Value: 1},
			},
		},
	})

	_, _ = commentLikesCollection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "commentId", Value: 1}, {Key: "userId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "userId", Value: 1}, {Key: "commentId", Value: 1}},
		},
	})

	return &Repository{
		commentsCollection:     commentsCollection,
		commentLikesCollection: commentLikesCollection,
	}
}

// CreateComment inserts a new comment
func (r *Repository) CreateComment(ctx context.Context, comment *Comment) error {
	comment.ID = primitive.NewObjectID()
	comment.CreatedAt = time.Now()
	comment.UpdatedAt = time.Now()
	comment.LikeCount = 0
	comment.IsEdited = false

	_, err := r.commentsCollection.InsertOne(ctx, comment)
	return err
}

// GetCommentByID retrieves a comment by ID
func (r *Repository) GetCommentByID(ctx context.Context, commentID primitive.ObjectID) (*Comment, error) {
	var comment Comment
	err := r.commentsCollection.FindOne(ctx, bson.M{
		"_id":       commentID,
		"deletedAt": nil,
	}).Decode(&comment)

	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrCommentNotFound
		}
		return nil, err
	}

	return &comment, nil
}

// UpdateComment updates a comment
func (r *Repository) UpdateComment(ctx context.Context, commentID primitive.ObjectID, updates bson.M) error {
	updates["updatedAt"] = time.Now()

	result, err := r.commentsCollection.UpdateOne(
		ctx,
		bson.M{"_id": commentID, "deletedAt": nil},
		bson.M{"$set": updates},
	)

	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return ErrCommentNotFound
	}

	return nil
}

// SoftDeleteComment soft deletes a comment
func (r *Repository) SoftDeleteComment(ctx context.Context, commentID primitive.ObjectID) error {
	result, err := r.commentsCollection.UpdateOne(
		ctx,
		bson.M{"_id": commentID, "deletedAt": nil},
		bson.M{"$set": bson.M{"deletedAt": time.Now(), "updatedAt": time.Now()}},
	)

	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return ErrCommentNotFound
	}

	return nil
}

// GetCommentsByAnchor retrieves comments for an anchor with pagination
// Comments by hiddenAuthors (block relationships with the viewer) are excluded in the query.
func (r *Repository) GetCommentsByAnchor(ctx context.Context, anchorID primitive.ObjectID, hiddenAuthors []primitive.ObjectID, sort string, page, limit int) ([]Comment, int64, error) {
	// Top-level comments only; replies are fetched per comment ("parentId": nil also matches
	// comments created before replies existed, which have no parentId field)
	filter := bson.M{
		"anchorId":  anchorID,
		"parentId":  nil,
		"deletedAt": nil,
	}
	if len(hiddenAuthors) > 0 {
		filter["userId"] = bson.M{"$nin": hiddenAuthors}
	}

	// Determine sort order (_id last, so equal values keep a stable order across pages)
	var sortOrder bson.D
	switch sort {
	case SortOldest:
		sortOrder = bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}
	case SortTop:
		sortOrder = bson.D{{Key: "likeCount", Value: -1}, {Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}
	default: // newest
		sortOrder = bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}
	}

	opts := options.Find().
		SetSort(sortOrder).
		SetSkip(int64(page-1) * int64(limit)).
		SetLimit(int64(limit))

	cursor, err := r.commentsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var comments []Comment
	if err = cursor.All(ctx, &comments); err != nil {
		return nil, 0, err
	}

	total, err := r.commentsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return comments, total, nil
}

// CreateCommentLike creates a like. created is false when it already existed.
func (r *Repository) CreateCommentLike(ctx context.Context, commentID, userID primitive.ObjectID) (created bool, err error) {
	like := CommentLike{
		ID:        primitive.NewObjectID(),
		CommentID: commentID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}

	if _, err := r.commentLikesCollection.InsertOne(ctx, like); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return false, nil // already liked
		}
		return false, err
	}
	return true, nil
}

// DeleteCommentLike removes a like. deleted is false when there was nothing to remove.
func (r *Repository) DeleteCommentLike(ctx context.Context, commentID, userID primitive.ObjectID) (deleted bool, err error) {
	result, err := r.commentLikesCollection.DeleteOne(ctx, bson.M{
		"commentId": commentID,
		"userId":    userID,
	})
	if err != nil {
		return false, err
	}
	return result.DeletedCount > 0, nil
}

// ExistsCommentLike checks if a like exists
func (r *Repository) ExistsCommentLike(ctx context.Context, commentID, userID primitive.ObjectID) (bool, error) {
	count, err := r.commentLikesCollection.CountDocuments(ctx, bson.M{
		"commentId": commentID,
		"userId":    userID,
	})
	return count > 0, err
}

// GetUserLikedComments batch checks which comments a user has liked
func (r *Repository) GetUserLikedComments(ctx context.Context, userID primitive.ObjectID, commentIDs []primitive.ObjectID) (map[primitive.ObjectID]bool, error) {
	if len(commentIDs) == 0 {
		return make(map[primitive.ObjectID]bool), nil
	}

	cursor, err := r.commentLikesCollection.Find(ctx, bson.M{
		"userId":    userID,
		"commentId": bson.M{"$in": commentIDs},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	result := make(map[primitive.ObjectID]bool)
	for cursor.Next(ctx) {
		var like CommentLike
		if err := cursor.Decode(&like); err == nil {
			result[like.CommentID] = true
		}
	}

	return result, nil
}

// IncrementCommentLikeCount atomically adjusts a comment's like count (never below zero)
func (r *Repository) IncrementCommentLikeCount(ctx context.Context, commentID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": commentID}
	if delta < 0 {
		filter["likeCount"] = bson.M{"$gte": -delta}
	}
	_, err := r.commentsCollection.UpdateOne(ctx, filter, bson.M{"$inc": bson.M{"likeCount": delta}})
	return err
}

// GetReplies returns the replies to a top-level comment, oldest first
func (r *Repository) GetReplies(ctx context.Context, parentID primitive.ObjectID, hiddenAuthors []primitive.ObjectID, page, limit int) ([]Comment, int64, error) {
	filter := bson.M{"parentId": parentID, "deletedAt": nil}
	if len(hiddenAuthors) > 0 {
		filter["userId"] = bson.M{"$nin": hiddenAuthors}
	}

	total, err := r.commentsCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: 1}, {Key: "_id", Value: 1}}).
		SetSkip(int64(page-1) * int64(limit)).
		SetLimit(int64(limit))
	cursor, err := r.commentsCollection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	replies := []Comment{}
	if err := cursor.All(ctx, &replies); err != nil {
		return nil, 0, err
	}
	return replies, total, nil
}

// IncrementReplyCount adjusts a comment's reply count, never going below zero
func (r *Repository) IncrementReplyCount(ctx context.Context, commentID primitive.ObjectID, delta int) error {
	filter := bson.M{"_id": commentID}
	if delta < 0 {
		filter["replyCount"] = bson.M{"$gte": -delta}
	}
	_, err := r.commentsCollection.UpdateOne(ctx, filter, bson.M{"$inc": bson.M{"replyCount": delta}})
	return err
}

// SoftDeleteReplies soft-deletes all live replies of a comment and returns how many were deleted
func (r *Repository) SoftDeleteReplies(ctx context.Context, parentID primitive.ObjectID) (int64, error) {
	now := time.Now()
	result, err := r.commentsCollection.UpdateMany(ctx,
		bson.M{"parentId": parentID, "deletedAt": nil},
		bson.M{"$set": bson.M{"deletedAt": now, "updatedAt": now}},
	)
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}
