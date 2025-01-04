package prosple

import (
	"testing"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/prosple"
	"github.com/monashcoding/mploy-golang-core/internal/models"
)

func convertJobType(jobType string) (models.JobType, error) {
	// Call the ProspleJobTypeToMPLOYJobType function from converter.go
	mappedJobType, err := prosple.ProspleJobTypeToMPLOYJobType(jobType)
	if err != nil {
		return "", err
	}

	// Return the mapped job type
	return mappedJobType, nil
}

func TestJobTypeConversion(t *testing.T) {
	// Define the input job type
	jobType := "Internship, Clerkship or Placement"

	// Call the function to convert the job type
	mappedJobType, err := convertJobType(jobType)
	if err != nil {
		t.Fatalf("Error converting job type: %v", err)
	}

	// Check if the mapped job type is correct (INTERN)
	expectedJobType := models.INTERN
	if mappedJobType != expectedJobType {
		t.Errorf("Expected job type %v, but got %v", expectedJobType, mappedJobType)
	}
}
