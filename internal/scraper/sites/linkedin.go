package sites

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"

	"github.com/monashcoding/mploy-golang-core/internal/scraper"
)

type LinkedinScraper struct {
	*scraper.BaseScraper
}

func NewLinkedinScraper() *LinkedinScraper {
	return &LinkedinScraper{
		BaseScraper: scraper.NewBaseScraper(),
	}
}

func (s *LinkedinScraper) Name() string {
	return "Linkedin"
}

func (s *LinkedinScraper) URL() string {
	return "https://www.linkedin.com/jobs/"
}

func (s *LinkedinScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	return nil, nil
}
