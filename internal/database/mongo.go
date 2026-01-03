// ================== internal/database/mongo.go ==================
package database

import (
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

type MongoDB struct {
	Client   *mongo.Client
	Database *mongo.Database
}

func Connect(uri, dbName string) (*MongoDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // you are telling yourself that you will start the func and wait till 10 sec for the db to answer you back
	defer cancel()                                                           // this is basically using the above cancel func and calling it at the end of this func using defer

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	// Ping to verify connection that been made above code
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	return &MongoDB{
		Client:   client,
		Database: client.Database(dbName),
	}, nil
}

func (m *MongoDB) Disconnect(ctx context.Context) error {
	return m.Client.Disconnect(ctx)
}
