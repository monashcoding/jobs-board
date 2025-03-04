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
	jobLimit  int
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
	}
}

func (s *GradconScraper) Name() string {
	return "GradConnections"
}

func (s *GradconScraper) URL() string {
	return "https://au.gradconnection.com"
}

func (s *GradconScraper) SetJobLimit(limit int) {
	s.jobLimit = limit
}

func (s *GradconScraper) PerformAPIScrape(offset int, pageSize int) ([]models.Listing, []string, error) {
	gradConnectApiUrl := "https://au.gradconnection.com/api/campaignsearch"
	var listings []models.Listing
	var companyIds []string

	result, err := SendAPIRequest(gradConnectApiUrl, pageSize, offset, s.unblocker)
	if err != nil {
		return nil, companyIds, err
	}

	for _, item := range *result {
		gradConnectListings, err := GradConnectToMPLOY(item, s.URL())
		companyIds = append(companyIds, item.CustomerOrganisation.Id)

		if err != nil {
			utils.Warn("Error converting opportunity to model: %v", err)
			continue
		}
		listings = append(listings, gradConnectListings...)
	}

	return listings, companyIds, nil
}

func (s *GradconScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	var listings []models.Listing
	var companyIds []string
	var pageSize = 50

	hasLimit := s.jobLimit > 0

	for i := 0; ; i++ {
		if hasLimit && len(listings) >= s.jobLimit {
			utils.Log("Reached job limit (%d), stopping scrape", s.jobLimit)
			break
		}

		utils.Log("Scraping GradConnect: Page %d...", i+1)

		newListings, newCompanyIds, err := s.PerformAPIScrape(i*pageSize, pageSize)
		if errors.Is(err, ErrNoMoreResults) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("failed to scrape GradConnect: %v", err)
		}

		// Add the new listings and company IDs
		listings = append(listings, newListings...)
		companyIds = append(companyIds, newCompanyIds...)

		// If the count of new listings is less than the page size, we've reached the end
		if len(newListings) < pageSize {
			break
		}
	}

	// Trim the list to match our target exactly
	if hasLimit && len(listings) > s.jobLimit {
		listings = listings[:s.jobLimit]
	}

	// Get Company Listing Data for Detailed Job Description
	s.MergeDetailedDescriptionsWithListings(listings, companyIds)

	return listings, nil
}
