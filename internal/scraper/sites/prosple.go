package sites

import (
	"context"
	"fmt"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/prosple"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type ProspleScraper struct{}

func NewProspleScraper() *ProspleScraper {
	return &ProspleScraper{}
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

func (s *ProspleScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	prospleApiUrl := s.API_URL()

	pages := 2 // Number of pages to scrape
	pageSize := 50 // Number of listings per page

	var listings []models.Listing

	for i := 0; i < pages; i++ {		
		utils.Log("Scraping Prosple: Page %d of %d...", i + 1, pages)

		query := prosple.GetGraphQlQuery()
		variables := prosple.GetGraphQlVariables(i * pageSize, pageSize)
	
		response, err := prosple.SendGraphQLQuery(prospleApiUrl, query, variables)

		if err != nil {
			return nil, fmt.Errorf("GraphQL query failed: %v", err)
		}
	
		for _, opp := range response.Data.OpportunitiesSearch.Opportunities {
			listing, err := prosple.OpportunityToModel(s.URL(), opp)
			if err != nil {
				fmt.Printf("Error converting opportunity to model: %v\n", err)
				continue
			}
			listings = append(listings, *listing)
		}
	}

	return listings, nil
}
