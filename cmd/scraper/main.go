package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/config"
	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
	"github.com/monashcoding/mploy-golang-core/internal/scraper"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/airwallex"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/atlassian"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/gradcon"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/janestreet"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/linkedin"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/prosple"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

func main() {
	// Validate environment variables
	missing := config.ValidateEnvironment()
	if len(missing) > 0 {
		utils.Fatal("Missing required environment variables: %s", strings.Join(missing, ", "))
	}

	utils.Success("Environment variables loaded and validated.")
	RunScraper()
}

func RunScraper() {
	startTime := time.Now()

	// Whitelist current IP with BrightData before starting the scraper
	if err := utils.WhitelistCurrentIP(); err != nil {
		utils.Fatal("Failed to whitelist IP with BrightData: %v", err)
	}

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up MongoDB connection
	mongodb, err := database.NewMongoDB(ctx)
	if err != nil {
		utils.Fatal("Failed to connect to MongoDB: %v", err)
	}
	defer mongodb.Disconnect(ctx)
	utils.Success("MongoDB connected.")

	// Set up graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-shutdown
		utils.Log("Received shutdown signal, shutting down...")
		cancel()
	}()

	// Create MVP Upload step for use in pipeline and logging
	mvpUpload := steps.NewMongoDBUploadStep(mongodb, os.Getenv("VERSION"))
	utils.SetLogHandler(func(log string) {
		mvpUpload.AddLog(log)
	})

	// Initialize scrapers
	scrapers := initializeScrapers()

	// Initialize pipeline steps
	pipelineSteps := initializePipelineSteps(mongodb, mvpUpload)

	// Collect all listings from active scrapers
	var allListings []models.Listing

	for _, s := range scrapers {
		if config.ShouldRunScraper(s.Name()) {
			utils.Log("Running scraper %s...", s.Name())
			listings, err := s.Scrape(ctx)
			if err != nil {
				utils.Error("Error in %s scraper: %v", s.Name(), err)
				continue
			}

			utils.Success("Finished scraper %s with %d listings.", s.Name(), len(listings))
			utils.Custom("Scraper %s returned %d listings.", s.Name(), len(listings))
			allListings = append(allListings, listings...)
		}
	}

	utils.Success("Scraping section finished. Starting pipelines...")

	// Surface every location string no city table matched, so the tables can be
	// grown from real scrape output instead of guesswork.
	utils.ReportUnmappedLocations()

	// Create the pipeline with active steps
	processingPipeline := pipeline.NewPipeline(pipelineSteps)

	// Run the pipeline
	if _, err := processingPipeline.Run(ctx, allListings); err != nil {
		utils.Error("Error processing listings: %v", err)
	}

	utils.Success("Pipelines completed.")
	utils.Custom("Total listings processed: %d", len(allListings))
	// Send completion notification
	if err := utils.SendScraperCompleteEmbed(startTime, len(allListings)); err != nil {
		utils.Warn("Failed to send completion notification to Discord: %v", err)
	}
}

// initializeScrapers creates all available scrapers
func initializeScrapers() []scraper.Scraper {
	// Create all scrapers
	allScrapers := []scraper.Scraper{
		gradcon.NewGradconScraper(),
		prosple.NewProspleScraper(),
		linkedin.NewLinkedInScraper(),
		janestreet.NewJaneStreetScraper(),
		atlassian.NewAtlassianScraper(),
		airwallex.NewAirwallexScraper(),
	}

	var validScrapers []scraper.Scraper

	// Apply job limits where applicable
	for _, s := range allScrapers {
		if config.ShouldRunScraper(s.Name()) {
			if jobLimit := config.GetScraperJobLimit(s.Name()); jobLimit > 0 {
				utils.Log("Setting %s job limit to %d", s.Name(), jobLimit)
				s.SetJobLimit(jobLimit)
			}
			validScrapers = append(validScrapers, s)
		}
	}

	return validScrapers
}

// initializePipelineSteps creates all available pipeline steps
func initializePipelineSteps(mongodb *database.MongoDB, uploadStep *steps.MongoDBUploadStep) []pipeline.Step {
	// Create all pipeline steps
	allSteps := []pipeline.Step{
		steps.NewWriteJSONStep(),
		steps.NewNormalisationStep(),
		steps.NewDeduplicationStep(),
		steps.NewSynchronisationStep(),
		steps.NewAiParserStepWithCache(mongodb),
		steps.NewBoardScoringStep(),
		uploadStep,
	}

	// Filter steps based on configuration
	var pipelineSteps []pipeline.Step

	// Only use configured steps
	for _, step := range allSteps {
		if config.ShouldRunPipelineStep(step.Name()) {
			utils.Log("Including pipeline step: %s", step.Name())
			pipelineSteps = append(pipelineSteps, step)
		}
	}

	return pipelineSteps
}
