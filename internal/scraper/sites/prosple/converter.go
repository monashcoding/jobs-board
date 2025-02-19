package prosple

import (
	"fmt"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

func OpportunityToModel(baseURL string, opportunity Opportunity) (*models.Listing, error) {
	sourceUrl := baseURL + opportunity.DetailPageUrl

	companyWebsite := baseURL + opportunity.ParentEmployer.DetailPageUrl

	company := models.Company{
		Name:    opportunity.ParentEmployer.Title,
		Website: &companyWebsite,
		Logo:    &opportunity.ParentEmployer.Logo.Thumbnail.Url,
	}

	jobType, err := ProspleJobTypeToMPLOYJobType(opportunity.OpportunityTypes[0].Label)
	if err != nil {
		utils.Warn("error getting job type: %v", err)
	}

	parsedOpenDate := ParseProspleTimeString(opportunity.ApplicationsOpenDate)
	parsedCloseDate := ParseProspleTimeString(opportunity.ApplicationsCloseDate)

	locations, err := ProspleLocationsToMPLOYLocationTypes(opportunity)
	if err != nil {
		utils.Warn("error getting locations: %v", err)
	}

	studyfield, err := ProspleStudyFieldsToMPLOYStudyFields(opportunity)
	if err != nil {
		utils.Warn("error getting study fields: %v", err)
	}

	workingRights, err := ProspleWorkingRightsToMPLOYWorkingRights(opportunity)
	if err != nil {
		utils.Warn("error getting working rights: %v", err)
	}

	return &models.Listing{
		Title:          opportunity.Title,
		Description:    &opportunity.Overview.FullText,
		Company:        company,
		ApplicationURL: &opportunity.ApplyByUrl,
		SourceURLs:     []string{sourceUrl},
		Type:           &jobType,
		OpenDate:       parsedOpenDate,
		CloseDate:      parsedCloseDate,
		Locations:      locations,
		StudyFields:    studyfield,
		WorkingRights:  workingRights,
		Source:         models.PROSPLE,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Days_lived:     0,
	}, nil
}

func ProspleLocationsToMPLOYLocationTypes(opportunity Opportunity) ([]models.LocationType, error) {
	// Related Ticket (https://www.notion.so/monashcoding/Prosple-Scraper-Standardise-JobType-Names-1591e0829dd8808ba898f8dcc46eaec7)

	// Predefined location types mapping
	locationMap := map[string]models.LocationType{
		"Australia":                    models.AUSTRALIA,
		"Australian Capital Territory": models.ACT,
		"New South Wales":              models.NSW,
		"Northern Territory":           models.NT,
		"Queensland":                   models.QLD,
		"South Australia":              models.SA,
		"Tasmania":                     models.TAS,
		"Victoria":                     models.VIC,
		"Western Australia":            models.WA,
	}

	// Initialize a slice to hold the mapped locations
	locations := []models.LocationType{}
	uniqueLocations := map[models.LocationType]struct{}{} // map to track unique locations

	// Iterate over the physical locations and their children
	for _, location := range opportunity.PhysicalLocations {
		// Check if the location is in the map
		if mappedLocation, exists := locationMap[location.Label]; exists {
			// If not already added, append to locations
			if _, exists := uniqueLocations[mappedLocation]; !exists {
				locations = append(locations, mappedLocation)
				uniqueLocations[mappedLocation] = struct{}{} // Mark as added
			}
		} else {
			// If the location isn't in the map, append "OTHERS" (only once)
			if _, exists := uniqueLocations[models.OTHERS]; !exists {
				locations = append(locations, models.OTHERS)
				uniqueLocations[models.OTHERS] = struct{}{} // Mark as added
			}
		}

		// Handle child locations
		for _, childLocation := range location.Children {
			// Check if the child location is in the map
			if mappedLocation, exists := locationMap[childLocation.Label]; exists {
				// If not already added, append to locations
				if _, exists := uniqueLocations[mappedLocation]; !exists {
					locations = append(locations, mappedLocation)
					uniqueLocations[mappedLocation] = struct{}{} // Mark as added
				}
			} else {
				// If the child location isn't in the map, append "OTHERS" (only once)
				if _, exists := uniqueLocations[models.OTHERS]; !exists {
					locations = append(locations, models.OTHERS)
					uniqueLocations[models.OTHERS] = struct{}{} // Mark as added
				}
			}
		}
	}

	// Look through locations and if there is australia and one of the states, remove australia
	if len(locations) > 1 {
		for i, location := range locations {
			if location == models.AUSTRALIA {
				for _, state := range locations {
					if state != models.AUSTRALIA && state != models.OTHERS {
						locations = append(locations[:i], locations[i+1:]...)
						break
					}
				}
			}
		}
	} else if len(locations) < 1 {
		locations = append(locations, models.AUSTRALIA)
	}

	return locations, nil
}

func ProspleStudyFieldsToMPLOYStudyFields(opportunity Opportunity) ([]string, error) {
	studyFields := []string{}
	for _, field := range opportunity.StudyFields {
		studyFields = append(studyFields, field.Label)
	}
	return studyFields, nil
}

func ProspleWorkingRightsToMPLOYWorkingRights(opportunity Opportunity) ([]models.WorkingRight, error) {
	// Related Ticket (https://www.notion.so/monashcoding/Prosple-Scraper-Standardise-JobType-Names-1591e0829dd8808ba898f8dcc46eaec7)

	// Predefined working rights mapping
	workingRightsMap := map[string]models.WorkingRight{
		"Australian Permanent Resident":       models.AUS_CITIZEN_PR,
		"Australian Citizen":                  models.AUS_CITIZEN_PR,
		"New Zealand Citizen":                 models.NZ_CITIZEN_PR,
		"New Zealand Permanent Resident":      models.NZ_CITIZEN_PR,
		"International Student/Graduate Visa": models.INTERNATIONAL,
		"New Zealand Temporary Work Visa":     models.INTERNATIONAL,
		"New Zealand Student Visa":            models.INTERNATIONAL,
		"Hong Kong Citizen":                   models.INTERNATIONAL,
		"Australian Work Visa (All Other)":    models.INTERNATIONAL,
		"Visa Sponsorship Included":           models.INTERNATIONAL,
		"Hong Kong Permanent Resident":        models.INTERNATIONAL,
	}

	workingRights := []models.WorkingRight{}
	seen := map[models.WorkingRight]struct{}{} // Map to track unique working rights

	// Iterate over the scraped working rights
	for _, workingRight := range opportunity.WorkingRights {
		// Map the working right to our enum value
		if mappedRight, exists := workingRightsMap[workingRight.Label]; exists {
			if _, alreadySeen := seen[mappedRight]; !alreadySeen {
				workingRights = append(workingRights, mappedRight)
				seen[mappedRight] = struct{}{} // Mark as seen
			}
		} else {
			// If not found in the map, default to "OTHER_RIGHTS"
			if _, alreadySeen := seen[models.OTHER_RIGHTS]; !alreadySeen {
				workingRights = append(workingRights, models.OTHER_RIGHTS)
				seen[models.OTHER_RIGHTS] = struct{}{} // Mark as seen
			}
		}

		// Handle child working rights
		for _, child := range workingRight.Children {
			if mappedRight, exists := workingRightsMap[child.Label]; exists {
				if _, alreadySeen := seen[mappedRight]; !alreadySeen {
					workingRights = append(workingRights, mappedRight)
					seen[mappedRight] = struct{}{} // Mark as seen
				}
			} else {
				// If the child isn't found in the map, default to "OTHER_RIGHTS"
				if _, alreadySeen := seen[models.OTHER_RIGHTS]; !alreadySeen {
					workingRights = append(workingRights, models.OTHER_RIGHTS)
					seen[models.OTHER_RIGHTS] = struct{}{} // Mark as seen
				}
			}
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

func ProspleJobTypeToMPLOYJobType(jobtype string) (models.JobType, error) {
	// Standardise the Prosple Job type to our Job type enums
	// Related Ticket (https://www.notion.so/monashcoding/Prosple-Scraper-Standardise-JobType-Names-1591e0829dd8808ba898f8dcc46eaec7)
	// Define a map that maps Prosple job types to MPLOY job types
	jobTypeMap := map[string]models.JobType{
		"Volunteering":                       models.OTHER,
		"Part-Time Student Job":              models.PRE_PENULTIMATE,
		"Graduate Job or Program":            models.GRADUATE,
		"Virtual Experience":                 models.OTHER,
		"Internship, Clerkship or Placement": models.INTERN,
		"Apprenticeship or Traineeship":      models.INTERN,
	}

	// Check if the job type exists in the map
	if mappedJobType, exists := jobTypeMap[jobtype]; exists {
		return mappedJobType, nil
	}

	// If the job type doesn't exist in the map, return an error
	return models.JobType(jobtype), fmt.Errorf("unknown job type: %s", jobtype)
}

func ParseProspleTimeString(timeStr string) *time.Time {
	parsedTime, err := time.Parse("2006-01-02T15:04:05.000Z", timeStr)

	if err != nil {
		//utils.Warn("error parsing close date: %v, defaulting to 1970-01-01T00:00:00Z", err)

		return nil
	} else {
		return &parsedTime
	}
}
