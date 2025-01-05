package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
	"github.com/monashcoding/mploy-golang-core/internal/utils"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline"
	"github.com/monashcoding/mploy-golang-core/internal/scraper"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites"
)

func main() {
	utils.Log("Starting MPLOY Scraper.")

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up MongoDB connection
	utils.Log("Setting up MongoDB connection.")
	mongodb, err := database.NewMongoDB(ctx)
	if err != nil {
		utils.Fatal("Failed to connect to MongoDB: %v", err)
	}
	defer mongodb.Disconnect(ctx)

	// Set up graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-shutdown
		utils.Log("Received shutdown signal, shutting down...")
		cancel()
	}()

	// Initialize scrapers
	scrapers := []scraper.Scraper{
		sites.NewGradconScraper(),
		//sites.NewProspleScraper(),
		//sites.NewLinkedinScraper(),
	}

	// Initialize pipeline
	processingPipeline := pipeline.NewPipeline(
		steps.NewLogStep("Initial"),
		//steps.NewNormalizationStep(),
		//steps.NewDeduplicationStep(),
		steps.NewMongoDBStep(mongodb),
	)

	// Collect all listings from scrapers
	utils.Log("Scraping started...")

	var allListings []models.Listing
	for _, s := range scrapers {
		utils.Log("Running scraper %s...", s.Name())
		listings, err := s.Scrape(ctx)
		if err != nil {
			utils.Error("[X] Error in %s scraper: %v", s.Name(), err)
			continue
		}
		utils.Log("Finished scraper %s with %d listings.", s.Name(), len(listings))
		allListings = append(allListings, listings...)
	}

	utils.Log("Scraping section finished [\u2713]")
	utils.Log("Starting pipeline section [...]")

	// Run pipeline with scrapers
	if _, err := processingPipeline.Run(ctx, allListings); err != nil {
		utils.Error("Error processing listings: %v", err)
	}

	utils.Log("Pipeline section finished [\u2713]")

	utils.Log("Processing completed [\u2713]")
}
