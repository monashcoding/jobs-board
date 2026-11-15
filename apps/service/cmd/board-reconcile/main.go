// Command board-reconcile brings the Discord job board to its cap against the
// whole active_jobs collection, rather than against one scrape.
//
// WHY THIS EXISTS: BoardScoring caps the listings a run gives it. A run is
// whatever the scrapers returned, so the cap is sized against a fraction of the
// board whenever RUN_SCRAPERS names a subset, and eligibility written weeks ago
// is never revisited. The same pass runs at the end of every scrape
// (BoardReconciliation); this is it on its own, for when the board needs fixing
// without waiting for a scrape, or when you want to see what it would do first.
//
//	go run ./cmd/board-reconcile            # dry run
//	go run ./cmd/board-reconcile -apply     # write
//
// The caps come from the same environment as the scraper's:
// BOARD_MAX_LISTINGS (default 100) and BOARD_MAX_PER_EMPLOYER (default 5).
//
// It only ever writes board_eligible, and never takes a slot from a listing
// that still qualifies for one, so it is safe to run with the Discord bot up:
// a promotion is a change stream event and the bot posts the thread, which is
// what a freshly scraped eligible listing does anyway.
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

func main() {
	apply := flag.Bool("apply", false, "write changes; without this the tool only reports what it would do")
	limit := flag.Int("limit", 0, "board cap; 0 reads BOARD_MAX_LISTINGS")
	perEmployer := flag.Int("per-employer", 0, "per-employer cap; 0 reads BOARD_MAX_PER_EMPLOYER")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Load .env the same way the scraper does. Only the MongoDB settings matter
	// here -- this tool never scrapes, so it deliberately does not require the
	// BrightData/OpenAI/Discord variables that config.ValidateEnvironment does.
	if err := godotenv.Load(); err != nil {
		utils.Warn("No .env file loaded (%v), falling back to the process environment.", err)
	}

	if os.Getenv("MONGODB_URI") == "" {
		utils.Fatal("MONGODB_URI is not set. It must point at the same database the scraper writes to.")
	}

	mongodb, err := database.NewMongoDB(ctx)
	if err != nil {
		utils.Fatal("Failed to connect to MongoDB: %v", err)
	}
	defer mongodb.Disconnect(ctx)

	boardLimit, boardPerEmployer := steps.BoardCaps()
	if *limit != 0 {
		boardLimit = *limit
	}
	if *perEmployer != 0 {
		boardPerEmployer = *perEmployer
	}

	coll := mongodb.GetDatabase().Collection("active_jobs")
	if err := steps.ReconcileBoard(ctx, coll, boardLimit, boardPerEmployer, *apply); err != nil {
		utils.Fatal("Board reconciliation failed: %v", err)
	}

	if !*apply {
		utils.Warn("Dry run. Re-run with -apply to write these changes.")
	}
}
