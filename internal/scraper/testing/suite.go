package testing

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gocolly/colly/v2"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/scraper"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestCase represents a single test scenario for a scraper
type TestCase struct {
	Name           string
	FixtureFolder  string
	ExpectedFile   string // JSON file containing expected listings
	ExpectError    bool
	FixtureMapping map[string]string // Maps URLs to fixture files
}

// ScraperTestSuite provides common testing functionality for scrapers
type ScraperTestSuite struct {
	T        *testing.T
	Scraper  scraper.Scraper
	TestData string // Base path for test data
}

// mockTransport implements http.RoundTripper for serving local test files
type mockTransport struct {
	t              *testing.T
	fixtureMapping map[string]string
	basePath       string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Get the fixture file for this URL
	fixtureFile, exists := m.fixtureMapping[req.URL.String()]
	if !exists {
		m.t.Fatalf("No fixture mapping found for URL: %s", req.URL.String())
	}

	// Load the fixture
	filename := filepath.Join(m.basePath, fixtureFile)
	content, err := os.ReadFile(filename)
	if err != nil {
		m.t.Fatalf("Failed to read test fixture %s: %v", filename, err)
	}

	// Create mock response
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(content)),
		Header:     make(http.Header),
	}, nil
}

// NewScraperTestSuite creates a new test suite for a scraper
func NewScraperTestSuite(t *testing.T, scraper scraper.Scraper, testDataPath string) *ScraperTestSuite {
	return &ScraperTestSuite{
		T:        t,
		Scraper:  scraper,
		TestData: testDataPath,
	}
}

// RunTestCase executes a single test case
func (s *ScraperTestSuite) RunTestCase(tc TestCase) {
	s.T.Helper()
	s.T.Run(tc.Name, func(t *testing.T) {
		// Create a new collector with mocked transport
		mockCollector := s.createMockCollector(tc.FixtureMapping, tc.FixtureFolder)

		// Replace the scraper's collector with our mock
		if baseScraper, ok := s.Scraper.(interface{ SetCollector(*colly.Collector) }); ok {
			baseScraper.SetCollector(mockCollector)
		} else {
			t.Fatal("Scraper doesn't implement SetCollector method")
		}

		// Run the scraper
		got, err := s.Scraper.Scrape(context.Background())

		// Check error expectation
		if (err != nil) != tc.ExpectError {
			t.Errorf("Scrape() error = %v, expectError %v", err, tc.ExpectError)
			return
		}

		if !tc.ExpectError {
			// Load expected results
			want := s.loadExpectedListings(tc.ExpectedFile)
			s.compareResults(t, got, want)
		}
	})
}

// createMockCollector creates a collector that uses local test files
func (s *ScraperTestSuite) createMockCollector(fixtureMapping map[string]string, fixturePath string) *colly.Collector {
	c := colly.NewCollector()
	c.WithTransport(&mockTransport{
		t:              s.T,
		fixtureMapping: fixtureMapping,
		basePath:       filepath.Join(s.TestData, fixturePath),
	})
	return c
}

// loadExpectedListings loads expected listings from a JSON file
func (s *ScraperTestSuite) loadExpectedListings(filename string) []models.Listing {
	path := filepath.Join(s.TestData, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		s.T.Fatalf("Failed to read expected listings file: %v", err)
	}

	var listings []models.Listing
	if err := json.Unmarshal(data, &listings); err != nil {
		s.T.Fatalf("Failed to unmarshal expected listings: %v", err)
	}

	return listings
}

// compareResults compares scraped listings with expected listings
func (s *ScraperTestSuite) compareResults(t *testing.T, got, want []models.Listing) {
	t.Helper()

	if len(got) != len(want) {
		t.Errorf("Got %d listings, want %d", len(got), len(want))
		return
	}

	for i := range got {
		s.compareListings(t, got[i], want[i])
	}
}

// compareListings compares two listings for equality
func (s *ScraperTestSuite) compareListings(t *testing.T, got, want models.Listing) {
	t.Helper()

	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Failed to marshal got listing: %v", err)
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal want listing: %v", err)
	}

	if string(gotJSON) != string(wantJSON) {
		t.Errorf("Listings don't match:\ngot:  %s\nwant: %s", gotJSON, wantJSON)
	}
}
