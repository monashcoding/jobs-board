package checkpoint

import (
	"context"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

func newTestHandle() *Handle {
	return &Handle{scraper: "LinkedIn", seen: map[string]models.Listing{}}
}

// The point of a checkpoint is that a second attempt recognises what the first
// one already fetched.
func TestRecordedListingsAreSeenAgain(t *testing.T) {
	h := newTestHandle()
	listing := models.Listing{Title: "Software Engineer"}

	if _, ok := h.Seen("4455907736"); ok {
		t.Fatal("nothing has been recorded yet")
	}

	h.Record(context.Background(), "4455907736", listing)

	got, ok := h.Seen("4455907736")
	if !ok {
		t.Fatal("a recorded job must be recognised on the next attempt")
	}
	if got.Title != listing.Title {
		t.Errorf("recovered the wrong listing: %+v", got)
	}
	if len(h.Listings()) != 1 {
		t.Errorf("expected 1 recovered listing, got %d", len(h.Listings()))
	}
}

// completeKey is a marker, not a job. A scraper whose item key happened to
// collide with it would otherwise report itself finished the moment it recorded
// that item, and every later attempt would skip the scraper outright.
func TestTheCompletionMarkerCannotBeRecordedAsAListing(t *testing.T) {
	h := newTestHandle()

	h.Record(context.Background(), completeKey, models.Listing{Title: "Software Engineer"})

	if _, ok := h.Seen(completeKey); ok {
		t.Error("the completion marker must not be storable as a listing")
	}
	if len(h.Listings()) != 0 {
		t.Errorf("the completion marker must not appear among the listings, got %d", len(h.Listings()))
	}
}

// An empty key cannot be matched back to a job, so recording one would grow the
// checkpoint without ever letting a resumed run skip anything.
func TestAnEmptyKeyIsNotRecorded(t *testing.T) {
	h := newTestHandle()

	h.Record(context.Background(), "", models.Listing{Title: "Software Engineer"})

	if len(h.Listings()) != 0 {
		t.Errorf("a listing with no key must not be checkpointed, got %d", len(h.Listings()))
	}
}

// A scraper is only skipped wholesale once it has actually finished; a
// part-finished one must be run again so the rest is fetched.
func TestAScraperIsNotCompleteUntilItSaysSo(t *testing.T) {
	h := newTestHandle()
	h.Record(context.Background(), "4455907736", models.Listing{Title: "Software Engineer"})

	if h.Complete() {
		t.Error("recording a listing must not mark the whole scraper finished")
	}

	h.MarkComplete(context.Background())

	if !h.Complete() {
		t.Error("a scraper that finished must be reported complete")
	}
}
