package database

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"

	"go.mongodb.org/mongo-driver/bson"
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

// EnsureIndexes creates the indexes active_jobs depends on. It is safe to call
// on every start: CreateMany is a no-op for an index that already exists.
//
// fingerprint is the upsert key for every write the scraper makes, so without an
// index each of the ~2,800 upserts in a run is a full collection scan, as is the
// $nin sweep that marks unseen jobs outdated and the enrichment-cache lookup.
// The collection is small enough that the build is effectively instant.
//
// The index is deliberately NOT unique. Re-keying reveals collision groups --
// documents that collapse onto one identity under a new algorithm -- and
// cmd/migrate-fingerprints deletes those in a separate phase so the Discord bot
// can tidy the matching threads. A unique index would reject the write instead.
func (m *MongoDB) EnsureIndexes(ctx context.Context) error {
	activeJobs := m.database.Collection("active_jobs")

	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "fingerprint", Value: 1}},
			Options: options.Index().SetName("fingerprint_1"),
		},
		{
			// Read twice per run to count and sweep the outdated backlog.
			Keys:    bson.D{{Key: "outdated", Value: 1}},
			Options: options.Index().SetName("outdated_1"),
		},
	}

	names, err := activeJobs.Indexes().CreateMany(ctx, models)
	if err != nil {
		return fmt.Errorf("failed to create active_jobs indexes: %w", err)
	}

	utils.Log("active_jobs indexes ensured: %v", names)
	return nil
}

// GetCollection returns the MongoDB collection
func (m *MongoDB) GetCollection(name string) *mongo.Collection {
	return m.database.Collection(name)
}

// Disconnect closes the MongoDB connection
func (m *MongoDB) Disconnect(ctx context.Context) error {
	if err := m.client.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}
	return nil
}

func (m *MongoDB) CreateListing(listing models.Listing) error {
	result, err := m.collection.InsertOne(context.Background(), listing)

	if err != nil {
		utils.Log("Create Listing Result: %v", result)
		return fmt.Errorf("failed to insert listing: %w", err)
	}

	return nil
}

func (m *MongoDB) CreateListings(listings []models.Listing) error {
	docs := make([]interface{}, len(listings))

	for i, listing := range listings {
		docs[i] = listing
	}

	result, err := m.collection.InsertMany(context.Background(), docs)

	if err != nil {
		utils.Log("Create Many Listings Result: %v", result)
		return fmt.Errorf("failed to insert multiple listing: %w", err)
	}

	return nil
}

func (m *MongoDB) UpdateListing(listing models.Listing, id string) error {
	result, err := m.collection.UpdateOne(context.Background(), id, listing)

	if err != nil {
		utils.Log("Update Listing Result: %v", result)
		return fmt.Errorf("failed to update listing: %w", err)
	}

	return nil
}

func (m *MongoDB) GetListing(id string) (models.Listing, error) {
	var listing models.Listing
	err := m.collection.FindOne(context.Background(), id).Decode(&listing)

	if err != nil {
		return listing, fmt.Errorf("failed to get listing: %w", err)
	}

	return listing, nil
}

// TODO:: Pass in a filter which can be nil (in which case all listings are returned)
func (m *MongoDB) GetListings() ([]models.Listing, error) {
	var listings []models.Listing
	cursor, err := m.collection.Find(context.Background(), bson.D{}) // Add filter options to bson.D object.

	if err != nil {
		return listings, fmt.Errorf("failed to get all listings: %w", err)
	}

	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var listing models.Listing
		if err := cursor.Decode(&listing); err != nil {
			return listings, fmt.Errorf("failed to decode listing: %w", err)
		}
		listings = append(listings, listing)
	}

	return listings, nil
}

func (m *MongoDB) DeleteListing(id string) error {
	result, err := m.collection.DeleteOne(context.Background(), id)

	if err != nil {
		utils.Log("Delete Listing Result: %v", result)
		return fmt.Errorf("failed to delete listing: %w", err)
	}

	return nil
}

// TODO:: Add filter options to bson.D object.
func (m *MongoDB) DeleteListings() error {
	utils.Log("[!] WARNING: This is a bulk delete operation and could potentially be dangerous")
	result, err := m.collection.DeleteMany(context.Background(), bson.D{})

	if err != nil {
		utils.Log("Delete Many Result: %v", result)
		return fmt.Errorf("failed to delete multiple listings: %w", err)
	}

	return nil
}

func (m *MongoDB) GetDatabase() *mongo.Database {
	return m.database
}
