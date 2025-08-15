package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Connection represents a database connection
type Connection struct {
	Client   *mongo.Client
	Database *mongo.Database
	URI      string
	DBName   string
}

// Config represents database configuration
type Config struct {
	URI      string
	DBName   string
	Username string
	Password string
	Timeout  time.Duration
	MaxPool  uint64
	MinPool  uint64
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		Timeout: 10 * time.Second,
		MaxPool: 100,
		MinPool: 5,
	}
}

// NewConnection creates a new database connection
func NewConnection(cfg *Config) (*Connection, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Build connection string
	var uri string
	if cfg.Username != "" && cfg.Password != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s", cfg.Username, cfg.Password, cfg.URI)
	} else {
		uri = cfg.URI
	}

	// Client options
	clientOptions := options.Client().ApplyURI(uri)
	clientOptions.SetMaxPoolSize(cfg.MaxPool)
	clientOptions.SetMinPoolSize(cfg.MinPool)
	clientOptions.SetMaxConnIdleTime(30 * time.Second)
	clientOptions.SetServerSelectionTimeout(5 * time.Second)
	clientOptions.SetSocketTimeout(10 * time.Second)

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Test connection
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	connection := &Connection{
		Client:   client,
		Database: client.Database(cfg.DBName),
		URI:      cfg.URI,
		DBName:   cfg.DBName,
	}

	return connection, nil
}

// Close closes the database connection
func (c *Connection) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Client.Disconnect(ctx)
}

// Ping checks if the database is accessible
func (c *Connection) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Client.Ping(ctx, readpref.Primary())
}

// IsConnected checks if the connection is still active
func (c *Connection) IsConnected() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.Client.Ping(ctx, readpref.Primary()) == nil
}

// GetCollection returns a collection by name
func (c *Connection) GetCollection(name string) *mongo.Collection {
	return c.Database.Collection(name)
}

// GetDatabase returns the database instance
func (c *Connection) GetDatabase() *mongo.Database {
	return c.Database
}

// GetClient returns the MongoDB client
func (c *Connection) GetClient() *mongo.Client {
	return c.Client
}

// WithTransaction executes a function within a transaction
func (c *Connection) WithTransaction(ctx context.Context, fn func(sessCtx mongo.SessionContext) error) error {
	session, err := c.Client.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(sessCtx)
	})
	return err
}

// HealthCheck performs a comprehensive health check
func (c *Connection) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check connection
	if err := c.Client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Check database access
	if err := c.Database.RunCommand(ctx, map[string]interface{}{"ping": 1}).Err(); err != nil {
		return fmt.Errorf("database access failed: %w", err)
	}

	return nil
}

// GetStats returns database statistics
func (c *Connection) GetStats() map[string]interface{} {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats := make(map[string]interface{})

	// Get database stats
	if dbStats := c.Database.RunCommand(ctx, map[string]interface{}{"dbStats": 1}); dbStats.Err() == nil {
		var result map[string]interface{}
		if err := dbStats.Decode(&result); err == nil {
			stats["database"] = result
		}
	}

	// Get collection stats
	collections, err := c.Database.ListCollectionNames(ctx, map[string]interface{}{})
	if err == nil {
		stats["collections"] = len(collections)
	}

	return stats
}

// CreateIndex creates an index on a collection
func (c *Connection) CreateIndex(collectionName string, keys interface{}, options *options.IndexOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := c.Database.Collection(collectionName)
	_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    keys,
		Options: options,
	})
	return err
}

// DropIndex drops an index from a collection
func (c *Connection) DropIndex(collectionName, indexName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := c.Database.Collection(collectionName)
	_, err := collection.Indexes().DropOne(ctx, indexName)
	return err
}
