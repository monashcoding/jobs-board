package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

var studyFieldMapping = map[string]string{
	"Business & Management":                 "Business & Management",
	"Creative Arts":                         "Creative Arts",
	"Engineering & Mathematics":             "Engineering & Mathematics",
	"Food, Hospitality & Personal Services": "Food, Hospitality & Personal Services",
	"Humanities, Arts & Social Sciences":    "Humanities, Arts & Social Sciences",
	"IT & Computer Science":                 "IT & Computer Science",
	"Law, Legal Studies & Justice":          "Law, Legal Studies & Justice",
	"Medical & Health Sciences":             "Medical & Health Sciences",
	"Property & Built Environment":          "Property & Built Environment",
	"Sciences":                              "Sciences",
	"Teaching & Education":                  "Teaching & Education",
	"Trades & Services":                     "Trades & Services",

	"Accounting":                           "Business & Management",
	"Actuarial Studies":                    "Sciences",
	"Administration":                       "Business & Management",
	"Agriculture":                          "Sciences",
	"Architecture":                         "Property & Built Environment",
	"Arts and Humanities":                  "Humanities, Arts & Social Sciences",
	"Aviation":                             "Engineering & Mathematics",
	"Banking and Finance":                  "Business & Management",
	"Business and Commerce":                "Business & Management",
	"Communications":                       "Humanities, Arts & Social Sciences",
	"Compliance":                           "Law, Legal Studies & Justice",
	"Computer Science":                     "IT & Computer Science",
	"Construction":                         "Property & Built Environment",
	"Consulting":                           "Business & Management",
	"Customer Service":                     "Trades & Services",
	"Cyber Security":                       "IT & Computer Science",
	"Data Science and Analytics":           "IT & Computer Science",
	"Defence":                              "Law, Legal Studies & Justice",
	"Design and User Experience":           "Creative Arts",
	"Economics":                            "Business & Management",
	"Education":                            "Teaching & Education",
	"Engineering":                          "Engineering & Mathematics",
	"Engineering - Aerospace/Aeronautical": "Engineering & Mathematics",
	"Engineering - Chemical/Processing":    "Engineering & Mathematics",
	"Engineering - Civil/Structural":       "Engineering & Mathematics",
	"Engineering - Electrical":             "Engineering & Mathematics",
	"Engineering - Environmental":          "Engineering & Mathematics",
	"Engineering - Geotechnical":           "Engineering & Mathematics",
	"Engineering - Mechanical":             "Engineering & Mathematics",
	"Engineering - Mechatronics":           "Engineering & Mathematics",
	"Engineering - Mining":                 "Engineering & Mathematics",
	"Engineering - Petroleum":              "Engineering & Mathematics",
	"Engineering - Software":               "IT & Computer Science",
	"Environment":                          "Sciences",
	"Exercise Physiology":                  "Medical & Health Sciences",
	"Fast Moving Consumer Goods":           "Business & Management",
	"Food Technology":                      "Food, Hospitality & Personal Services",
	"Funds Management":                     "Business & Management",
	"Geology":                              "Sciences",
	"Government":                           "Law, Legal Studies & Justice",
	"Health":                               "Medical & Health Sciences",
	"Health Policy, Promotion and Administration": "Medical & Health Sciences",
	"Horticulture":                    "Sciences",
	"Hospitality, Sports and Tourism": "Food, Hospitality & Personal Services",
	"Human Resources":                 "Business & Management",
	"Industrial Design":               "Creative Arts",
	"Information Systems":             "IT & Computer Science",
	"Information Technology":          "IT & Computer Science",
	"Insurance":                       "Business & Management",
	"Intelligence":                    "Law, Legal Studies & Justice",
	"Investment Banking":              "Business & Management",
	"Journalism":                      "Humanities, Arts & Social Sciences",
	"Law":                             "Law, Legal Studies & Justice",
	"Logistics and Supply Chain":      "Business & Management",
	"Management":                      "Business & Management",
	"Marine Biology":                  "Sciences",
	"Marketing and Sales":             "Business & Management",
	"Mathematics":                     "Sciences",
	"Media and Advertising":           "Humanities, Arts & Social Sciences",
	"Medical and Biomedical Science":  "Medical & Health Sciences",
	"Medicine":                        "Medical & Health Sciences",
	"Mining Oil and Gas":              "Engineering & Mathematics",
	"Nursing and Midwifery":           "Medical & Health Sciences",
	"Operations":                      "Business & Management",
	"Pharmacy and Pharmacology":       "Medical & Health Sciences",
	"Physics":                         "Sciences",
	"Physiotherapy, Occupational Therapy and Rehabilitation": "Medical & Health Sciences",
	"Planning and Surveying":                                 "Property & Built Environment",
	"Procurement":                                            "Business & Management",
	"Project Management":                                     "Business & Management",
	"Property":                                               "Property & Built Environment",
	"Psychology, Counselling and Social Work":                "Medical & Health Sciences",
	"Radiography and Medical Imaging":                        "Medical & Health Sciences",
	"Recruitment":                                            "Business & Management",
	"Research and Development":                               "Sciences",
	"Retail":                                                 "Trades & Services",
	"Science":                                                "Sciences",
	"Speech Pathology":                                       "Medical & Health Sciences",
	"Statistics":                                             "Sciences",
	"Telecommunications":                                     "IT & Computer Science",
	"Transport":                                              "Engineering & Mathematics",
	"Utilities":                                              "Engineering & Mathematics",
}

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
	uniqueStudyFields := make(map[string]bool)

	for _, field := range listing.StudyFields {
		// Check if the study field exists in the mapping
		conciseField, exists := studyFieldMapping[field]
		if !exists {
			return listing, fmt.Errorf("unknown study field: %s in listing: %+v", field, listing)
		}
		uniqueStudyFields[conciseField] = true
	}

	// Convert map keys back to a list
	var standardizedFields []string
	for field := range uniqueStudyFields {
		standardizedFields = append(standardizedFields, field)
	}

	// Assign the unique, standardized fields back to the listing
	listing.StudyFields = standardizedFields

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
