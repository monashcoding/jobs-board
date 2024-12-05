package testing

import (
	"context"
	"encoding/json"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline"
	"os"
	"path/filepath"
	"testing"
)

// TestCase represents a single test scenario for a pipeline step
type TestCase struct {
	Name        string
	InputFile   string // JSON file containing input listings
	OutputFile  string // JSON file containing expected output listings
	ExpectError bool
}

// PipelineStepTestSuite provides common testing functionality for pipeline steps
type PipelineStepTestSuite struct {
	T        *testing.T
	Step     pipeline.Step
	TestData string // Base path for test data
}

// NewPipelineStepTestSuite creates a new test suite for a pipeline step
func NewPipelineStepTestSuite(t *testing.T, step pipeline.Step, testDataPath string) *PipelineStepTestSuite {
	return &PipelineStepTestSuite{
		T:        t,
		Step:     step,
		TestData: testDataPath,
	}
}

// loadListings loads listings from a JSON file
func (s *PipelineStepTestSuite) loadListings(filename string) []models.Listing {
	path := filepath.Join(s.TestData, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		s.T.Fatalf("Failed to read listings file %s: %v", filename, err)
	}

	var listings []models.Listing
	if err := json.Unmarshal(data, &listings); err != nil {
		s.T.Fatalf("Failed to unmarshal listings from %s: %v", filename, err)
	}

	return listings
}

// compareListings compares two slices of listings
func (s *PipelineStepTestSuite) compareListings(t *testing.T, got, want []models.Listing) {
	t.Helper()

	if len(got) != len(want) {
		t.Errorf("Got %d listings, want %d", len(got), len(want))
		return
	}

	for i := range got {
		gotJSON, err := json.Marshal(got[i])
		if err != nil {
			t.Fatalf("Failed to marshal got listing: %v", err)
		}

		wantJSON, err := json.Marshal(want[i])
		if err != nil {
			t.Fatalf("Failed to marshal want listing: %v", err)
		}

		if string(gotJSON) != string(wantJSON) {
			t.Errorf("Listing %d doesn't match:\ngot:  %s\nwant: %s", i, gotJSON, wantJSON)
		}
	}
}

// RunTestCase executes a single test case
func (s *PipelineStepTestSuite) RunTestCase(tc TestCase) {
	s.T.Helper()
	s.T.Run(tc.Name, func(t *testing.T) {
		// Load input listings
		input := s.loadListings(tc.InputFile)

		// Run the step
		got, err := s.Step.Process(context.Background(), input)

		// Check error expectation
		if (err != nil) != tc.ExpectError {
			t.Errorf("Process() error = %v, expectError %v", err, tc.ExpectError)
			return
		}

		if !tc.ExpectError {
			// Load and compare expected output
			want := s.loadListings(tc.OutputFile)
			s.compareListings(t, got, want)
		}
	})
}
