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

// ListingFingerprint generates a unique identifier for a listing based on its content
func generateListingFingerprint(listing models.Listing) string {
	// Normalize strings by trimming spaces and converting to lowercase
	normalizeString := func(s string) string {
		return strings.ToLower(strings.TrimSpace(s))
	}

	// Sort locations to ensure consistent order
	locations := make([]string, len(listing.Locations))
	for i, loc := range listing.Locations {
		locations[i] = string(loc)
	}
	sort.Strings(locations)

	// Create components array with required fields
	components := []string{
		normalizeString(listing.Title),
		normalizeString(listing.Company.Name),
	}

	// Add application URL if it exists.
	// NOTE: Known tracking/search params are stripped before hashing so the same listing
	// isn't treated as a new document when the scraper returns the URL with different
	// non-identifying params (e.g. q=, hl=, utm_*). Essential ATS job-identity params
	// (jobId=, vacancyid=, Areq=, etc.) are intentionally preserved.
	// See https://github.com/monashcoding/mploy-golang-core/pull/97
	if listing.ApplicationURL != nil {
		components = append(components, normalizeString(stripTrackingParams(*listing.ApplicationURL)))
	}

	// Add sorted locations
	components = append(components, strings.Join(locations, "|"))

	// Create SHA-256 hash of the combined string
	hasher := sha256.New()
	hasher.Write([]byte(strings.Join(components, ":::")))
	return hex.EncodeToString(hasher.Sum(nil))
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

	// Mark as outdated only the jobs this run did NOT see. Blanket-marking every
	// document and letting the upserts flip it back wrote to the whole
	// collection twice per run, and every one of those writes is a change-stream
	// event that downstream consumers have to process.
	seenFingerprints := make([]string, len(listings))
	for i, listing := range listings {
		seenFingerprints[i] = generateListingFingerprint(listing)
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
			fingerprint := generateListingFingerprint(listing)

			// Prepare the update operation
			filter := bson.M{"fingerprint": fingerprint}

			set := bson.M{
				"title":           listing.Title,
				"description":     listing.Description,
				"one_liner":       listing.OneLinerDesc,
				"company":         listing.Company,
				"application_url": listing.ApplicationURL,
				"wfh_status":      listing.WFHStatus,
				"source_urls":     listing.SourceURLs,
				"type":            listing.Type,
				"close_date":      listing.CloseDate,
				"locations":       listing.Locations,
				"study_fields":    listing.StudyFields,
				"working_rights":  listing.WorkingRights,
				"industry_field":  listing.IndustryField,
				"source":          listing.Source,
				"is_sponsored":    listing.IsSponsored,
				"version":         s.version,
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
			if listing.Discipline != "" {
				set["discipline"] = listing.Discipline
				set["board_score"] = listing.BoardScore
				set["board_eligible"] = listing.BoardEligible
			}

			if listing.OpenDate != nil {
				set["open_date"] = listing.OpenDate
			}

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
