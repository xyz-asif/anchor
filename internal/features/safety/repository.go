package safety

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Repository struct {
	reportsCollection *mongo.Collection
	usersCollection   *mongo.Collection
}

func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		reportsCollection: db.Collection("reports"),
		usersCollection:   db.Collection("users"),
	}
}

func (r *Repository) CreateReport(ctx context.Context, report *Report) error {
	report.CreatedAt = time.Now()
	report.UpdatedAt = time.Now()
	report.Status = "pending"

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

	filter := bson.M{"_id": blockerID}
	update := bson.M{
		"$addToSet": bson.M{"blockedUsers": blockedID},
		"$set":      bson.M{"updatedAt": time.Now()},
	}

	_, err := r.usersCollection.UpdateOne(ctx, filter, update)
	return err
}
