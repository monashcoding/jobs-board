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

// CollyScraper provides common functionality for all scrapers
type CollyScraper struct {
	collector *colly.Collector
}

// NewCollyScraper creates a new base scraper with configured Colly collector
func NewCollyScraper() *CollyScraper {
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

	return &CollyScraper{
		collector: c,
	}
}

// SetCollector allows replacing the collector for testing
func (s *BaseScraper) SetCollector(c *colly.Collector) {
	s.collector = c
}

// GetCollector returns the current collector
func (s *BaseScraper) GetCollector() *colly.Collector {
	return s.collector
}
