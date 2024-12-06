package sites

import (
	"context"
	"fmt"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/prosple"
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

func (s *ProspleScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	prospleApiUrl := "https://prosple-gw.global.ssl.fastly.net/internal"
	query := prosple.GetGraphQlQuery()
	variables := prosple.GetGraphQlVariables()

	response, err := prosple.SendGraphQLQuery(prospleApiUrl, query, variables)
	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %v", err)
	}

	var listings []models.Listing
	for _, opp := range response.Data.OpportunitiesSearch.Opportunities {
		listing, err := prosple.OpportunityToModel(s.URL(), opp)
		if err != nil {
			fmt.Printf("Error converting opportunity to model: %v\n", err)
			continue
		}
		listings = append(listings, *listing)
	}

	return listings, nil
}
