package prosple

import (
	"fmt"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"time"
)

func OpportunityToModel(baseURL string, opportunity Opportunity) (*models.Listing, error) {
	sourceUrl := baseURL + opportunity.DetailPageURl

	company := models.Company{
		Name:    opportunity.ParentEmployer.Title,
		Website: baseURL + opportunity.ParentEmployer.DetailPageUrl,
	}

	jobType := models.JobType(opportunity.OpportunityTypes[0].Label)

	parsedCloseDate, err := time.Parse("2006-01-02T15:04:05.000Z", opportunity.ApplicationsCloseDate)
	if err != nil {
		fmt.Printf("warn: error parsing close date: %v, defaulting to 1970-01-01T00:00:00Z\n", err)
		parsedCloseDate = time.Unix(0, 0).UTC() // Default to 1970-01-01T00:00:00Z
	}

	locations, err := ProspleLocationsToLocationTypes(opportunity)
	if err != nil {
		return nil, fmt.Errorf("error getting locations: %v", err)
	}

	studyfield, err := ProspleStudyFieldsToStudyFields(opportunity)
	if err != nil {
		return nil, fmt.Errorf("error getting study fields: %v", err)
	}

	var parsedStartDate time.Time
	if opportunity.StartDate.Category != nil {
		parsedStartDate, err = time.Parse("2006-01-02T15:04:05.000Z", opportunity.StartDate.Category.Label)
		if err != nil {
			fmt.Printf("warn: error parsing start date: %v, defaulting to 1970-01-01T00:00:00Z\n", err)
			parsedStartDate = time.Unix(0, 0).UTC() // Default to 1970-01-01T00:00:00Z
		}
	} else {
		parsedStartDate = time.Now()
	}

	workingRights, err := ProspleWorkingRightsToWorkingRights(opportunity)
	if err != nil {
		return nil, fmt.Errorf("error getting working rights: %v", err)
	}

	return &models.Listing{
		Title:          opportunity.Title,
		Description:    opportunity.Overview.FullText,
		Company:        company,
		ApplicationURL: opportunity.ApplyByUrl,
		SourceURLs:     []string{sourceUrl},
		Type:           jobType,
		CloseDate:      &parsedCloseDate,
		Locations:      locations,
		StudyFields:    studyfield,
		StartDate:      parsedStartDate,
		WorkingRights:  workingRights,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}, nil
}

func ProspleLocationsToLocationTypes(opportunity Opportunity) ([]models.LocationType, error) {
	locations := []models.LocationType{}
	for _, location := range opportunity.PhysicalLocations {
		locations = append(locations, models.LocationType(location.Label))

		for _, childLocation := range location.Children {
			locations = append(locations, models.LocationType(childLocation.Label))
		}
	}
	return locations, nil
}

func ProspleStudyFieldsToStudyFields(opportunity Opportunity) ([]models.StudyField, error) {
	studyFields := []models.StudyField{}
	for _, field := range opportunity.StudyFields {
		studyFields = append(studyFields, models.StudyField(field.Label))
	}
	return studyFields, nil
}

func ProspleWorkingRightsToWorkingRights(opportunity Opportunity) ([]models.WorkingRight, error) {
	workingRights := []models.WorkingRight{}
	for _, workingRight := range opportunity.WorkingRights {
		workingRights = append(workingRights, models.WorkingRight(workingRight.Label))

		for _, child := range workingRight.Children {
			workingRights = append(workingRights, models.WorkingRight(child.Label))
		}
	}
	return workingRights, nil
}
