package todos

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

func (r *Repository) CountByUser(ctx context.Context, userID string) (int64, error) {
	count, err := r.collection.CountDocuments(ctx, bson.M{"userId": userID})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func NewRepository(db *mongo.Database) *Repository {
	collection := db.Collection("todos")

	// Create indexes
	collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}}},
		{Keys: bson.D{{Key: "completed", Value: 1}}},
		{Keys: bson.D{{Key: "createdAt", Value: -1}}},
	})

	return &Repository{collection: collection}
}

func (r *Repository) Create(ctx context.Context, todo *Todo) error {
	todo.CreatedAt = time.Now()
	todo.UpdatedAt = time.Now()
	todo.Completed = false

	result, err := r.collection.InsertOne(ctx, todo)
	if err != nil {
		return err
	}

	todo.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id, userID string) (*Todo, error) {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errors.New("Invalid todo ID")
	}

	var todo Todo
	err = r.collection.FindOne(ctx, bson.M{
		"_id":    objectID,
		"userId": userID,
	}).Decode(&todo)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return &todo, nil
}

func (r *Repository) Update(ctx context.Context, id, userID string, update bson.M) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return errors.New("Invalid todo ID")
	}

	update["updatedAt"] = time.Now()

	result, err := r.collection.UpdateOne(
		ctx,
		bson.M{"_id": objectID, "userId": userID},
		bson.M{"$set": update},
	)

	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return errors.New("Todo not found")
	}

	return nil
}

func (r *Repository) Delete(ctx context.Context, id, userID string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return errors.New("Invalid todo ID")
	}

	result, err := r.collection.DeleteOne(ctx, bson.M{
		"_id":    objectID,
		"userId": userID,
	})

	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return errors.New("Todo not found")
	}

	return nil
}

func (r *Repository) List(ctx context.Context, userID string, completed *bool, limit int) ([]Todo, error) {
	filter := bson.M{"userId": userID}
	if completed != nil {
		filter["completed"] = *completed
	}

	opts := options.Find()
	opts.SetSort(bson.D{{Key: "createdAt", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var todos []Todo
	if err := cursor.All(ctx, &todos); err != nil {
		return nil, err
	}

	if todos == nil {
		todos = []Todo{}
	}

	return todos, nil
}
