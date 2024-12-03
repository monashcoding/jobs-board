package sites

import (
	"context"
	"fmt"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites"
)

// TestHelloName calls greetings.Hello with a name, checking
// for a valid return value.
func TestHelloName(t *testing.T) {
		prosple := sites.NewProspleScraper()
		jobListing, err := prosple.Scrape(context.Background())

		// Assert no error
		if err != nil {
			t.Fatalf("Error: %v", err)
			
			// Assert jobListing is not nil
			if jobListing == nil {
				t.Fatalf("Expected jobListing to be of type []models.Listing, but got nil")
			}
		}
		
		fmt.Printf("Job listings: %v\n", jobListing)
}