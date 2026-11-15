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
	jobLimit  int
}

func NewProspleScraper() *ProspleScraper {
	config := unblocker.Config{
		Features: unblocker.Features{
			UseRateLimiting: false,
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

func (s *ProspleScraper) SetJobLimit(limit int) {
	s.jobLimit = limit
}

func (s *ProspleScraper) PerformGraphQLScrape(offset int, pageSize int, field string) ([]models.Listing, error) {
	prospleApiUrl := "https://prosple-gw.global.ssl.fastly.net/internal"
	var listings []models.Listing

	variables := GetGraphQlVariables(offset, pageSize, field)

	response, err := SendGraphQLQuery(prospleApiUrl, variables, s.unblocker)

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
	var pageSize = 50

	// Fields to scrape
	fields := []string{"502", "532", "677"}

	// hasLimit determines if we should keep scraping until we reach a specific limit
	hasLimit := s.jobLimit > 0

	// Scrape each field
	for _, field := range fields {
		for i := 0; ; i++ {
			if hasLimit && len(listings) >= s.jobLimit {
				utils.Log("Reached job limit (%d), stopping scrape", s.jobLimit)
				break
			}

			utils.Log("Scraping Prosple: Field %s, Page %d...", field, i+1)

			newListings, err := s.PerformGraphQLScrape(i*pageSize, pageSize, field)
			if err != nil {
				return nil, fmt.Errorf("failed to scrape Prosple: %v", err)
			}

			// If the count of new listings is less than the page size, we've reached the end for this field
			listings = append(listings, newListings...)
			if len(newListings) < pageSize {
				break
			}
		}
	}

	if hasLimit && len(listings) > s.jobLimit {
		listings = listings[:s.jobLimit]
		utils.Log("Trimmed listings to match job limit exactly (%d)", s.jobLimit)
	}

	return listings, nil
}
