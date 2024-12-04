package sites

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites"
)

// TestHelloName calls greetings.Hello with a name, checking
// for a valid return value.
func TestHelloName(t *testing.T) {
	prosple := sites.NewProspleScraper()
	jobListing, err := prosple.Scrape(context.Background())

	// Write to a json file
	file, err := os.Create("prosple.json")
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(jobListing[0]); err != nil {
		t.Fatalf("Failed to encode job listing to JSON: %v", err)
	}
}
