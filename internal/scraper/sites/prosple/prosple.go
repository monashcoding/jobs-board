package prosple

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/unblocker"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type ProspleScraper struct {
	unblocker unblocker.Service
	maxPages  int
	pageSize  int
	scrapeAll bool
}

func NewProspleScraper() *ProspleScraper {
	config := unblocker.Config{
		Features: unblocker.Features{
			UseRateLimiting: true,
			UseIPRotation:   true,
		},
		BrightData: &unblocker.BrightDataConfig{
			CustomerID:  os.Getenv("BRIGHTDATA_CUSTOMER_ID"),
			ZoneName:    os.Getenv("BRIGHTDATA_ZONE_NAME"),
			Password:    os.Getenv("BRIGHTDATA_PASSWORD"),
			Country:     "au",
			UseLocalDNS: true,
		},
		Rate: &unblocker.RateLimitConfig{
			BaseDelay: time.Second * 2,
			MaxJitter: time.Second,
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

func (s *ProspleScraper) SetMaxPages(maxPages int) {
	s.maxPages = maxPages
}

func (s *ProspleScraper) SetPageSize(pageSize int) {
	s.pageSize = pageSize
}

func (s *ProspleScraper) SetScrapeAll(scrapeAll bool) {
	s.scrapeAll = scrapeAll
}

func (s *ProspleScraper) PerformGraphQLScrape(offset int, pageSize int, field string) ([]models.Listing, error) {
	prospleApiUrl := s.API_URL()
	var listings []models.Listing

	query := GetGraphQlQuery()
	variables := GetGraphQlVariables(offset, pageSize, field)

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
	// Fields to scrape
	fields := []string{"502", "532", "677"}

	// Scrape all listings
	if s.scrapeAll {
		// Scrape until pages are exhausted (i.e. the size of the returned array is not the same as the page size)
		for _, field := range fields {
			for i := 0; ; i++ {
				utils.Log("Fully Scraping Prosple: Field %s, Page %d", field, i+1)

				newListings, err := s.PerformGraphQLScrape(i*s.pageSize, s.pageSize, field)

				if err != nil {
					return nil, fmt.Errorf("failed to scrape Prosple: %v", err)
				}

				if len(newListings) != s.pageSize {
					// End of results
					listings = append(listings, newListings...)
					break
				}

				listings = append(listings, newListings...)
			}
		}

		return listings, nil
	}

	// Scrape a fixed number of pages
	for _, field := range fields {
		for i := 0; i < s.maxPages; i++ {
			utils.Log("Scraping Prosple: Field %s, Page %d of %d...", field, i+1, s.maxPages)

			newListings, err := s.PerformGraphQLScrape(i*s.pageSize, s.pageSize, field)

			if err != nil {
				return nil, fmt.Errorf("failed to scrape Prosple: %v", err)
			}

			listings = append(listings, newListings...)
		}
	}

	return listings, nil
}
