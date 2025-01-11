package main

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/gradcon"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/linkedin"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites/prosple"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	// Melbourne is UTC+10/+11 (depending on daylight savings)
	melbOffset := 11 * time.Hour
	now := time.Now().UTC().Add(melbOffset)

	// Set scheduled time to 21:05 (9:05 PM)
	scheduledHour := 21
	scheduledMinute := 5
	scheduledDay := now.Day()

	// If current time is past 21:05, schedule for next day
	if scheduledHour < now.Hour() || (scheduledHour == now.Hour() && scheduledMinute < now.Minute()) {
		scheduledDay = now.Day() + 1
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
	).Add(melbOffset)

	utils.Log("Next run at %s (Melbourne time)", nextRun.Format("2006-01-02 15:04:05"))

	duration := nextRun.Sub(time.Now().UTC().Add(melbOffset))

	// Create ticker for daily runs
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// First run after initial delay
	time.Sleep(duration)
	RunScraper()

	// Subsequent runs
	for range ticker.C {
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

	// Initialize scrapers
	scrapers := []scraper.Scraper{
		gradcon.NewGradconScraper(),
		prosple.NewProspleScraper(),
		linkedin.NewLinkedInScraper(),
	}

	mvpUpload := steps.NewMVPUploadStep(mongodb)
	utils.SetLogHandler(func(log string) {
		mvpUpload.AddLog(log)
	})

	// Initialize pipeline
	processingPipeline := pipeline.NewPipeline(
		steps.NewWriteJSONStep(),
		//steps.NewNormalizationStep(),
		//steps.NewDeduplicationStep(),
		mvpUpload,
	)

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
	if _, err := processingPipeline.Run(ctx, allListings); err != nil {
		utils.Error("Error processing listings: %v", err)
	}

	utils.Success("Pipelines completed.")
}
