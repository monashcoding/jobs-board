package prosple

import (
	"testing"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/prosple"
	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// JobTypeConversionTestSuite provides common functionality for testing job type conversion
type JobTypeConversionTestSuite struct {
	T *testing.T
}

// NewJobTypeConversionTestSuite creates a new test suite for job type conversion
func NewJobTypeConversionTestSuite(t *testing.T) *JobTypeConversionTestSuite {
	return &JobTypeConversionTestSuite{
		T: t,
	}
}

// RunTestCase executes a single test case for job type conversion
func (s *JobTypeConversionTestSuite) RunTestCase(jobType string, expectedJobType models.JobType) {
	s.T.Helper()
	s.T.Run("Test Job Type Conversion", func(t *testing.T) {
		// Call the function to convert the job type
		mappedJobType, err := prosple.ProspleJobTypeToMPLOYJobType(jobType)
		if err != nil {
			t.Fatalf("Error converting job type: %v", err)
		}

		// Check if the mapped job type is correct
		if mappedJobType != expectedJobType {
			t.Errorf("Expected job type %v, but got %v", expectedJobType, mappedJobType)
		}
	})
}

// TestJobTypeConversion runs the test suite for job type conversion
func TestJobTypeConversion(t *testing.T) {
	// Create a new test suite
	testSuite := NewJobTypeConversionTestSuite(t)

	// Define test cases with input job type and expected job type
	testCases := []struct {
		jobType        string
		expectedJobType models.JobType
	}{
		{
			jobType:        "Internship, Clerkship or Placement",
			expectedJobType: models.INTERN,
		},
	}

	// Run each test case
	for _, tc := range testCases {
		testSuite.RunTestCase(tc.jobType, tc.expectedJobType)
	}
}
