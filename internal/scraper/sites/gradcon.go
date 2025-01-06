package sites

import (
	"context"
	"fmt"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/scraper"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/gradcon"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type GradconScraper struct {
	*scraper.CollyScraper
}

func NewGradconScraper() *GradconScraper {
	return &GradconScraper{
		CollyScraper: scraper.NewCollyScraper(),
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

	result, err := gradcon.SendAPIRequest(gradConnectApiUrl, pageSize, offset)
	if err != nil {
		// return listings, err
	}

	for _, item := range *result {
		gradConnectListings, err := gradcon.GradConnectToMPLOY(item, s.URL())

		if err != nil {
			utils.Warn("Error converting opportunity to model: %v", err)
			continue
		}
		listings = append(listings, gradConnectListings...)
	}

	return listings, nil
}

func (s *GradconScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	pages := 10    // Number of pages to scrape
	pageSize := 20 // Number of listings per page

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

		if err != nil {
			return nil, fmt.Errorf("failed to scrape GradConnect: %v", err)
		}

		listings = append(listings, newListings...)
	}

	return listings, nil
}
