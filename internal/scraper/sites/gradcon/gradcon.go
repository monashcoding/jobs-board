package gradcon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/unblocker"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type GradconScraper struct {
	unblocker unblocker.Service
	baseURL   string
}

func NewGradconScraper() *GradconScraper {
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
			BaseDelay: time.Second * 1,
			MaxJitter: time.Second * 1,
		},
	}

	unblockerService, err := unblocker.NewService(config)
	if err != nil {
		utils.Error("Failed to initialize unblocker: %v", err)
		return nil
	}

	return &GradconScraper{
		unblocker: unblockerService,
		baseURL:   "https://www.linkedin.com",
	}
}

func (s *GradconScraper) Name() string {
	return "GradConnections"
}

func (s *GradconScraper) URL() string {
	return "https://au.gradconnection.com"
}

func (s *GradconScraper) API_URL() string {
	return "https://au.gradconnection.com/api/campaignsearch"
}

func (s *GradconScraper) PerformAPIScrape(offset int, pageSize int) ([]models.Listing, error) {
	gradConnectApiUrl := s.API_URL()
	var listings []models.Listing

	result, err := SendAPIRequest(gradConnectApiUrl, pageSize, offset, s.unblocker)
	if err != nil {
		return nil, err
	}

	for _, item := range *result {
		gradConnectListings, err := GradConnectToMPLOY(item, s.URL())

		if err != nil {
			utils.Warn("Error converting opportunity to model: %v", err)
			continue
		}
		listings = append(listings, gradConnectListings...)
	}

	return listings, nil
}

func (s *GradconScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	pages := 30    // Number of pages to scrape
	pageSize := 50 // Number of listings per page

	listings := []models.Listing{}

	// Scrape all listings
	scrapeAll := true
	if scrapeAll {
		// Scrape until pages are exhausted (i.e. the size of the returned array is not the same as the page size)
		for i := 0; ; i++ {
			utils.Log("Fully Scraping GradConnect: Page %d", i+1)

			newListings, err := s.PerformAPIScrape(i*pageSize, pageSize)

			if err != nil {
				listings = append(listings, newListings...)
				return listings, nil
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

	for i := 0; i < pages; i++ {
		utils.Log("Scraping GradConnect: Page %d of %d...", i+1, pages)

		newListings, err := s.PerformAPIScrape(i*pageSize, pageSize)
		if errors.Is(err, ErrNoMoreResults) {
			utils.Log("Reached end of available listings")
			break
		}

		if err != nil {
			return nil, fmt.Errorf("failed to scrape GradConnect: %v", err)
		}

		listings = append(listings, newListings...)
	}

	return listings, nil
}
