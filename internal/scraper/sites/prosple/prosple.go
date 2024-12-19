package prosple

import (
	"context"
	"fmt"
	"github.com/monashcoding/mploy-golang-core/internal/unblocker"
	"os"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type ProspleScraper struct {
	unblocker unblocker.Service
}

func NewProspleScraper() *ProspleScraper {
	// Configure unblocker
	config := unblocker.Config{
		Features: unblocker.Features{
			UseRateLimiting:    true,
			UseIPRotation:      true,
			UseBrowserProfiles: true,
		},
		BrightData: &unblocker.BrightDataConfig{
			CustomerID:  os.Getenv("BRIGHTDATA_CUSTOMER_ID"),
			ZoneName:    os.Getenv("BRIGHTDATA_ZONE_NAME"),
			Password:    os.Getenv("BRIGHTDATA_PASSWORD"),
			Country:     "au",
			UseLocalDNS: true,
		},
		Rate: &unblocker.RateLimitConfig{
			RequestsPerMinute: 30,
			BaseDelay:         time.Second * 2,
			MaxJitter:         time.Second,
		},
	}

	unblockerService, err := unblocker.NewService(config)
	if err != nil {
		utils.Error("Failed to initialize unblocker: %v", err)
		return nil
	}

	return &ProspleScraper{
		unblocker: unblockerService,
	}
}

func (s *ProspleScraper) Name() string {
	return "Prosple"
}

func (s *ProspleScraper) URL() string {
	return "https://au.prosple.com"
}

func (s *ProspleScraper) API_URL() string {
	return "https://prosple-gw.global.ssl.fastly.net/internal"
}

func (s *ProspleScraper) PerformGraphQLScrape(offset int, pageSize int) ([]models.Listing, error) {
	prospleApiUrl := s.API_URL()
	var listings []models.Listing

	query := GetGraphQlQuery()
	variables := GetGraphQlVariables(offset, pageSize)

	response, err := SendGraphQLQuery(prospleApiUrl, query, variables, s.unblocker)

	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %v", err)
	}

	for _, opp := range response.Data.OpportunitiesSearch.Opportunities {
		listing, err := OpportunityToModel(s.URL(), opp)
		if err != nil {
			fmt.Printf("Error converting opportunity to model: %v\n", err)
			continue
		}
		listings = append(listings, *listing)
	}

	return listings, nil
}

func (s *ProspleScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	var listings []models.Listing
	pages := 2     // Number of pages to scrape
	pageSize := 50 // Number of listings per page

	// Scrape all listings
	scrapeAll := false
	if scrapeAll {
		// Scrape until pages are exhausted (i.e. the size of the returned array is not the same as the page size)
		for i := 0; ; i++ {
			utils.Log("Fully Scraping Prosple: Page %d", i+1)

			newListings, err := s.PerformGraphQLScrape(i*pageSize, pageSize)

			if err != nil {
				return nil, fmt.Errorf("failed to scrape Prosple: %v", err)
			}

			if len(newListings) != pageSize {
				// End of results
				listings = append(listings, newListings...)
				break
			}

			listings = append(listings, newListings...)
		}

		return listings, nil
	}

	// Scrape a fixed number of pages
	for i := 0; i < pages; i++ {
		utils.Log("Scraping Prosple: Page %d of %d...", i+1, pages)

		newListings, err := s.PerformGraphQLScrape(i*pageSize, pageSize)

		if err != nil {
			return nil, fmt.Errorf("failed to scrape Prosple: %v", err)
		}

		listings = append(listings, newListings...)
	}

	return listings, nil
}
