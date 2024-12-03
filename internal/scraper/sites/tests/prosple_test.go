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
		fmt.Println("Starting")

		prosple := sites.NewProspleScraper()
		prosple.Scrape(context.Background())

		fmt.Println("Test Ends")
}