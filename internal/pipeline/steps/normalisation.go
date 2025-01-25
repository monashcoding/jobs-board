package steps

import (
	"context"
	"strings"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type NormalisationStep struct{}

func NewNormalisationStep() *NormalisationStep {
	return &NormalisationStep{}
}

func (s *NormalisationStep) Name() string {
	return "Normalisation"
}

func (s *NormalisationStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	normalised := []models.Listing{} // process and insert listings

	for _, listing := range listings {
		if ListingIsEOI(listing) {
			continue
		}

		if ListingActiveDateIsInFuture(listing) {
			continue
		}

		listing, err := RemoveEmptyStrings(listing)
		if err != nil {
			utils.Warn("Failed to remove empty strings: %v", err)
		}

		listing, err = StandardiseStudyFields(listing)
		if err != nil {
			utils.Warn("Failed to Standardise Study Fields: %v", err)
		}

		listing, err = StandardiseCompanyName(listing)
		if err != nil {
			utils.Warn("Failed to Standardise Company Name: %v", err)
		}

		listing, err = AssignIndustryByCompany(listing)
		if err != nil {
			utils.Warn("Failed to Assign Industry by Company: %v", err)
		}

		// Append the normalised listing to the result slice
		normalised = append(normalised, listing)
	}

	utils.Success("Normalisation: %d -> %d listings", len(listings), len(normalised))

	return normalised, nil
}

func ListingIsEOI(listing models.Listing) bool {

	eoiString := []string{
		"eoi",
		"eoy",
		"expression of interest",
		"expressions of interest",
		"notify me",
		"register interest",
		"talent community",
		"meet a",
		"virtual internship",
		"virtual experience",
	}

	for _, eoi := range eoiString {
		if strings.Contains(strings.ToLower(listing.Title), eoi) {
			return true
		}
	}

	return false
}

func ListingActiveDateIsInFuture(listing models.Listing) bool {
	if listing.OpenDate == nil {
		return false
	}

	return listing.OpenDate.After(time.Now())
}

func StandardiseStudyFields(listing models.Listing) (models.Listing, error) {
	// Standardise the Study Field strings to a common format
	// Related Ticket (https://www.notion.so/monashcoding/Pipeline-Normalisation-Standardise-Study-Field-Strings-1591e0829dd880a0abcbd9a8ce13c67c)
	return listing, nil
}

func RemoveEmptyStrings(listing models.Listing) (models.Listing, error) {
	// Remove empty strings
	// Related Ticket (https://www.notion.so/monashcoding/1171e0829dd88010a8acdab2a23428e7?v=11c1e0829dd88085a07e000c9515dfca&p=1591e0829dd880528ca3ed948c02f303&pm=s)
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

	return listing, nil
}

func StandardiseCompanyName(listing models.Listing) (models.Listing, error) {
	// Standardise the Study Field strings to a common format
	// Related Ticket (https://www.notion.so/monashcoding/Pipeline-Normalisation-Standardise-Study-Field-Strings-1591e0829dd880a0abcbd9a8ce13c67c)

	// 1. Trim the company name
	standardisedName := strings.TrimSpace(listing.Company.Name)

	// 2. Match with known ambiguous company names
	for _, company := range utils.GetCompanyMap() {
		for _, match := range company.Matches {
			if strings.EqualFold(standardisedName, match) {
				standardisedName = company.Name
				break
			}
		}
	}

	listing.Company.Name = standardisedName

	return listing, nil
}

func AssignIndustryByCompany(listing models.Listing) (models.Listing, error) {
	// Validate input
	companyName := strings.TrimSpace(listing.Company.Name)

	// Get the industry map
	companyToIndustryMap := utils.GetIndustryMap()

	// Find the industry for the company
	for industry, companies := range companyToIndustryMap {
		for _, company := range companies {
			if strings.EqualFold(companyName, company) {
				listing.IndustryField = industry
				break
			}
		}
	}

	return listing, nil
}
