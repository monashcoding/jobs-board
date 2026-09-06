package steps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		set["company_tier"] = listing.CompanyTier
	}

	if listing.OpenDate != nil {
		set["open_date"] = listing.OpenDate
	}

	// Written only when the AI parser ran, for the same reason as the fields
	// above: an empty hash from a partial run would otherwise invalidate the
	// stored description and force it to be reformatted from scratch.
	if listing.SourceDescriptionHash != "" {
		set["source_description_hash"] = listing.SourceDescriptionHash
	}

	return set
}

// volatileSetKeys are written on every run by definition, so they are excluded
// from the content hash. Including updated_at would make every document differ
// from itself and defeat the whole comparison.
var volatileSetKeys = map[string]bool{"updated_at": true}

// unseenListingUpdate is what is written to a listing this run did not see.
func unseenListingUpdate() bson.M {
	return bson.M{"outdated": true, "board_eligible": false}
}

// ContentHash fingerprints what this run would write for a listing. Documents
// whose hash is unchanged are skipped entirely rather than rewritten with a
// fresh updated_at.
//
// This matters well beyond saving a write. Every upsert that touches a document
// is an oplog entry and therefore a change stream event, and the Discord bot
// turns each event into a fetch of the thread, a fetch of its starter message
// and an edit. Stamping updated_at unconditionally guaranteed every document
// changed on every run, so a scrape rewrote every live thread -- thousands of
// Discord API calls to replace content with itself.
func ContentHash(set bson.M) string {
	stable := make(map[string]interface{}, len(set))
	for k, v := range set {
		if !volatileSetKeys[k] {
			stable[k] = v
		}
	}

	// encoding/json sorts map keys, so this encoding is stable across runs.
	encoded, err := json.Marshal(stable)
	if err != nil {
		// A listing we cannot encode simply always writes, which is the safe
		// direction: it can never suppress a real change.
		utils.Warn("Failed to hash listing content, it will be rewritten: %v", err)
		return ""
	}

	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// storedState is the minimum needed to decide whether a document can be left
// untouched.
type storedState struct {
	Fingerprint string `bson:"fingerprint"`
	ContentHash string `bson:"content_hash"`
	Outdated    bool   `bson:"outdated"`
	DaysLived   int    `bson:"days_lived"`
}

// loadStoredState fetches the current hash and liveness of every fingerprint
// this run saw, so unchanged documents can be skipped.
func loadStoredState(ctx context.Context, coll *mongo.Collection, fingerprints []string) map[string]storedState {
	state := make(map[string]storedState)

	cursor, err := coll.Find(ctx,
		bson.M{"fingerprint": bson.M{"$in": fingerprints}},
		options.Find().SetProjection(bson.M{
			"fingerprint":  1,
			"content_hash": 1,
			"outdated":     1,
			"days_lived":   1,
		}))
	if err != nil {
		utils.Warn("Failed to load stored state; every document will be rewritten: %v", err)
		return state
	}
	defer cursor.Close(ctx)

	var docs []storedState
	if err := cursor.All(ctx, &docs); err != nil {
		utils.Warn("Failed to decode stored state; every document will be rewritten: %v", err)
		return state
	}

	for _, doc := range docs {
		// Duplicates share a fingerprint until the migration deletes them.
		// Preferring the live one keeps a stale duplicate from suppressing a
		// write the real document needs.
		if existing, clash := state[doc.Fingerprint]; clash && !existing.Outdated {
			continue
		}
		state[doc.Fingerprint] = doc
	}

	return state
}

// needsWrite reports whether a listing has to be written at all.
func needsWrite(stored storedState, found bool, hash string) bool {
	if !found || hash == "" || stored.ContentHash == "" {
		return true
	}
	if stored.ContentHash != hash {
		return true
	}
	// The content matches, but the document still has to be brought back to
	// life if a previous run marked it outdated or started ageing it out.
	return stored.Outdated || stored.DaysLived != 0
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

	// Eligibility is cleared alongside outdated. Board scoring only ever writes
	// board_eligible for the listings in the run it is given, so a listing the
	// scraper stops seeing keeps whatever flag it was last given -- forever. A
	// board built weeks ago therefore stayed live under a cap that had since
	// been added: 83 documents were eligible after a run that marked 37, and
	// 27 of the stragglers were one employer, which is the exact pile-up the
	// per-employer cap exists to prevent. A listing that is gone from the
	// source cannot belong on the board.
	_, err := activeColl.UpdateMany(ctx,
		bson.M{"fingerprint": bson.M{"$nin": seenFingerprints}},
		bson.M{"$set": unseenListingUpdate()})
	if err != nil {
		return nil, fmt.Errorf("failed to mark existing jobs: %w", err)
	}

	// Skip documents this run would rewrite identically. Each write is a change
	// stream event and the Discord bot edits a thread for every one of them.
	stored := loadStoredState(ctx, activeColl, seenFingerprints)

	operations := make([]mongo.WriteModel, 0, len(listings))
	unchanged := 0

	for i, listing := range listings {
		fingerprint := seenFingerprints[i]

		set := buildListingSet(listing, fingerprint, s.version, now)
		hash := ContentHash(set)

		state, found := stored[fingerprint]
		if !needsWrite(state, found, hash) {
			unchanged++
			continue
		}

		if hash != "" {
			set["content_hash"] = hash
		}

		operations = append(operations, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"fingerprint": fingerprint}).
			SetUpdate(bson.M{
				"$set": set,
				"$setOnInsert": bson.M{
					"created_at": now,
				},
			}).
			SetUpsert(true))
	}

	utils.Success("Upload: %d listings changed, %d unchanged and left untouched.",
		len(operations), unchanged)

	batchSize := 100
	for i := 0; i < len(operations); i += batchSize {
		end := i + batchSize
		if end > len(operations) {
			end = len(operations)
		}

		opts := options.BulkWrite().SetOrdered(false)
		if _, err := activeColl.BulkWrite(ctx, operations[i:end], opts); err != nil {
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
