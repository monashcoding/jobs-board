package steps

import (
	"context"
	"fmt"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type DeduplicationStep struct {
	seenHashes map[string]bool
}

func NewDeduplicationStep() *DeduplicationStep {
	return &DeduplicationStep{
		seenHashes: make(map[string]bool),
	}
}

func (s *DeduplicationStep) Name() string {
	return "Deduplication"
}

func (s *DeduplicationStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	/*------------------------------------------------*/
	/* Deduplication Strategy:                        */
	/* Listing counts as a duplicate if:				      */
	/* - Title is identical											      */
	/* - Company is identical											    */
	/* - Locations is identical											  */
	/* - ApplicationURL is identical								  */
	/*------------------------------------------------*/

	dict := make(map[string]models.Listing)

	for _, listing := range listings {
		hash := fmt.Sprintf("%s-%s-%s-%s", listing.Title, listing.Company.Name, listing.Locations, *listing.ApplicationURL)

		existingModel, ok := dict[hash]

		if !ok {
			dict[hash] = listing
		} else {
			existingModel.SourceURLs = append(existingModel.SourceURLs, listing.SourceURLs...)
			utils.Log("Deduplication: Duplicate listing found: %s | %s", listing.Company.Name, listing.Title)
		}
	}

	var uniqueListings []models.Listing
	for _, listing := range dict {
		uniqueListings = append(uniqueListings, listing)
	}

	// process and insert listings. objects with existing IDs will be overwritten
	utils.Log("Deduplication: %d -> %d listings", len(listings), len(uniqueListings))

	return uniqueListings, nil
}
