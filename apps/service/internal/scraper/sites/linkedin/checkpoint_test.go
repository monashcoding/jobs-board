package linkedin

import (
	"context"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// fakeCheckpoint stands in for the Mongo-backed store and records what the
// scraper asked of it.
type fakeCheckpoint struct {
	seen     map[string]models.Listing
	recorded []string
	lookups  int
}

func (f *fakeCheckpoint) Seen(key string) (models.Listing, bool) {
	f.lookups++
	listing, ok := f.seen[key]
	return listing, ok
}

func (f *fakeCheckpoint) Record(ctx context.Context, key string, listing models.Listing) {
	f.recorded = append(f.recorded, key)
	f.seen[key] = listing
}

// The whole point: a job an earlier attempt already fetched is not fetched
// again. The scraper here has no unblocker at all, so anything that reached
// processJob would fail the test rather than quietly hitting the network.
func TestCheckpointedJobsAreNotRefetched(t *testing.T) {
	jobs := []Job{{ID: "4455907736"}, {ID: "4455907737"}, {ID: "4455907738"}}

	cp := &fakeCheckpoint{seen: map[string]models.Listing{
		"4455907736": {Title: "Software Engineer"},
		"4455907737": {Title: "Data Engineer"},
		"4455907738": {Title: "Security Engineer"},
	}}

	s := &LinkedInScraper{checkpoint: cp}
	listings := s.processJobs(context.Background(), jobs)

	if len(listings) != len(jobs) {
		t.Fatalf("expected every job recovered from the checkpoint, got %d of %d", len(listings), len(jobs))
	}
	if len(cp.recorded) != 0 {
		t.Errorf("nothing was refetched, so nothing should have been recorded again: %v", cp.recorded)
	}

	// Recovered listings must keep the order of the search results, not the
	// order they happen to come back from storage in.
	want := []string{"Software Engineer", "Data Engineer", "Security Engineer"}
	for i, title := range want {
		if listings[i].Title != title {
			t.Errorf("listing %d is %q, want %q", i, listings[i].Title, title)
		}
	}
}

// A run that died part-way leaves some jobs checkpointed and some not. The
// scraper must consult the checkpoint for every job, so the ones it holds are
// skipped and only the remainder cost a fetch.
func TestOnlyTheUnfetchedJobsAreLookedPast(t *testing.T) {
	jobs := []Job{{ID: "4455907736"}, {ID: "4455907737"}}

	cp := &fakeCheckpoint{seen: map[string]models.Listing{
		"4455907736": {Title: "Software Engineer"},
	}}

	s := &LinkedInScraper{checkpoint: cp, jobLimit: 1}
	listings := s.processJobs(context.Background(), jobs)

	if cp.lookups != 1 {
		t.Errorf("expected the checkpoint to be consulted for the first job, got %d lookups", cp.lookups)
	}
	// The job limit is reached by the recovered listing alone, so the second
	// job is never reached and never fetched.
	if len(listings) != 1 || listings[0].Title != "Software Engineer" {
		t.Errorf("expected the recovered listing to satisfy the limit, got %+v", listings)
	}
}

// Without a checkpoint the scraper must behave exactly as it did before.
func TestAScraperWithoutACheckpointIsUnaffected(t *testing.T) {
	s := &LinkedInScraper{}

	if listings := s.processJobs(context.Background(), nil); len(listings) != 0 {
		t.Errorf("expected no listings from no jobs, got %d", len(listings))
	}
}
