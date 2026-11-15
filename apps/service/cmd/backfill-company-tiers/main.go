// Command backfill-company-tiers writes company_tier onto the documents already
// in active_jobs.
//
// WHY THIS EXISTS: the scraper sets company_tier alongside board_score, so every
// listing a run sees is tiered from the moment the field ships. Documents the
// run does not see are not: a listing that has closed, or one whose source has
// stopped carrying it, keeps whatever it had. The Discord bot orders its weekly
// recap by this field and sorts an untiered listing last, so without a backfill
// the whole existing backlog ranks below anything scraped afterwards.
//
// The tier is derived from the stored company name, so this is idempotent and
// safe to re-run: a second pass reports zero changes. Re-run it after editing
// the tier lists in internal/utils/notable_companies.go.
//
//	go run ./cmd/backfill-company-tiers            # dry run
//	go run ./cmd/backfill-company-tiers -apply     # write
//
// It only ever adds or corrects one field, so it can run with the Discord bot
// up. Each write is a change stream event and the bot re-renders the thread it
// belongs to, which is the same cost as any ordinary scrape touching the
// listing.
package main

import (
	"context"
	"flag"
	"os"
	"sort"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

// batchSize bounds one bulk write. Large enough that the pass is a handful of
// round trips, small enough that a failure names a manageable set of documents.
const batchSize = 500

type docTier struct {
	id       primitive.ObjectID
	company  string
	stored   string
	computed string
	eligible bool
}

func main() {
	apply := flag.Bool("apply", false, "write changes; without this the tool only reports what it would do")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
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

	coll := mongodb.GetDatabase().Collection("active_jobs")

	docs := loadTiers(ctx, coll)
	if len(docs) == 0 {
		utils.Warn("active_jobs is empty; nothing to backfill")
		return
	}

	pending := report(docs)

	if len(pending) == 0 {
		utils.Success("Every document already carries the right company_tier; nothing to do.")
		return
	}

	if !*apply {
		utils.Warn("Dry run. Re-run with -apply to write these changes.")
		return
	}

	write(ctx, coll, pending)
}

func loadTiers(ctx context.Context, coll *mongo.Collection) []docTier {
	cursor, err := coll.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{
		"company.name":   1,
		"company_tier":   1,
		"board_eligible": 1,
	}))
	if err != nil {
		utils.Fatal("Failed to read active_jobs: %v", err)
	}
	defer cursor.Close(ctx)

	var docs []docTier
	for cursor.Next(ctx) {
		var raw struct {
			ID      primitive.ObjectID `bson:"_id"`
			Company struct {
				Name string `bson:"name"`
			} `bson:"company"`
			CompanyTier   string `bson:"company_tier"`
			BoardEligible bool   `bson:"board_eligible"`
		}
		if err := cursor.Decode(&raw); err != nil {
			utils.Warn("Skipping undecodable document: %v", err)
			continue
		}

		docs = append(docs, docTier{
			id:       raw.ID,
			company:  raw.Company.Name,
			stored:   raw.CompanyTier,
			computed: utils.CompanyTier(raw.Company.Name),
			eligible: raw.BoardEligible,
		})
	}
	if err := cursor.Err(); err != nil {
		utils.Fatal("Cursor error reading active_jobs: %v", err)
	}

	return docs
}

// report prints what the pass would do and returns the documents that need a
// write. An eligible listing that comes out unranked is called out by name: the
// company list is a hard gate for eligibility, so it should be impossible, and
// it means the two tier lists no longer cover the whole notable list.
func report(docs []docTier) []docTier {
	var pending []docTier
	counts := map[string]int{}
	var untierable []docTier

	for _, doc := range docs {
		counts[doc.computed]++

		if doc.eligible && doc.computed == utils.TierUnranked {
			untierable = append(untierable, doc)
		}

		if doc.stored != doc.computed {
			pending = append(pending, doc)
		}
	}

	utils.Log("Documents:              %d", len(docs))
	utils.Log("  headline:             %d", counts[utils.TierHeadline])
	utils.Log("  major:                %d", counts[utils.TierMajor])
	utils.Log("  unranked:             %d", counts[utils.TierUnranked])
	utils.Log("Documents to write:     %d", len(pending))

	if len(untierable) > 0 {
		utils.Warn("%d board-eligible listings have no tier. Every eligible employer is on "+
			"the notable list, so these names are missing from both tier lists in "+
			"internal/utils/notable_companies.go:", len(untierable))
		reportEmployers(untierable)
	}

	return pending
}

// reportEmployers prints the distinct employers behind a set of documents, most
// listings first, so a gap in the tier lists can be fixed by name.
func reportEmployers(docs []docTier) {
	byEmployer := map[string]int{}
	for _, doc := range docs {
		byEmployer[doc.company]++
	}

	names := make([]string, 0, len(byEmployer))
	for name := range byEmployer {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if byEmployer[names[i]] != byEmployer[names[j]] {
			return byEmployer[names[i]] > byEmployer[names[j]]
		}
		return names[i] < names[j]
	})

	for i, name := range names {
		if i >= 20 {
			utils.Warn("  ... and %d more", len(names)-20)
			break
		}
		utils.Warn("  x%d  %s", byEmployer[name], name)
	}
}

func write(ctx context.Context, coll *mongo.Collection, pending []docTier) {
	written, failed := 0, 0

	for start := 0; start < len(pending); start += batchSize {
		end := start + batchSize
		if end > len(pending) {
			end = len(pending)
		}

		models := make([]mongo.WriteModel, 0, end-start)
		for _, doc := range pending[start:end] {
			models = append(models, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": doc.id}).
				SetUpdate(bson.M{"$set": bson.M{"company_tier": doc.computed}}))
		}

		// Unordered, so one rejected document does not abandon the rest of its
		// batch. Every write is independent.
		result, err := coll.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
		if result != nil {
			written += int(result.ModifiedCount)
		}
		if err != nil {
			utils.Error("Batch %d-%d failed: %v", start, end, err)
			failed += end - start
		}
	}

	utils.Success("Backfill complete: %d documents tiered, %d failed", written, failed)

	if failed > 0 {
		os.Exit(1)
	}
}
