package steps

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type UploadStep struct {
	collection *mongo.Collection
}

func NewMongoDBStep(collection *mongo.Collection) *UploadStep {
	return &UploadStep{
		collection: collection,
	}
}

func (s *UploadStep) Name() string {
	return "Upload"
}

func (s *UploadStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	if len(listings) == 0 {
		return listings, nil
	}

	operations := make([]mongo.WriteModel, len(listings))
	for i, listing := range listings {
		operations[i] = mongo.NewUpdateOneModel().
			SetUpsert(true).
			SetFilter(bson.M{"id": listing.ID}).
			SetUpdate(bson.M{"$set": listing})
	}

	_, err := s.collection.BulkWrite(ctx, operations)
	if err != nil {
		return nil, err
	}

	return listings, nil
}
