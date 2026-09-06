package gradcon

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

func (s *GradconScraper) MergeDetailedDescriptionsWithListings(listings []models.Listing, allCompanyIds []string) {
	// find unique company ids
	uniqueCompanyIds := make(map[string]bool)
	for _, companyId := range allCompanyIds {
		uniqueCompanyIds[companyId] = true
	}

	gradConnectCompanyListingData := []GradConnectDecriptionData{}

	// Call the gradconnect customer organisation API for each company Id
	utils.Log("Getting detailed descriptions for %d companies", len(uniqueCompanyIds))

	counter := 0
	skipped := 0
	totalCompanies := len(uniqueCompanyIds)

	for companyId := range uniqueCompanyIds {
		utils.Log("Processing company API request %d/%d (ID: %s)", counter, totalCompanies, companyId)
		companyApiReponse, err := SendCompanyAPIRequest(companyId, s.unblocker)
		if err != nil {
			// GradConnection returns intermittent 500s for individual companies.
			// Skip this one rather than losing the whole scrape.
			utils.Warn("error getting company api response: %v", err)
			skipped++
			counter++
			continue
		}

		gradConnectCompanyListingData = append(gradConnectCompanyListingData, *companyApiReponse...)
		counter++
	}

	mergeCount := 0

	// Merge the detailed descriptions with the listings
	for i, listing := range listings {
		for _, companyListing := range gradConnectCompanyListingData {
			match := regexp.MustCompile(`[^/]*$`).FindString(listing.SourceURLs[0])

			if companyListing.Slug == match {
				mergeCount++
				trimmedContent := strings.TrimPrefix(companyListing.Content, "{'body': '")
				trimmedContent = strings.TrimPrefix(trimmedContent, "{'body': \"")
				trimmedContent = strings.TrimSuffix(trimmedContent, "'}")
				trimmedContent = strings.TrimSuffix(trimmedContent, "\"}")
				listings[i].Description = &trimmedContent
			}
		}
	}

	utils.Log("Merged %d detailed descriptions out of %d listings", mergeCount, len(listings))
	if skipped > 0 {
		utils.Warn("Skipped %d/%d companies whose detail API failed", skipped, totalCompanies)
	}
}

func GradConnectToMPLOY(gradConnect GradConnectData, url string) ([]models.Listing, error) {
	campaigns := gradConnect.Campaigns
	listings := []models.Listing{}

	if len(campaigns) > 0 {
		for _, campaign := range campaigns {

			locations, err := GradConLocationsToMPLOYLocationTypes(campaign)
			if err != nil {
				utils.Warn("error getting locations: %v", err)
			}

			studyFields, err := GradConStudyFieldsToMPLOYStudyFields(campaign)
			if err != nil {
				utils.Warn("error getting study fields: %v", err)
			}

			jobType, err := GradConJobTypeToMPLOYJobType(campaign)
			if err != nil {
				utils.Warn("error getting job type: %v", err)
			}

			closeDate := GetGradConCloseDate(campaign)
			openDate := GetGradConOpenDate(campaign)

			workingRights, err := GradConWorkRightsToMPLOYWorkingRights(campaign)
			if err != nil {
				utils.Warn("error getting working rights: %v", err)
			}

			employerUrl := url + "/employers/" + gradConnect.CustomerOrganisation.Slug
			jobUrl := employerUrl + "/jobs/" + campaign.Slug
			logo := gradConnect.CustomerOrganisation.Logo

			// Fall back to SourceURL if ApplicationURL is empty
			applicationURL := campaign.OriginTargetURL
			if applicationURL == "" {
				applicationURL = jobUrl
			}

			listing := models.Listing{
				Title:       campaign.Title,
				Description: &campaign.Description,
				Company: models.Company{
					Name:    gradConnect.CustomerOrganisation.Name,
					Website: &employerUrl,
					Logo:    &logo,
				},
				ApplicationURL: &applicationURL,
				SourceURLs:     []string{jobUrl},
				Type:           &jobType,
				OpenDate:       openDate,
				CloseDate:      closeDate,
				Locations:      locations,
				StudyFields:    studyFields,
				WorkingRights:  workingRights,
				Source:         models.GRADCONNECTION,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				Days_lived:     0,
			}

			listings = append(listings, listing)
		}
	}

	return listings, nil
}

func GradConLocationsToMPLOYLocationTypes(campaign GradConnectCampaign) ([]models.LocationType, error) {
	// Related Ticket (https://www.notion.so/monashcoding/GradConnect-Scraper-Standardise-JobType-Location-and-Work-Right-Strings-15f1e0829dd88047b4f3d98aba2e309b)

	// The city-to-state table used to live here. It is shared now, so the
	// places GradConnection taught us are known to the other scrapers too --
	// Prosple was filing "Sydney" under OTHERS for want of exactly this list.
	locations := []models.LocationType{}
	seen := map[models.LocationType]bool{}

	for _, label := range campaign.Locations {
		location, known := utils.AustralianLocationFor(label)
		if !known {
			// Record the miss so the shared table can be grown from real scrape
			// output instead of guesswork.
			utils.RecordUnmappedLocation("GradConnection", label)
			location = utils.OverseasLocationFor(label)
		}

		if !seen[location] {
			locations = append(locations, location)
			seen[location] = true
		}
	}

	// A nationwide marker alongside a specific state says nothing the state
	// does not.
	if len(locations) > 1 && utils.HasAustralianState(locations) {
		trimmed := locations[:0]
		for _, location := range locations {
			if location != models.AUSTRALIA {
				trimmed = append(trimmed, location)
			}
		}
		locations = trimmed
	}

	if len(locations) == 0 {
		// A campaign with no locations at all tells us nothing about where the
		// job is, so mark it unknown rather than claiming it is Australian.
		locations = append(locations, models.OTHERS)
	}

	return utils.SanitiseAustralianLocations(locations, campaign.Title, campaign.Locations), nil
}

func GradConStudyFieldsToMPLOYStudyFields(campaign GradConnectCampaign) ([]string, error) {
	studyFields := []string{}
	studyFields = append(studyFields, campaign.Disciplines...)
	return studyFields, nil
}

func GetGradConCloseDate(campaign GradConnectCampaign) *time.Time {
	endDate := campaign.Interval.End

	parsedEndDate, err := time.Parse(time.RFC3339, endDate)

	if err != nil {
		//utils.Warn("error parsing close date: %v, defaulting to 1970-01-01T00:00:00Z", err)

		return nil
	} else {
		return &parsedEndDate
	}
}

func GetGradConOpenDate(campaign GradConnectCampaign) *time.Time {
	openDate := campaign.Interval.Start

	parsedOpenDate, err := time.Parse(time.RFC3339, openDate)

	if err != nil {
		return nil
	} else {
		return &parsedOpenDate
	}
}

func GradConWorkRightsToMPLOYWorkingRights(campaign GradConnectCampaign) ([]models.WorkingRight, error) {
	// Extend this to match working right type names to our Enum
	// Related Ticket (https://www.notion.so/monashcoding/GradConnect-Scraper-Standardise-JobType-Location-and-Work-Right-Strings-15f1e0829dd88047b4f3d98aba2e309b)

	// Define the mapping between GradConnect work rights and MPLOY work rights
	workRightMapping := map[string]models.WorkingRight{
		"Australian Permanent Resident":           models.AUS_CITIZEN_PR,
		"Australian Citizen":                      models.AUS_CITIZEN_PR,
		"Australian Working Visa":                 models.INTERNATIONAL,
		"Australian Bridging Visa":                models.INTERNATIONAL,
		"Australian Student Visa":                 models.INTERNATIONAL,
		"Australian Skilled Migration Visa (485)": models.INTERNATIONAL,
		"New Zealand Citizen":                     models.NZ_CITIZEN_PR,
	}

	// Initialize the result array
	workingRights := []models.WorkingRight{}

	// Map each work right from the campaign
	for _, workRight := range campaign.WorkRights {
		// Check if the work right exists in the mapping
		if mappedWorkRight, exists := workRightMapping[workRight]; exists {
			workingRights = append(workingRights, mappedWorkRight)
		} else {
			// Map unknown work rights to OTHER_RIGHTS
			workingRights = append(workingRights, models.OTHER_RIGHTS)
		}
	}

	// Remove duplicates from workingRights
	uniqueWorkingRights := []models.WorkingRight{}
	seenRights := map[models.WorkingRight]struct{}{}

	for _, right := range workingRights {
		if _, seen := seenRights[right]; !seen {
			uniqueWorkingRights = append(uniqueWorkingRights, right)
			seenRights[right] = struct{}{}
		}
	}

	workingRights = uniqueWorkingRights

	return workingRights, nil
}

func GradConJobTypeToMPLOYJobType(campaign GradConnectCampaign) (models.JobType, error) {
	// Standardise the Gradconnection Job type to our Job type enums
	// Related Ticket (https://www.notion.so/monashcoding/GradConnect-Scraper-Standardise-JobType-Location-and-Work-Right-Strings-15f1e0829dd88047b4f3d98aba2e309b)
	// Define the mapping between GradConnect job types and MPLOY job types
	jobTypeMapping := map[string]models.JobType{
		"Graduate Jobs":          models.GRADUATE,
		"Internships":            models.INTERN,
		"Entry Level Jobs":       models.GRADUATE,
		"Events":                 models.OTHER,
		"Scholarships":           models.OTHER,
		"Casual Jobs":            models.OTHER,
		"Cadetships":             models.INTERN,
		"Alumni Jobs":            models.OTHER,
		"Clerkships":             models.INTERN,
		"Part Time Student Jobs": models.OTHER,
	}

	if jobType, ok := campaign.JobType.(string); ok {
		// Map the string to the appropriate MPLOY job type
		if mappedJobType, exists := jobTypeMapping[jobType]; exists {
			return mappedJobType, nil
		}
		// Default to OTHER for unknown job types
		return models.OTHER, nil
	}

	if jobTypeMap, ok := campaign.JobType.(map[string]interface{}); ok {
		// Check if both 'id' and 'name' exist and are strings
		name, nameOk := jobTypeMap["name"].(string)
		_, idOk := jobTypeMap["id"].(string)

		if !nameOk || !idOk {
			return models.OTHER, fmt.Errorf("job_type map missing required string fields id and/or name")
		}

		// Map the "name" field to the appropriate MPLOY job type
		if mappedJobType, exists := jobTypeMapping[name]; exists {
			return mappedJobType, nil
		}
		// Default to OTHER for unknown job types
		return models.OTHER, nil
	}

	// Return error if JobType is not valid
	return models.JobType(""), fmt.Errorf("job type is not a valid string or object: %v", campaign.JobType)
}
