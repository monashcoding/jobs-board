package sites

import (
	"context"
	"fmt"

	"github.com/monashcoding/mploy-golang-core/internal/models"

	"github.com/monashcoding/mploy-golang-core/internal/scraper"
)

type ProspleScraper struct {
	*scraper.BaseScraper
}

func NewProspleScraper() *ProspleScraper {
	return &ProspleScraper{
		BaseScraper: scraper.NewBaseScraper(),
	}
}

func (s *ProspleScraper) Name() string {
	return "Prosple"
}

func (s *ProspleScraper) URL() string {
	return "https://au.prosple.com/"
}

func (s *ProspleScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	fmt.Println("Scraping Prosple")
	return nil, nil
}