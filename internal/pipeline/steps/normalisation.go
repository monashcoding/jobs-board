package steps

import (
	"context"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type NormalizationStep struct{}

func NewNormalizationStep() *NormalizationStep {
	return &NormalizationStep{}
}

func (s *NormalizationStep) Name() string {
	return "Normalization"
}

func (s *NormalizationStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	normalized := make([]models.Listing, len(listings))
	// process and insert listings

	for i, listing := range listings {
		normalizedListing, err := StandardiseStudyFields(listing)	
		if err != nil {
			utils.Error("Error in StandardiseStudyFields: %v", err)
		}

		normalizedListing, err = RemoveEmptyStrings(normalizedListing)
		if err != nil {
			utils.Error("Error in RemoveEmptyStrings: %v", err)
		}

		normalized[i] = normalizedListing
	}

	return normalized, nil
}

func StandardiseStudyFields(listing models.Listing) (models.Listing, error) {
	// Standardise the Study Field strings to a common format
	// Related Ticket (https://www.notion.so/monashcoding/Pipeline-Normalisation-Standardise-Study-Field-Strings-1591e0829dd880a0abcbd9a8ce13c67c)
	utils.Log("Add your own logs")


	return listing, nil
}


func RemoveEmptyStrings(listing models.Listing) (models.Listing, error) {
	// Remove empty strings
	// Related Ticket (https://www.notion.so/monashcoding/1171e0829dd88010a8acdab2a23428e7?v=11c1e0829dd88085a07e000c9515dfca&p=1591e0829dd880528ca3ed948c02f303&pm=s)
	utils.Log("Add your own logs")

	return listing, nil
}
