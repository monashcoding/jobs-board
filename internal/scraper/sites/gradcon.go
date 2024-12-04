package sites

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"

	"github.com/monashcoding/mploy-golang-core/internal/scraper"
)

type GradconScraper struct {
	*scraper.BaseScraper
}

func NewGradconScraper() *GradconScraper {
	return &GradconScraper{
		BaseScraper: scraper.NewBaseScraper(),
	}
}

func (s *GradconScraper) Name() string {
	return "GradConnections"
}

func (s *GradconScraper) URL() string {
	return "https://au.gradconnection.com/"
}

func (s *GradconScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	return nil, nil
}
