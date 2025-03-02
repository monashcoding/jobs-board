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
		hash := fmt.Sprintf("%s-%s", listing.Title, listing.Company.Name)

		existingModel, ok := dict[hash]

		if !ok {
			dict[hash] = listing
		} else {
			existingModel.SourceURLs = append(existingModel.SourceURLs, listing.SourceURLs...)
			utils.Log("Deduplication: Duplicate listing found: %s | %s", listing.Company.Name, listing.Title)
			//utils.Custom("Deduplication: Duplicate listing found: %s | %s", listing.Company.Name, listing.Title)
		}
	}

	var uniqueListings []models.Listing
	for _, listing := range dict {
		uniqueListings = append(uniqueListings, listing)
	}

	dict2 := make(map[string]models.Listing)
	sameURLCompany := utils.GetSameURLCompanyMap()

	for _, listing := range uniqueListings {
		var hash string
		if sameURLCompany[listing.Company.Name] {
			hash = fmt.Sprintf("%s-%s", listing.Title, listing.Company.Name)
		} else {
			hash = fmt.Sprintf("%s", *listing.ApplicationURL)
		}

		existingModel, ok := dict2[hash]

		if !ok {
			dict2[hash] = listing
		} else {
			existingModel.SourceURLs = append(existingModel.SourceURLs, listing.SourceURLs...)
			utils.Log("App url Deduplication: Duplicate listing found: %s | %s", listing.Company.Name, listing.Title)
		}
	}

	var uniqueListings2 []models.Listing
	for _, listing := range dict2 {
		uniqueListings2 = append(uniqueListings2, listing)
	}

	// Loop through uniqueListings2 and if the source of the listing is not the same as the source override map, remove it
	sourceOverrideMap := utils.GetSourceOverrideMap()
	var uniqueListings3 []models.Listing
	for _, listing := range uniqueListings2 {
		if sourceOverrideMap[listing.Company.Name] == listing.Source {
			uniqueListings3 = append(uniqueListings3, listing)
			// else if company name is not in sourceOverrideMap, add it to uniqueListings3
		} else if _, ok := sourceOverrideMap[listing.Company.Name]; !ok {
			uniqueListings3 = append(uniqueListings3, listing)
		} else {
			utils.Log("Deduplication: Source override map mismatch: %s | %s", listing.Company.Name, listing.Title)
		}
	}

	// process and insert listings. objects with existing IDs will be overwritten
	utils.Success("Deduplication: %d -> %d listings", len(listings), len(uniqueListings3))
	utils.Custom("Deduplication: %d -> %d listings", len(listings), len(uniqueListings3))

	return uniqueListings3, nil
}
