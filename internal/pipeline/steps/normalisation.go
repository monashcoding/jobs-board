package steps

import (
	"context"
	"strings"
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

	for _, listing := range listings {
		// Check if Title contains "eoi" or "eoy"
		if strings.Contains(strings.ToLower(listing.Title), "eoi") || strings.Contains(strings.ToLower(listing.Title), "eoy") {
			continue // Skip this listing entirely
		}

		// Trim string fields
		listing.Title = strings.TrimSpace(listing.Title)

		if listing.Description != nil {
			trimmedDescription := strings.TrimSpace(*listing.Description)
			listing.Description = &trimmedDescription
		}

		if listing.ApplicationURL != nil {
			trimmedApplicationURL := strings.TrimSpace(*listing.ApplicationURL)
			listing.ApplicationURL = &trimmedApplicationURL
		}


		for j, sourceURL := range listing.SourceURLs {
			listing.SourceURLs[j] = strings.TrimSpace(sourceURL)
		}

		// Filter out "..." from StudyFields
		filteredStudyFields := []string{}
		for _, studyField := range listing.StudyFields {
			trimmedField := strings.TrimSpace(studyField)
			if trimmedField != "..." {
				filteredStudyFields = append(filteredStudyFields, trimmedField)
			}
		}
		listing.StudyFields = filteredStudyFields

		// Append the normalized listing to the result slice
		normalized = append(normalized, listing)
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
