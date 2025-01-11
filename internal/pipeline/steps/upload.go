package steps

import (
	"context"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type UploadStep struct {
	mongoDb *database.MongoDB
}

func NewMongoDBStep(mongoDb *database.MongoDB) *UploadStep {
	return &UploadStep{mongoDb}
}

func (s *UploadStep) Name() string {
	return "Upload"
}

func (s *UploadStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	/* ---------------------------------------------------------- /
	| Naive sync strategy [To be replaced after MVP]        		  |
	| 1. Delete all listings from DB                              |
	| 2. Insert all listings from pipeline                        |
	| 						                                                |
	| In future, need to think about a smarter way to handle      |
	| updates and deletions. (How do we know a listing is expired |
	| or updated?)                                                |
	| ---------------------------------------------------------- */

	if len(listings) == 0 {
		return listings, nil
	}

	// Sync with database
	utils.Log("Starting database sync [...]")

	utils.Log("Database sync step 1: Deleting all listings [...]")

	err := s.mongoDb.DeleteListings() // Delete all listings
	if err != nil {
		utils.Error("[X] Failed to delete listings: %v", err)
	}
	utils.Log("Database sync step 1: Successfully deleted all listings [\u2713]")

	utils.Log("Database sync step 2: Inserting all listings [...]")
	err = s.mongoDb.CreateListings(listings) // Insert all listings
	if err != nil {
		utils.Error("[X] Failed to insert listings: %v", err)
	}
	utils.Log("Database sync step 2: Successfully inserted all listings [\u2713]")

	dbListings, err := s.mongoDb.GetListings()
	if err != nil {
		utils.Error("[X] Failed to get listings from database: %v", err)
	}

	utils.Log("Database Contains %d Listings", len(dbListings))

	return dbListings, nil
}
