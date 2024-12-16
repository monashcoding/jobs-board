package sites

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"

	"github.com/monashcoding/mploy-golang-core/internal/scraper"
)

type LinkedinScraper struct {
	*scraper.CollyScraper
}

func NewLinkedinScraper() *LinkedinScraper {
	return &LinkedinScraper{
		CollyScraper: scraper.NewCollyScraper(),
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
