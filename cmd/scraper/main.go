package scraper

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
	"github.com/monashcoding/mploy-golang-core/internal/scraper"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites"
)

func main() {
	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up MongoDB connection
	mongodb, err := database.NewMongoDB(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer mongodb.Disconnect(ctx)

	// Set up graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-shutdown
		log.Println("Shutting down gracefully...")
		cancel()
	}()

	// Initialize scrapers
	scrapers := []scraper.Scraper{
		sites.NewGradconScraper(),
		sites.NewProspleScraper(),
		sites.NewLinkedinScraper(),
	}

	// Initialize pipeline
	processingPipeline := pipeline.NewPipeline(
		steps.NewNormalizationStep(),
		steps.NewDeduplicationStep(),
		steps.NewMongoDBStep(mongodb.GetCollection()),
	)

	// Collect all listings from scrapers
	var allListings []models.Listing
	for _, s := range scrapers {
		listings, err := s.Scrape(ctx)
		if err != nil {
			log.Printf("Error in %s scraper: %v", s.Name(), err)
			continue
		}
		allListings = append(allListings, listings...)
	}

	// Run pipeline with scrapers
	if _, err := processingPipeline.Run(ctx, allListings); err != nil {
		log.Printf("Error processing listings: %v", err)
	}

	log.Println("Processing complete")
}
