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
	// Match location type names to our Enum
	// Related Ticket (https://www.notion.so/monashcoding/GradConnect-Scraper-Standardise-JobType-Location-and-Work-Right-Strings-15f1e0829dd88047b4f3d98aba2e309b)
	
	// Define a mapping where each state includes its corresponding cities
	stateAndCityMapping := map[models.LocationType][]string{
		models.VIC: {"Victoria", "Melbourne", "Geelong", "Ballarat", "Bendigo", "Docklands", "Clayton", "Melbourne - Bayside and South Eastern Suburbs", "Melbourne - Eastern Suburbs", "Melbourne - Northern Suburbs", "Melbourne - Western Suburbs", "Melbourne - CBD and Inner Suburbs", "Swan Hill", "Shepparton", "Traralgon", "Horsham", "Wangaratta", "Mildura", "Colac", "Pakenham", "Warrnambool", "Sale", "Echuca", "Regional Victoria", "Port Melbourne", "Dandenong", "Richmond", "Warragul"},
		models.NSW: {"New South Wales", "Sydney", "Newcastle", "Wollongong", "Albury", "Armidale", "Austinmer", "Botany", "Dubbo", "Tamworth", "Orange", "Moree", "Hunter Valley", "Williamtown", "Grafton", "Nowra", "Penrith", "Parramatta", "Rhodes", "Chatswood", "Central Coast", "North Sydney", "Garden Island (NSW)", "Wagga Wagga", "Mulwala", "Rydalmere", "Gosford", "Regional New South Wales", "Parkes", "Goulburn", "Coffs Harbour", "Paramatta"},
		models.QLD: {"Queensland", "Brisbane", "Gold Coast", "Cairns", "Townsville", "Maroochydore", "Mount Isa", "Mackay", "Rockhampton", "Emerald", "Dalby", "Toowoomba", "South Brisbane", "Roma", "Gladstone", "Central Queensland", "Sunshine Coast", "Weipa", "Eagle Farm", "Regional Queensland"},
		models.WA:  {"Western Australia", "Perth", "Fremantle", "Albany", "Broome", "Kalgoorlie", "Geraldton", "Esperance", "Kwinana", "Bunbury", "Rockingham", "Cannington", "Narrogin", "Moora", "Regional Western Australia"},
		models.NT:  {"Northern Territory", "Darwin", "Alice Springs", "Regional Northern Territory", "NT"},
		models.SA:  {"South Australia", "Adelaide", "Whyalla", "Port Pirie", "Port Lincoln", "Gawler", "Regional South Australia"},
		models.ACT: {"Australian Capital Territory", "Canberra", "Regional ACT", "ACT"},
		models.TAS: {"Tasmania", "Hobart", "Launceston", "Regional Tasmania", "TAS"},
		models.AUSTRALIA: {"Australia", "Australia Wide", "Melbourne - CBD and Inner Suburbs, AU"},
	}

	// Create a reverse lookup map for fast city-to-state mapping
	cityToStateMapping := make(map[string]models.LocationType)
	for state, cities := range stateAndCityMapping {
		for _, city := range cities {
			cityToStateMapping[city] = state
		}
	}

	// Initialize the result slice
	locations := []models.LocationType{}
	visitedStates := make(map[models.LocationType]bool)

	// Loop through the campaign locations and map them to LocationType
	for _, location := range campaign.Locations {
		if state, exists := cityToStateMapping[location]; exists {
			// Add the state only if it hasn't been added before
			if !visitedStates[state] {
				locations = append(locations, state)
				visitedStates[state] = true
			}
		} else {
			locations = append(locations, models.OTHERS)
		}
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
		//utils.Warn("error parsing close date: %v, defaulting to 1970-01-01T00:00:00Z", err)

		return defaultDate
	} else {
		return parsedEndDate
	}
}

func GradConWorkRightsToMPLOYWorkingRights(campaign GradConnectCampaign) ([]models.WorkingRight, error) {
	// TODO:: Extend this to match working right type names to our Enum
	// Related Ticket (https://www.notion.so/monashcoding/GradConnect-Scraper-Standardise-JobType-Location-and-Work-Right-Strings-15f1e0829dd88047b4f3d98aba2e309b)
	
	// Define the mapping between GradConnect work rights and MPLOY work rights
	workRightMapping := map[string]models.WorkingRight{
		"Australian Permanent Resident":         models.AUS_PR,
		"Australian Citizen":                    models.AUS_CITIZEN,
		"Australian Working Visa":               models.WORK_VISA,
		"Australian Bridging Visa":              models.WORK_VISA,
		"Australian Student Visa":               models.INTERNATIONAL,
		"Australian Skilled Migration Visa (485)": models.WORK_VISA,
		"New Zealand Citizen":                   models.NZ_CITIZEN,
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

	return workingRights, nil
}

func GradConJobTypeToMPLOYJobType(campaign GradConnectCampaign) (models.JobType, error) {
	// Standardise the Gradconnection Job type to our Job type enums
	// Related Ticket (https://www.notion.so/monashcoding/GradConnect-Scraper-Standardise-JobType-Location-and-Work-Right-Strings-15f1e0829dd88047b4f3d98aba2e309b)	
	// Define the mapping between GradConnect job types and MPLOY job types
	jobTypeMapping := map[string]models.JobType{
		"Graduate Jobs":         models.GRADUATE,
		"Internships":           models.INTERN,
		"Entry Level Jobs":      models.GRADUATE,
		"Events":                models.OTHER,
		"Scholarships":          models.OTHER,
		"Casual Jobs":           models.OTHER,
		"Cadetships":            models.INTERN,
		"Alumni Jobs":           models.OTHER,
		"Clerkships":            models.INTERN,
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