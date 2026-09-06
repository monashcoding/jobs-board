// Command mark-for-reclassification asks the next scraper run to re-derive the
// AI classification for documents already in active_jobs.
//
// WHY THIS EXISTS: the AI parser skips any listing that already carries an
// industry, a job type and a discipline, which is what keeps a steady-state run
// down to a handful of API calls. The cost is that a listing classified under
// an older prompt keeps that answer forever. Two changes to the prompt made
// that a problem:
//
//   - Job type never defined where GRADUATE ended and OTHER began, so "junior"
//     and "entry level" roles were split across both -- ten byte-identical
//     titles came back with both answers. Job type is a hard board gate, so a
//     listing that landed on OTHER could never reach the board.
//   - Working rights are now read off the description when the source does not
//     publish them. LinkedIn publishes none and is three quarters of the board,
//     so those listings say nothing about who may apply.
//
// It marks rather than deletes. Clearing the stored fields outright would strip
// discipline and board_eligible from any listing the next run does not happen to
// scrape -- a closed listing, or one its source has stopped carrying -- and the
// upload step only writes those fields back when the AI parser produced them, so
// a listing that fell out of the scrape set would be stranded blank and drop off
// the board. A marked document keeps everything it has until a run actually
// re-derives it.
//
//	go run ./cmd/mark-for-reclassification            # dry run
//	go run ./cmd/mark-for-reclassification -apply     # write
//
// Idempotent: a document already marked is left alone, and the mark is cleared
// by the run that answers it. Safe to run with the Discord bot up -- only
// listings that are not on the board are eligible to be marked, so no already
// posted thread can lose its place.
package main

import (
	"context"
	"flag"
	"os"
	"regexp"
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

// seniorityVocabulary matches the titles the old prompt had no rule for. These
// are the words that sit between "graduate program" and "senior engineer",
// which is exactly the region the model was guessing in.
var seniorityVocabulary = regexp.MustCompile(
	`(?i)\b(junior|jnr|entry[ -]?level|fresher|early[ -]career|associate|trainee)\b`)

type candidate struct {
	id     primitive.ObjectID
	title  string
	reason string
}

func main() {
	apply := flag.Bool("apply", false, "write changes; without this the tool only reports what it would do")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Load .env the same way the scraper does. Only the MongoDB settings matter
	// here -- this tool never scrapes and never calls OpenAI, so it deliberately
	// does not require the variables config.ValidateEnvironment insists on.
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

	candidates := load(ctx, coll)
	if len(candidates) == 0 {
		utils.Success("Nothing to mark: every document already carries a classification made under the current prompt.")
		return
	}

	if !*apply {
		utils.Warn("Dry run. Re-run with -apply to mark these documents.")
		return
	}

	write(ctx, coll, candidates)
}

// load returns the documents worth re-asking about, and reports what it found.
//
// Board-eligible listings are excluded. They are already posted, and a fresh
// classification could move one to OTHER and take it off the board -- trading a
// listing a student can see today for a slightly better label. The listings this
// is meant to rescue are the ones sitting off the board, which is where every
// junior role misfiled as OTHER already is.
func load(ctx context.Context, coll *mongo.Collection) []candidate {
	filter := bson.M{
		"board_eligible":         bson.M{"$ne": true},
		"needs_reclassification": bson.M{"$ne": true},
	}

	cursor, err := coll.Find(ctx, filter, options.Find().SetProjection(bson.M{
		"title":          1,
		"type":           1,
		"working_rights": 1,
	}))
	if err != nil {
		utils.Fatal("Failed to read active_jobs: %v", err)
	}
	defer cursor.Close(ctx)

	var candidates []candidate
	scanned, seniority, rights, both := 0, 0, 0, 0

	for cursor.Next(ctx) {
		var raw struct {
			ID            primitive.ObjectID `bson:"_id"`
			Title         string             `bson:"title"`
			WorkingRights []string           `bson:"working_rights"`
		}
		if err := cursor.Decode(&raw); err != nil {
			utils.Warn("Skipping undecodable document: %v", err)
			continue
		}
		scanned++

		ambiguousTitle := seniorityVocabulary.MatchString(raw.Title)
		missingRights := len(raw.WorkingRights) == 0

		reason := ""
		switch {
		case ambiguousTitle && missingRights:
			reason = "ambiguous seniority, no working rights"
			both++
		case ambiguousTitle:
			reason = "ambiguous seniority"
			seniority++
		case missingRights:
			reason = "no working rights"
			rights++
		default:
			continue
		}

		candidates = append(candidates, candidate{id: raw.ID, title: raw.Title, reason: reason})
	}
	if err := cursor.Err(); err != nil {
		utils.Fatal("Cursor error reading active_jobs: %v", err)
	}

	utils.Log("Documents scanned (not on the board, not already marked): %d", scanned)
	utils.Log("  ambiguous seniority only:      %d", seniority)
	utils.Log("  no working rights only:        %d", rights)
	utils.Log("  both:                          %d", both)
	utils.Log("Documents to mark:               %d", len(candidates))

	if len(candidates) > 0 {
		utils.Log("A marked document costs one OpenAI call on the next run. Sample:")
		for i, c := range candidates {
			if i >= 10 {
				break
			}
			utils.Log("  %-28s  %s", c.reason, c.title)
		}
	}

	return candidates
}

func write(ctx context.Context, coll *mongo.Collection, candidates []candidate) {
	written, failed := 0, 0

	for start := 0; start < len(candidates); start += batchSize {
		end := start + batchSize
		if end > len(candidates) {
			end = len(candidates)
		}

		operations := make([]mongo.WriteModel, 0, end-start)
		for _, c := range candidates[start:end] {
			operations = append(operations, mongo.NewUpdateOneModel().
				SetFilter(bson.M{"_id": c.id}).
				SetUpdate(bson.M{
					"$set": bson.M{"needs_reclassification": true},
					// The upload step skips a listing whose content hash matches
					// what is stored, and a re-classification that lands on the
					// same answer produces exactly that. Without this the mark
					// would never be written back as cleared, and the listing
					// would be re-sent to OpenAI on every run forever. Clearing
					// the hash forces one write, which is what clears the mark.
					"$unset": bson.M{"content_hash": ""},
				}))
		}

		// Unordered, so one rejected document does not abandon the rest of its
		// batch. Every write is independent.
		result, err := coll.BulkWrite(ctx, operations, options.BulkWrite().SetOrdered(false))
		if result != nil {
			written += int(result.ModifiedCount)
		}
		if err != nil {
			utils.Error("Batch %d-%d failed: %v", start, end, err)
			failed += end - start
		}
	}

	utils.Success("Marked %d documents for reclassification, %d failed. "+
		"They are re-derived on the next scraper run.", written, failed)

	if failed > 0 {
		os.Exit(1)
	}
}
