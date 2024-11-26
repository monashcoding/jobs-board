package scraper

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"time"

	"github.com/gocolly/colly/v2"
)

// Scraper interface defines methods that each site scraper must implement
type Scraper interface {
	Name() string
	URL() string // Each scraper defines its own URLs
	Scrape(ctx context.Context) ([]models.Listing, error)
}

// BaseScraper provides common functionality for all scrapers
type BaseScraper struct {
	collector *colly.Collector
}

// NewBaseScraper creates a new base scraper with configured Colly collector
func NewBaseScraper() *BaseScraper {
	c := colly.NewCollector(
		colly.UserAgent("MPLOY"),
		colly.MaxDepth(2),
		colly.Async(true),
	)

	// Set up rate limiting
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		RandomDelay: 5 * time.Second,
		Parallelism: 2,
	})

	return &BaseScraper{
		collector: c,
	}
}
