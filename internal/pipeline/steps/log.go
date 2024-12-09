package steps

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
	"strings"
)

type LogStep struct {
	prefix string
}

func NewLogStep(prefix string) *LogStep {
	return &LogStep{
		prefix: prefix,
	}
}

func (s *LogStep) Name() string {
	return "Log"
}

func (s *LogStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	utils.Log("\n=== %s: Processing %d listings ===", s.prefix, len(listings))

	for _, listing := range listings {
		// Create a compact location string
		locations := strings.Join(compactStringSlice(locationsToStrings(listing.Locations)), "/")

		// Get company name or "Unknown"
		company := listing.Company.Name
		if company == "" {
			company = "Unknown"
		}

		// Format close date if exists
		closeDate := "No close date"
		if listing.CloseDate != nil {
			closeDate = listing.CloseDate.Format("2006-01-02")
		}

		// Create compact string for study fields
		fields := strings.Join(compactStringSlice(studyFieldsToStrings(listing.StudyFields)), "/")
		if fields == "" {
			fields = "Any"
		}

		// Print compact single-line summary
		utils.Log("[%s] %s @ %s | Type: %s | Loc: %s | Fields: %s | Closes: %s | URL: %s",
			listing.ID.Hex()[:8],
			truncateString(listing.Title, 40),
			company,
			listing.Type,
			locations,
			fields,
			closeDate,
			truncateString(listing.ApplicationURL, 40),
		)
	}
	return listings, nil
}

// Helper functions

func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}

func compactStringSlice(slice []string) []string {
	if len(slice) <= 3 {
		return slice
	}
	return append(slice[:2], "...")
}

func locationsToStrings(locs []models.LocationType) []string {
	result := make([]string, len(locs))
	for i, loc := range locs {
		result[i] = string(loc)
	}
	return result
}

func studyFieldsToStrings(fields []models.StudyField) []string {
	result := make([]string, len(fields))
	for i, field := range fields {
		result[i] = string(field)
	}
	return result
}
