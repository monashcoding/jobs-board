package steps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoDBUploadStep struct {
	mongodb *database.MongoDB
	version string
	logs    []string
}

func NewMongoDBUploadStep(mongodb *database.MongoDB, version string) *MongoDBUploadStep {
	return &MongoDBUploadStep{
		mongodb: mongodb,
		logs:    make([]string, 0),
		version: version,
	}
}

func (s *MongoDBUploadStep) Name() string {
	return "MVPUpload"
}

// stripAnsiCodes removes ANSI color codes from the string
func stripAnsiCodes(str string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(str, "")
}

// AddLog allows other parts of the application to add logs
func (s *MongoDBUploadStep) AddLog(log string) {
	// Clean the log before storing
	cleanLog := stripAnsiCodes(log)
	s.logs = append(s.logs, cleanLog)
}

// fingerprintTitlePattern matches everything we drop from a title before
// hashing. Digits survive, so the 2026 and 2027 intakes of the same program stay
// distinct listings.
var fingerprintTitlePattern = regexp.MustCompile(`[^a-z0-9]+`)

// normaliseFingerprintTitle lowercases a title and reduces every run of
// punctuation or whitespace to a single space, so cosmetic rewording of the same
// job ("Cyber Graduate Program | 2027" vs "Cyber Graduate Program - 2027") keeps
// one identity.
func normaliseFingerprintTitle(title string) string {
	lowered := strings.ToLower(strings.TrimSpace(title))
	return strings.TrimSpace(fingerprintTitlePattern.ReplaceAllString(lowered, " "))
}

// GenerateListingFingerprint returns the stable identity of a listing. It is the
// upsert key for the active_jobs collection, so two scrapes of the same job must
// produce the same value or a duplicate document is inserted -- which the Discord
// bot mirrors as a duplicate forum thread.
//
// The identity is title + company + locations + SOURCE url. Two url fields are
// easy to confuse here, and the distinction is the whole point:
//
//   - application_url points at a third-party ATS and is NOT part of the identity.
//     Sources rewrite it for reasons unrelated to the job (Deloitte switched to a
//     bit.ly shortlink, Thales migrated from Workday to their own careers site),
//     and each rewrite used to mint a duplicate document.
//   - source_urls[0] is our own stable handle on the listing at the site we
//     scraped it from, and it distinguishes genuinely separate postings that share
//     a title. Jane Street lists several roles called "Software Engineer" that
//     differ only by position id; without this they collapse into one.
//
// Locations are kept: dropping them was measured to merge distinct postings that
// differ only by city.
//
// Changing this function re-keys the entire collection. Every existing document
// must be migrated in place (cmd/migrate-fingerprints) BEFORE the new scraper
// runs, or every listing is re-inserted as new.
func GenerateListingFingerprint(listing models.Listing) string {
	// Sort locations to ensure consistent order
	locations := make([]string, len(listing.Locations))
	for i, loc := range listing.Locations {
		locations[i] = string(loc)
	}
	sort.Strings(locations)

	// Scrapers set a single source URL per listing; nothing downstream merges
	// them, so the first entry is a stable choice.
	sourceURL := ""
	if len(listing.SourceURLs) > 0 {
		sourceURL = strings.TrimRight(strings.ToLower(strings.TrimSpace(listing.SourceURLs[0])), "/")
	}

	components := []string{
		normaliseFingerprintTitle(listing.Title),
		strings.ToLower(strings.TrimSpace(listing.Company.Name)),
		strings.Join(locations, "|"),
		sourceURL,
	}

	hasher := sha256.New()
	hasher.Write([]byte(strings.Join(components, ":::")))
	return hex.EncodeToString(hasher.Sum(nil))
}

// buildListingSet assembles the $set document for one listing.
//
// Fields an enrichment step may legitimately have failed to produce are written
// only when this run actually produced them, so a partial run leaves the stored
// value alone instead of nulling it out.
func buildListingSet(listing models.Listing, fingerprint string, version string, now time.Time) bson.M {
	set := bson.M{
		"title":           listing.Title,
		"description":     listing.Description,
		"company":         listing.Company,
		"application_url": listing.ApplicationURL,
		"wfh_status":      listing.WFHStatus,
		"source_urls":     listing.SourceURLs,
		"close_date":      listing.CloseDate,
		"locations":       listing.Locations,
		"study_fields":    listing.StudyFields,
		"working_rights":  listing.WorkingRights,
		"source":          listing.Source,
		"is_sponsored":    listing.IsSponsored,
		"version":         version,
		"updated_at":      now,
		"outdated":        false,
		"days_lived":      0,
		"fingerprint":     fingerprint,
	}

	// Only write the enrichment fields when this run actually produced
	// them. RUN_PIPELINE_STEPS is an allowlist, so skipping AI_Parser or
	// BoardScoring leaves them at their zero values -- writing those
	// unconditionally would erase good data across the whole collection
	// on the next partial run.
	//
	// The same applies per listing, not just per run: when an OpenAI
	// call fails the listing keeps its zero value while its neighbours
	// are enriched normally, so an unguarded write nulls out a good
	// stored value. An exhausted quota returns 429 for every remaining
	// call in the run, which turned a billing problem into silent data
	// loss across most of the collection.
	if listing.OneLinerDesc != nil && *listing.OneLinerDesc != "" {
		set["one_liner"] = listing.OneLinerDesc
	}

	if listing.Type != nil {
		set["type"] = listing.Type
	}

	if listing.IndustryField != "" {
		set["industry_field"] = listing.IndustryField
	}

	if listing.Discipline != "" {
		set["discipline"] = listing.Discipline
		set["board_score"] = listing.BoardScore
		set["board_eligible"] = listing.BoardEligible
	}

	if listing.OpenDate != nil {
		set["open_date"] = listing.OpenDate
	}

	return set
}

func (s *MongoDBUploadStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	if len(listings) == 0 {
		return listings, nil
	}

	createSnapshot := false

	if createSnapshot {
		// 1. Store historical snapshot
		// Create a timestamp-based collection name
		timestamp := time.Now().Format("2006_01_02_15_04_05")
		historicalCollection := fmt.Sprintf("jobs_%s", timestamp)

		// Create historical snapshot
		histColl := s.mongodb.GetDatabase().Collection(historicalCollection)
		docs := make([]interface{}, len(listings))
		for i, listing := range listings {
			docs[i] = listing
		}

		_, err := histColl.InsertMany(ctx, docs)
		if err != nil {
			return nil, fmt.Errorf("failed to create historical snapshot: %w", err)
		}
	}
	// 2. Update active jobs collection
	activeColl := s.mongodb.GetDatabase().Collection("active_jobs")

	// Get number of old jobs
	oldTotal, _ := activeColl.CountDocuments(ctx, bson.M{})
	oldActive, _ := activeColl.CountDocuments(ctx, bson.M{"outdated": false})

	now := time.Now()

	// Mark as outdated only the jobs that this run did NOT see. Blanket-marking
	// every document and letting the upserts flip it back produced two or three
	// writes per document per run across the whole collection, and every one of
	// those is a change-stream event the Discord bot has to process.
	seenFingerprints := make([]string, len(listings))
	for i, listing := range listings {
		seenFingerprints[i] = GenerateListingFingerprint(listing)
	}

	_, err := activeColl.UpdateMany(ctx,
		bson.M{"fingerprint": bson.M{"$nin": seenFingerprints}},
		bson.M{"$set": bson.M{"outdated": true}})
	if err != nil {
		return nil, fmt.Errorf("failed to mark existing jobs: %w", err)
	}

	batchSize := 100
	// Process listings in batches
	for i := 0; i < len(listings); i += batchSize {
		end := i + batchSize
		if end > len(listings) {
			end = len(listings)
		}
		batch := listings[i:end]

		operations := make([]mongo.WriteModel, len(batch))
		for j, listing := range batch {
			// Generate fingerprint for matching
			fingerprint := GenerateListingFingerprint(listing)

			// Prepare the update operation
			filter := bson.M{"fingerprint": fingerprint}

			set := buildListingSet(listing, fingerprint, s.version, now)

			update := bson.M{
				"$set": set,
				"$setOnInsert": bson.M{
					"created_at": now,
				},
			}

			operations[j] = mongo.NewUpdateOneModel().
				SetFilter(filter).
				SetUpdate(update).
				SetUpsert(true)
		}

		// Execute batch operation
		opts := options.BulkWrite().SetOrdered(false)
		_, err := activeColl.BulkWrite(ctx, operations, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to update active jobs batch: %w", err)
		}
	}

	// Get listings that are marked as outdated
	// Set them as false if their days_lived is less than 7 and increment their days_lived by 1
	// Keep them as true if their days_lived is greater than or equal to 7
	outdatedListingsCursor, err := activeColl.Find(ctx, bson.M{"outdated": true})
	if err != nil {
		return nil, fmt.Errorf("failed to find outdated listings: %w", err)
	}
	defer outdatedListingsCursor.Close(ctx)

	var outdatedListings []models.Listing
	if err = outdatedListingsCursor.All(ctx, &outdatedListings); err != nil {
		return nil, fmt.Errorf("failed to decode outdated listings: %w", err)
	}

	for i := 0; i < len(outdatedListings); i += batchSize {
		end := i + batchSize
		if end > len(outdatedListings) {
			end = len(outdatedListings)
		}
		batch := outdatedListings[i:end]

		var operations []mongo.WriteModel
		for _, listing := range batch {
			if listing.Days_lived >= 7 {
				continue
			}

			filter := bson.M{"_id": listing.ID}
			update := bson.M{
				"$inc": bson.M{"days_lived": 1},
				"$set": bson.M{"outdated": false},
			}

			operations = append(operations, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update))
		}

		if len(operations) > 0 {
			opts := options.BulkWrite().SetOrdered(false)
			_, err := activeColl.BulkWrite(ctx, operations, opts)
			if err != nil {
				return nil, fmt.Errorf("failed to update outdated listings batch: %w", err)
			}
		}
	}

	newTotal, _ := activeColl.CountDocuments(ctx, bson.M{})
	newActive, _ := activeColl.CountDocuments(ctx, bson.M{"outdated": false})

	utils.Success("Inserted %d new listings. Total Listings %d -> %d | Active Listings %d -> %d",
		newTotal-oldTotal,
		oldTotal,
		newTotal,
		oldActive,
		newActive,
	)

	return listings, nil
}
