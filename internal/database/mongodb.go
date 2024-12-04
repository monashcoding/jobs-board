package database

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type MongoDB struct {
	client     *mongo.Client
	database   *mongo.Database
	collection *mongo.Collection
}

// NewMongoDB creates a new MongoDB instance
func NewMongoDB(ctx context.Context) (*MongoDB, error) {
	// Configure client options
	clientOptions := options.Client().
		ApplyURI(os.Getenv("MONGODB_URI")).
		SetConnectTimeout(10 * time.Second).
		SetMaxConnIdleTime(60 * time.Second).
		SetMinPoolSize(5).
		SetMaxPoolSize(100)

	// Create client
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create MongoDB client: %w", err)
	}

	// Ping database to verify connection
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(ctxPing, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Get database and collection
	database := client.Database(os.Getenv("MONGODB_DATABASE"))
	collection := database.Collection(os.Getenv("MONGODB_COLLECTION"))

	return &MongoDB{
		client:     client,
		database:   database,
		collection: collection,
	}, nil
}

// GetCollection returns the MongoDB collection
func (m *MongoDB) GetCollection() *mongo.Collection {
	return m.collection
}

// Disconnect closes the MongoDB connection
func (m *MongoDB) Disconnect(ctx context.Context) error {
	if err := m.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}
	return nil
}
