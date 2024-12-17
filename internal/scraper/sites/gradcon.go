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

func (s *GradconScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	numItemsOnPage := 20
	num_pages := 2
	listings := []models.Listing{}
	//empListings := []tempListing{}
	for i := 0; i < num_pages; i++ {
		offset := i * numItemsOnPage

		// Create a new HTTP client
		result, err := gradcon.SendAPIRequest(s.API_URL(), 20, offset)
		if err != nil {
			utils.Error("Failed to send API Request")
		}
	
		for _, item := range *result {
			gradConnectListings, err := gradcon.GradConnectToMPLOY(item, s.URL())

			if err != nil {
				fmt.Printf("Error converting opportunity to model: %v\n", err)
				continue
			}
			listings = append(listings, gradConnectListings...)
		}

	}
	fmt.Println(listings)
	return listings, nil
}
