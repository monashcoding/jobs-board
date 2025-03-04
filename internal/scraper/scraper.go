package scraper

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// Scraper interface defines methods that each site scraper must implement
type Scraper interface {
	Name() string
	URL() string // Each scraper defines its own URLs
	Scrape(ctx context.Context) ([]models.Listing, error)
	SetJobLimit(limit int)
}
