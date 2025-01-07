package gradcon

import (
	"fmt"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

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

			workingRights, err := GradConWorkRightsToMPLOYWorkingRights(campaign)
			if err != nil {
				utils.Warn("error getting working rights: %v", err)
			}

			employerUrl := url + "/employers/" + gradConnect.CustomerOrganisation.Slug
			jobUrl := employerUrl + "/jobs/" + campaign.Slug

			listing := models.Listing{
				Title:       campaign.Title,
				Description: &campaign.Description,
				Company: models.Company{
					Name:    gradConnect.CustomerOrganisation.Name,
					Website: &employerUrl,
				},
				ApplicationURL: &campaign.OriginTargetURL,
				SourceURLs:     []string{jobUrl},
				Type:           &jobType,
				CloseDate:      &closeDate,
				Locations:      locations,
				StudyFields:    studyFields,
				WorkingRights:  workingRights,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			listings = append(listings, listing)
		}
	}

	return listings, nil
}

func GradConLocationsToMPLOYLocationTypes(campaign GradConnectCampaign) ([]models.LocationType, error) {
	// TODO:: Extend this to match location type names to our Enum
	// Related Ticket (https://www.notion.so/monashcoding/GradConnect-Scraper-Standardise-JobType-Location-and-Work-Right-Strings-15f1e0829dd88047b4f3d98aba2e309b)

	locations := []models.LocationType{}
	for _, location := range campaign.Locations {
		locations = append(locations, models.LocationType(location))
	}

	return locations, nil
}

func GradConStudyFieldsToMPLOYStudyFields(campaign GradConnectCampaign) ([]string, error) {
	studyFields := []string{}
	studyFields = append(studyFields, campaign.Disciplines...)
	return studyFields, nil
}

func GetGradConCloseDate(campaign GradConnectCampaign) time.Time {
	defaultDate := time.Unix(0, 0).UTC() // Default to 1970-01-01T00:00:00Z
	endDate := campaign.Interval.End

	parsedEndDate, err := time.Parse(time.RFC3339, endDate)

	if err != nil {
		utils.Warn("error parsing close date: %v, defaulting to 1970-01-01T00:00:00Z", err)

		return defaultDate
	} else {
		return parsedEndDate
	}
}

func GradConWorkRightsToMPLOYWorkingRights(campaign GradConnectCampaign) ([]models.WorkingRight, error) {
	// TODO:: Extend this to match working right type names to our Enum
	// Related Ticket (https://www.notion.so/monashcoding/GradConnect-Scraper-Standardise-JobType-Location-and-Work-Right-Strings-15f1e0829dd88047b4f3d98aba2e309b)

	workingRights := []models.WorkingRight{}
	for _, workRight := range campaign.WorkRights {
		workingRights = append(workingRights, models.WorkingRight(workRight))
	}
	return workingRights, nil
}

func GradConJobTypeToMPLOYJobType(campaign GradConnectCampaign) (models.JobType, error) {
	// TODO:: Extend this to match working right type names to our Enum
	// Related Ticket (https://www.notion.so/monashcoding/GradConnect-Scraper-Standardise-JobType-Location-and-Work-Right-Strings-15f1e0829dd88047b4f3d98aba2e309b)
	
	if jobType, ok := campaign.JobType.(string); ok {
    // Use jobType as string
		return models.JobType(jobType), nil
	}

	if jobTypeMap, ok := campaign.JobType.(map[string]interface{}); ok {
    // Check if both 'id' and 'name' exist and are strings
    name, nameOk := jobTypeMap["name"].(string)
    _, idOk := jobTypeMap["id"].(string)
    
    if !nameOk || !idOk {
        return "", fmt.Errorf("job_type map missing required string fields id and/or name")
    }
    
    // Now safe to use name and id
    return models.JobType(name), nil
}

	return models.JobType(""), fmt.Errorf("job type is not a valid string or object: %v", campaign.JobType)
}
