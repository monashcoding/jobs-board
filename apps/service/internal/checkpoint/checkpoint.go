// Package checkpoint remembers what a scrape already fetched, so a run that
// dies part-way can resume instead of starting over.
//
// A scrape is a long, expensive, interruptible thing: roughly eighteen minutes,
// thousands of proxied fetches, and any number of ways to end early -- a
// redeploy replacing the container out from under it, an OOM, a proxy outage.
// Until now every one of those cost the whole run. One died at LinkedIn job
// 430 of 839 and the 429 already fetched were simply lost.
//
// Checkpoints live in Mongo rather than on disk deliberately: the container is
// replaced on redeploy, which is the very case this exists to survive, and
// anything written to its filesystem goes with it.
package checkpoint

import (
	"context"
	"fmt"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CollectionName holds one document per checkpointed listing.
const CollectionName = "scrape_checkpoints"

// DefaultTTL is how long a checkpoint stays usable.
//
// Postings change, so resuming into yesterday's half-run would publish stale
// data as though it were fresh. A checkpoint is for finishing an interrupted
// run today, not for skipping work tomorrow: anything older is ignored on load
// and reaped by the TTL index.
const DefaultTTL = 12 * time.Hour

// completeKey marks a scraper that ran to the end. It is not a listing key, and
// the separator makes it unmatchable by one.
const completeKey = "|complete|"

type record struct {
	Scraper string         `bson:"scraper"`
	Key     string         `bson:"key"`
	Listing models.Listing `bson:"listing"`
	SavedAt time.Time      `bson:"saved_at"`
}

// Store reads and writes checkpoints for every scraper in a run.
type Store struct {
	coll *mongo.Collection
	ttl  time.Duration
}

// NewStore builds a store over the checkpoint collection.
func NewStore(db *database.MongoDB) *Store {
	return &Store{coll: db.GetCollection(CollectionName), ttl: DefaultTTL}
}

// EnsureIndexes creates the TTL index that reaps checkpoints nothing cleared.
// A run that dies before it can clear its own checkpoints is the normal case
// here, so they have to expire on their own.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	_, err := s.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "saved_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(int32(s.ttl.Seconds())),
	})
	if err != nil {
		return fmt.Errorf("failed to create checkpoint TTL index: %w", err)
	}
	return nil
}

// Clear removes every checkpoint. Call it once a run has been uploaded, at
// which point resuming it would mean scraping the same listings twice.
func (s *Store) Clear(ctx context.Context) error {
	if _, err := s.coll.DeleteMany(ctx, bson.M{}); err != nil {
		return fmt.Errorf("failed to clear checkpoints: %w", err)
	}
	return nil
}

// For loads what the named scraper had already fetched.
//
// A load failure is never fatal: the handle comes back empty and usable, so a
// broken checkpoint costs a full scrape rather than the run.
func (s *Store) For(ctx context.Context, scraper string) *Handle {
	h := &Handle{store: s, scraper: scraper, seen: map[string]models.Listing{}}

	cursor, err := s.coll.Find(ctx,
		bson.M{"scraper": scraper, "saved_at": bson.M{"$gt": time.Now().Add(-s.ttl)}},
		options.Find().SetSort(bson.D{{Key: "saved_at", Value: 1}, {Key: "key", Value: 1}}))
	if err != nil {
		utils.Warn("Could not read checkpoints for %s, starting it from scratch: %v", scraper, err)
		return h
	}
	defer cursor.Close(ctx)

	var records []record
	if err := cursor.All(ctx, &records); err != nil {
		utils.Warn("Could not decode checkpoints for %s, starting it from scratch: %v", scraper, err)
		return &Handle{store: s, scraper: scraper, seen: map[string]models.Listing{}}
	}

	for _, r := range records {
		if r.Key == completeKey {
			h.complete = true
			continue
		}
		h.seen[r.Key] = r.Listing
		h.listings = append(h.listings, r.Listing)
	}

	if h.complete {
		utils.Log("Checkpoint: %s already finished with %d listings, skipping it.", scraper, len(h.listings))
	} else if len(h.listings) > 0 {
		utils.Log("Checkpoint: resuming %s with %d listings already fetched.", scraper, len(h.listings))
	}
	return h
}

// Handle is one scraper's view of the checkpoint.
type Handle struct {
	store    *Store
	scraper  string
	complete bool
	seen     map[string]models.Listing
	listings []models.Listing
}

// Complete reports whether this scraper ran to the end in an earlier attempt,
// in which case its stored Listings stand in for running it again.
func (h *Handle) Complete() bool { return h.complete }

// Listings returns what was recovered, in the order it was fetched.
func (h *Handle) Listings() []models.Listing { return h.listings }

// Seen reports a listing already fetched under this key.
func (h *Handle) Seen(key string) (models.Listing, bool) {
	listing, ok := h.seen[key]
	return listing, ok
}

// Record stores one freshly scraped listing.
//
// A write failure costs only this listing's place in a later resume, so it is
// logged and swallowed rather than failing the scrape it is meant to protect.
func (h *Handle) Record(ctx context.Context, key string, listing models.Listing) {
	if key == "" || key == completeKey {
		return
	}
	h.seen[key] = listing
	h.listings = append(h.listings, listing)
	h.write(ctx, key, listing)
}

// MarkComplete records that the scraper finished, so a resumed run can take its
// listings wholesale instead of fetching them again.
func (h *Handle) MarkComplete(ctx context.Context) {
	h.complete = true
	h.write(ctx, completeKey, models.Listing{})
}

func (h *Handle) write(ctx context.Context, key string, listing models.Listing) {
	// A handle with nowhere to write still tracks the run in memory, so a
	// scraper handed one behaves the same either way.
	if h.store == nil {
		return
	}

	// A checkpoint is worthless if writing it can fail the run it protects, so
	// this uses a context of its own: a cancelled scrape must still be able to
	// bank the work it already did.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	_, err := h.store.coll.UpdateOne(writeCtx,
		bson.M{"scraper": h.scraper, "key": key},
		bson.M{"$set": record{Scraper: h.scraper, Key: key, Listing: listing, SavedAt: time.Now()}},
		options.Update().SetUpsert(true))
	if err != nil {
		utils.Warn("Failed to checkpoint %s/%s, a resumed run will refetch it: %v", h.scraper, key, err)
	}
}
