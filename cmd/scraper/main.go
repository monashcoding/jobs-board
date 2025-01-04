package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
	"github.com/monashcoding/mploy-golang-core/internal/utils"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline"
	"github.com/monashcoding/mploy-golang-core/internal/scraper"
	"github.com/monashcoding/mploy-golang-core/internal/scraper/sites"
)

func main() {
	// Load environment variables
	utils.Log("Loading environment variables from .env file...")
	if err := godotenv.Load(); err != nil {
		utils.Fatal(".env file not found: %v\n", err)
	}
	utils.Log("Environment variables loaded.")

	// Scraper Wrapper
	if (os.Getenv("SCRAPING_TYPE") == "INSTANT") { 
		RunScraper()
		return
	}

	if (os.Getenv("SCRAPING_TYPE") == "SCHEDULED") {
		ScheduleScraper()
		return
	}

	utils.Fatal("Invalid scraping type. Please set SCRAPING_TYPE to either INSTANT or SCHEDULED in .env file.")
}

func ScheduleScraper() {
	now := time.Now()

	// Calculate next run time
	scheduledHour := os.Getenv("SCRAPING_TIME_HOUR")
	scheduledMinute := os.Getenv("SCRAPING_TIME_MINUTE")

	if scheduledHour == "" || scheduledMinute == "" {
		utils.Fatal("Scheduled hour or minute not set in .env file.")
	}	

	scheduledHourInt, err := strconv.Atoi(scheduledHour)
	if err != nil {
		utils.Fatal("Invalid scheduled hour in .env file.")
	}

	scheduledMinuteInt, err := strconv.Atoi(scheduledMinute)
	if err != nil {
		utils.Fatal("Invalid scheduled minute in .env file.")
	}

	scheduledDay := now.Day()

	// if hour and minute is before now, schedule for next day
	if scheduledHourInt < now.Hour() || (scheduledHourInt == now.Hour() && scheduledMinuteInt < now.Minute()) {
		scheduledDay = now.Day() + 1
	}

	nextRun := time.Date(
		now.Year(), // year
		now.Month(), // month
		scheduledDay, // day
		scheduledHourInt, // hour 
		scheduledMinuteInt, // minute
		0, // second
		0, // nanosecond
		now.Location(), // location
	)

	utils.Log("Next run at %s", nextRun.String())
	
	duration := nextRun.Sub(now)

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
		sites.NewProspleScraper(),
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