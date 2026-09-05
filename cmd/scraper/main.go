package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/checkpoint"
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

	// Indexes are ensured before the pipeline runs rather than in a migration,
	// so a rebuilt collection heals itself on the next run.
	if err := mongodb.EnsureIndexes(ctx); err != nil {
		utils.Warn("Failed to ensure indexes, continuing with degraded query performance: %v", err)
	}

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

	// Checkpoints let an interrupted run resume. A scrape is ~18 minutes of
	// proxied fetches and any number of things end it early -- a redeploy
	// replacing the container, an OOM, a proxy outage -- and until now every one
	// of those cost the entire run.
	checkpoints := checkpoint.NewStore(mongodb)
	if err := checkpoints.EnsureIndexes(ctx); err != nil {
		utils.Warn("Checkpoints will not expire on their own: %v", err)
	}

	// Collect all listings from active scrapers
	var allListings []models.Listing

	for _, s := range scrapers {
		if config.ShouldRunScraper(s.Name()) {
			resume := checkpoints.For(ctx, s.Name())

			// A scraper that finished in an earlier attempt is not run again;
			// its listings are taken as they were.
			if resume.Complete() {
				utils.Custom("Scraper %s recovered %d listings from a checkpoint.", s.Name(), len(resume.Listings()))
				allListings = append(allListings, resume.Listings()...)
				continue
			}

			// Scrapers that know how to resume mid-way take the handle and bank
			// each item as they go. The rest start over, and have their finished
			// output checkpointed below so a later attempt skips them entirely.
			resumable, banksItself := s.(interface{ SetCheckpoint(linkedin.JobCheckpoint) })
			if banksItself {
				resumable.SetCheckpoint(resume)
			}

			utils.Log("Running scraper %s...", s.Name())
			listings, err := s.Scrape(ctx)
			if err != nil {
				utils.Error("Error in %s scraper: %v", s.Name(), err)
				continue
			}

			if !banksItself {
				for i, listing := range listings {
					resume.Record(ctx, strconv.Itoa(i), listing)
				}
			}
			resume.MarkComplete(ctx)

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
	} else {
		// Only once the pipeline has uploaded does resuming stop being useful
		// and start being wrong: the listings are banked, and a resumed run
		// would scrape them a second time. A failed pipeline keeps them, so a
		// rerun does not have to scrape the whole day again to retry the upload.
		if err := checkpoints.Clear(ctx); err != nil {
			utils.Warn("Failed to clear checkpoints; the next run may resume from them: %v", err)
		}
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
