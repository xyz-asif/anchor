package search

import (
	"context"
	"log"

	"github.com/xyz-asif/gotodo/internal/pkg/searchterms"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const backfillBatch = 500

// Backfill computes search terms for every anchor and user that doesn't have the current
// version of them. Each write only applies if the document's text is unchanged since it was
// read, so it never overwrites the terms of an edit made meanwhile.
func (r *Repository) Backfill(ctx context.Context) error {
	anchors, err := backfill(ctx, r.anchorsCollection, bson.M{"title": 1, "description": 1, "tags": 1},
		func(doc bson.M) (bson.M, *searchterms.Fields) {
			var a struct {
				Title       string   `bson:"title"`
				Description string   `bson:"description"`
				Tags        []string `bson:"tags"`
			}
			decode(doc, &a)
			return bson.M{"title": doc["title"], "description": doc["description"], "tags": doc["tags"]},
				searchterms.ForAnchor(a.Title, a.Description, a.Tags)
		})
	if err != nil {
		return err
	}
	users, err := backfill(ctx, r.usersCollection, bson.M{"username": 1, "displayName": 1},
		func(doc bson.M) (bson.M, *searchterms.Fields) {
			var u struct {
				Username    string `bson:"username"`
				DisplayName string `bson:"displayName"`
			}
			decode(doc, &u)
			return bson.M{"username": doc["username"], "displayName": doc["displayName"]},
				searchterms.ForUser(u.Username, u.DisplayName)
		})
	if err != nil {
		return err
	}
	if anchors+users > 0 {
		log.Printf("search: indexed %d existing anchors and %d existing users", anchors, users)
	}
	return nil
}

func decode(doc bson.M, out interface{}) {
	raw, _ := bson.Marshal(doc)
	_ = bson.Unmarshal(raw, out)
}

func backfill(ctx context.Context, coll *mongo.Collection, projection bson.M,
	compute func(bson.M) (unchanged bson.M, fields *searchterms.Fields)) (int, error) {
	cursor, err := coll.Find(ctx, bson.M{"search.v": bson.M{"$ne": searchterms.Version}},
		options.Find().SetProjection(projection).SetBatchSize(backfillBatch))
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	done := 0
	var batch []mongo.WriteModel
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := coll.BulkWrite(ctx, batch, options.BulkWrite().SetOrdered(false))
		done += len(batch)
		batch = batch[:0]
		return err
	}
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return done, err
		}
		filter, fields := compute(doc)
		filter["_id"] = doc["_id"]
		batch = append(batch, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(bson.M{"$set": bson.M{"search": fields}}))
		if len(batch) == backfillBatch {
			if err := flush(); err != nil {
				return done, err
			}
		}
	}
	if err := cursor.Err(); err != nil {
		return done, err
	}
	return done, flush()
}
