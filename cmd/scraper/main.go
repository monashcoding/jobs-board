package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	config "github.com/monashcoding/mploy-golang-core"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/gradcon"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/linkedin"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/prosple"

	"github.com/joho/godotenv"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
	"github.com/monashcoding/mploy-golang-core/internal/utils"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline"
	"github.com/monashcoding/mploy-golang-core/internal/scraper"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		utils.Fatal(".env file not found: %v\n", err)
	}
	utils.Success("Environment variables loaded.")

	// Scraper Wrapper
	if os.Getenv("ENVIRONMENT") == "DEVELOPMENT" {
		RunScraper()
		return
	}

	if os.Getenv("ENVIRONMENT") == "PRODUCTION" {
		ScheduleScraper()
		return
	}

	utils.Fatal("Invalid environment. Please set ENVIRONMENT to either DEVELOPMENT or PRODUCTION in .env file.")
}

func ScheduleScraper() {
	now := time.Now().UTC()

	// Calculate next run time (xx:05 of the next hour)
	scheduledMinute := 5
	scheduledHour := now.Hour()
	scheduledDay := now.Day()

	// If we're past the 5th minute of this hour, schedule for next hour
	if now.Minute() >= scheduledMinute {
		scheduledHour++
		// If we're scheduling past midnight
		if scheduledHour >= 24 {
			scheduledHour = 0
			scheduledDay++
		}
	}

	nextRun := time.Date(
		now.Year(),      // year
		now.Month(),     // month
		scheduledDay,    // day
		scheduledHour,   // hour
		scheduledMinute, // minute
		0,               // second
		0,               // nanosecond
		time.UTC,        // location
	)

	utils.Log("Next run at %s UTC", nextRun.Format("2006-01-02 15:04:05 MST"))

	duration := nextRun.Sub(now)

	// Create ticker for hourly runs
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// First run after initial delay
	time.Sleep(duration)
	RunScraper()

	// Subsequent runs
	for range ticker.C {
		utils.Log("Starting scheduled scraper run at %s UTC", time.Now().UTC().Format("2006-01-02 15:04:05 MST"))
		RunScraper()
	}
}

func RunScraper() {
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

	// Load Config
	config := config.GetConfig()

	// Initialize scrapers
	var scrapers []scraper.Scraper
	if config.RunGradConnection {
		gradconScraper := gradcon.NewGradconScraper()
		// Set GradConnection specific settings
		gradconScraper.SetPageSize(config.GradConnectionPageSize)
		gradconScraper.SetMaxPages(config.GradConnectionMaxPages)
		gradconScraper.SetScrapeAll(config.GradConnectionScrapeAll)

		scrapers = append(scrapers, gradconScraper)
	}
	if config.RunProsple {
		prospleScraper := prosple.NewProspleScraper()
		// Set Prosple specific settings
		prospleScraper.SetPageSize(config.ProsplePageSize)
		prospleScraper.SetMaxPages(config.ProspleMaxPages)
		prospleScraper.SetScrapeAll(config.ProspleScrapeAll)
		scrapers = append(scrapers, prospleScraper)
	}
	if config.RunLinkedIn {
		linkedinScraper := linkedin.NewLinkedInScraper()
		// Set LinkedIn specific settings
		linkedinScraper.SetMaxListings(config.LinkedInMaxListings)
		scrapers = append(scrapers, linkedinScraper)
	}
	if config.RunLinkedIn {
		linkedinScraper := linkedin.NewLinkedInScraper()
		// Set LinkedIn specific settings
		linkedinScraper.SetMaxListings(config.LinkedInMaxListings)
		scrapers = append(scrapers, linkedinScraper)
	}

	mvpUpload := steps.NewMVPUploadStep(mongodb, config.Version)
	utils.SetLogHandler(func(log string) {
		mvpUpload.AddLog(log)
	})

	// Initialize pipeline
	var pipelineSteps []pipeline.Step

	if config.RunWriteJson {
		pipelineSteps = append(pipelineSteps, steps.NewWriteJSONStep())
	}
	if config.RunNormalisation {
		pipelineSteps = append(pipelineSteps, steps.NewNormalisationStep())
	}
	if config.RunDeduplication {
		pipelineSteps = append(pipelineSteps, steps.NewDeduplicationStep())
	}
	if config.RunSynchronisation {
		pipelineSteps = append(pipelineSteps, steps.NewSynchronisationStep())
	}
	if config.RunAiParser {
		pipelineSteps = append(pipelineSteps, steps.NewAiParserStep())
	}
	if config.RunUpload {
		pipelineSteps = append(pipelineSteps, mvpUpload)
	}

	// Collect all listings from scrapers
	var allListings []models.Listing

	for _, s := range scrapers {
		utils.Log("Running scraper %s...", s.Name())
		listings, err := s.Scrape(ctx)
		if err != nil {
			utils.Error("Error in %s scraper: %v", s.Name(), err)
			continue
		}
		utils.Success("Finished scraper %s with %d listings.", s.Name(), len(listings))
		allListings = append(allListings, listings...)
	}

	utils.Success("Scraping section finished. Starting pipelines...")

	// Run pipeline with scrapers
	processingPipeline := pipeline.NewPipeline(pipelineSteps)

	if _, err := processingPipeline.Run(ctx, allListings); err != nil {
		utils.Error("Error processing listings: %v", err)
	}

	utils.Success("Pipelines completed.")
}
