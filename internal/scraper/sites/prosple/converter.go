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

	locations := []models.LocationType{}
	seen := map[models.LocationType]struct{}{}
	rawLocations := []string{} // every label we saw, for the overseas check

	add := func(label string) {
		rawLocations = append(rawLocations, label)

		location, known := utils.AustralianLocationFor(label)
		if !known {
			// Reported so the shared table can be grown from real scrape output
			// rather than guesswork.
			utils.RecordUnmappedLocation("Prosple", label)
			location = utils.OverseasLocationFor(label)
		}

		if _, exists := seen[location]; !exists {
			locations = append(locations, location)
			seen[location] = struct{}{}
		}
	}

	for _, location := range opportunity.PhysicalLocations {
		add(location.Label)
		for _, child := range location.Children {
			add(child.Label)
		}
	}

	// A nationwide marker alongside a specific state says nothing the state
	// does not.
	if len(locations) > 1 {
		trimmed := locations[:0]
		for _, location := range locations {
			if location == models.AUSTRALIA && utils.HasAustralianState(locations) {
				continue
			}
			trimmed = append(trimmed, location)
		}
		locations = trimmed
	}

	if len(locations) == 0 {
		// No locations at all tells us nothing about where the job is, so mark
		// it unknown rather than claiming it is Australian. Normalisation gets
		// a second attempt at it from the title.
		locations = append(locations, models.OTHERS)
	}

	return utils.SanitiseAustralianLocations(locations, opportunity.Title, rawLocations), nil
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
		"Australian Student Visa":             models.INTERNATIONAL,
		"Australian Temporary Work Visa":      models.INTERNATIONAL,
		"Australian Bridging Visa":            models.INTERNATIONAL,
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
			// If not found in the map, default to "OTHER_RIGHTS" and report the
			// string so the table above can be grown from real scrape output.
			// The fallback is otherwise invisible, and it is not a harmless
			// one: OTHER_RIGHTS reads on the board as a restriction, so a label
			// we simply do not recognise looks like one the employer imposed.
			utils.RecordUnmappedRight("Prosple", workingRight.Label)
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
				// Same fallback as the parent right above, reported the same way.
				utils.RecordUnmappedRight("Prosple", child.Label)
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
