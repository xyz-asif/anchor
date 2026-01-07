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

type Repository struct {
	commentsCollection     *mongo.Collection
	commentLikesCollection *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	commentsCollection := db.Collection("comments")
	commentLikesCollection := db.Collection("commentLikes")

	// Create indexes
	commentsCollection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
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
	})

	commentLikesCollection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
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
			return nil, errors.New("comment not found")
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
		return errors.New("comment not found")
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
		return errors.New("comment not found")
	}

	return nil
}

// GetCommentsByAnchor retrieves comments for an anchor with pagination
func (r *Repository) GetCommentsByAnchor(ctx context.Context, anchorID primitive.ObjectID, sort string, page, limit int) ([]Comment, int64, error) {
	filter := bson.M{
		"anchorId":  anchorID,
		"deletedAt": nil,
	}

	// Determine sort order
	var sortOrder bson.D
	switch sort {
	case SortOldest:
		sortOrder = bson.D{{Key: "createdAt", Value: 1}}
	case SortTop:
		sortOrder = bson.D{{Key: "likeCount", Value: -1}, {Key: "createdAt", Value: -1}}
	default: // newest
		sortOrder = bson.D{{Key: "createdAt", Value: -1}}
	}

	opts := options.Find().
		SetSort(sortOrder).
		SetSkip(int64((page - 1) * limit)).
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

// CreateCommentLike creates a like (idempotent)
func (r *Repository) CreateCommentLike(ctx context.Context, commentID, userID primitive.ObjectID) error {
	like := CommentLike{
		ID:        primitive.NewObjectID(),
		CommentID: commentID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}

	_, err := r.commentLikesCollection.InsertOne(ctx, like)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil // Already liked, idempotent
		}
		return err
	}

	return nil
}

// DeleteCommentLike removes a like (idempotent)
func (r *Repository) DeleteCommentLike(ctx context.Context, commentID, userID primitive.ObjectID) error {
	_, err := r.commentLikesCollection.DeleteOne(ctx, bson.M{
		"commentId": commentID,
		"userId":    userID,
	})
	return err
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

// IncrementCommentLikeCount increments/decrements like count
func (r *Repository) IncrementCommentLikeCount(ctx context.Context, commentID primitive.ObjectID, delta int) error {
	_, err := r.commentsCollection.UpdateOne(
		ctx,
		bson.M{"_id": commentID},
		bson.M{
			"$inc": bson.M{"likeCount": delta},
			"$set": bson.M{"updatedAt": time.Now()},
		},
	)
	if err != nil {
		return err
	}

	// Ensure count doesn't go negative
	if delta < 0 {
		_, _ = r.commentsCollection.UpdateOne(ctx,
			bson.M{"_id": commentID, "likeCount": bson.M{"$lt": 0}},
			bson.M{"$set": bson.M{"likeCount": 0}},
		)
	}

	return nil
}
