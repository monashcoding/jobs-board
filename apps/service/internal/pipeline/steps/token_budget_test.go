package steps

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"
)

// newTestBudget gives the budget a clock the test drives, so pacing can be
// asserted without any real waiting.
func newTestBudget(t *testing.T, limit int) (*tokenBudget, *time.Time, *[]time.Duration) {
	t.Helper()

	now := time.Unix(0, 0)
	var slept []time.Duration

	b := newTokenBudget(limit)
	// newTokenBudget applies headroom; the tests want the exact number.
	b.limit = limit
	b.nowFunc = func() time.Time { return now }
	b.sleepFor = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		now = now.Add(d)
		return nil
	}

	return b, &now, &slept
}

func TestTokenBudgetAllowsSpendUpToTheLimit(t *testing.T) {
	b, _, slept := newTestBudget(t, 1000)

	for i := 0; i < 10; i++ {
		if err := b.Acquire(context.Background(), 100); err != nil {
			t.Fatalf("Acquire: %v", err)
		}
	}

	if len(*slept) != 0 {
		t.Fatalf("spending exactly the limit should never wait, waited %v", *slept)
	}
}

// This is the behaviour that replaces discovering the ceiling as a 429.
func TestTokenBudgetWaitsForTheWindowToDrain(t *testing.T) {
	b, _, slept := newTestBudget(t, 1000)

	if err := b.Acquire(context.Background(), 1000); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if len(*slept) != 0 {
		t.Fatalf("the first full-budget request must not wait")
	}

	if err := b.Acquire(context.Background(), 500); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	if len(*slept) != 1 {
		t.Fatalf("expected exactly one wait, got %v", *slept)
	}
	if (*slept)[0] != time.Minute {
		t.Fatalf("waited %s, want the full window to drain (%s)", (*slept)[0], time.Minute)
	}
}

// Spends age out, so a paced run keeps flowing rather than stalling forever.
func TestTokenBudgetReleasesAfterTheWindow(t *testing.T) {
	b, now, slept := newTestBudget(t, 1000)

	if err := b.Acquire(context.Background(), 1000); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	*now = now.Add(time.Minute + time.Second)

	if err := b.Acquire(context.Background(), 1000); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if len(*slept) != 0 {
		t.Fatalf("a drained window must not wait, waited %v", *slept)
	}
}

// A single request bigger than the entire budget can never fit. It has to go
// through once the window is clear rather than spin forever.
func TestTokenBudgetDoesNotDeadlockOnAnOversizeRequest(t *testing.T) {
	b, _, _ := newTestBudget(t, 100)

	done := make(chan error, 1)
	go func() { done <- b.Acquire(context.Background(), 5000) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("an oversize request deadlocked instead of being let through")
	}
}

func TestTokenBudgetRespectsContextCancellation(t *testing.T) {
	b := newTokenBudget(1000)
	b.limit = 100

	if err := b.Acquire(context.Background(), 100); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := b.Acquire(ctx, 100); err == nil {
		t.Fatal("a cancelled context must abort the wait, not block the run")
	}
}

func TestTokenBudgetAppliesHeadroom(t *testing.T) {
	b := newTokenBudget(200_000)
	if b.limit >= 200_000 {
		t.Fatalf("limit %d leaves no headroom for estimation error", b.limit)
	}
	if b.limit < 100_000 {
		t.Fatalf("limit %d is needlessly conservative", b.limit)
	}
}

func TestEstimateTokens(t *testing.T) {
	// Four characters per token, plus whatever the reply may cost.
	if got := estimateTokens(4000, 100); got != 1100 {
		t.Fatalf("estimateTokens(4000, 100) = %d, want 1100", got)
	}
}

// The budget is hit by every worker at once. This is the property that has to
// hold under that: the window is never over-spent, and every caller gets
// through. It runs on the real clock with a short window so the interaction
// between the mutex, the wait and the retry is exercised for real.
func TestTokenBudgetUnderConcurrencyNeverOverspendsAndAlwaysCompletes(t *testing.T) {
	const (
		limit     = 1000
		window    = 200 * time.Millisecond
		workers   = 24
		perWorker = 8
		cost      = 100
	)

	b := newTokenBudget(limit)
	b.limit = limit
	b.window = window

	// Record the budget's own stamps, not the caller's. A goroutine can be
	// descheduled between being granted and reading the clock, which would make
	// correctly paced grants look bunched and fail this test spuriously.
	var (
		mu     sync.Mutex
		grants []time.Time
	)
	b.onGrant = func(at time.Time, _ int) {
		mu.Lock()
		grants = append(grants, at)
		mu.Unlock()
	}

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				if err := b.Acquire(context.Background(), cost); err != nil {
					t.Errorf("Acquire: %v", err)
					return
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("workers stalled: the budget deadlocked under concurrency")
	}

	if len(grants) != workers*perWorker {
		t.Fatalf("got %d grants, want %d", len(grants), workers*perWorker)
	}

	// No window anywhere on the timeline may hold more than the limit.
	sort.Slice(grants, func(i, j int) bool { return grants[i].Before(grants[j]) })
	for i := range grants {
		spent := 0
		for j := i; j < len(grants) && grants[j].Sub(grants[i]) < window; j++ {
			spent += cost
		}
		if spent > limit {
			t.Fatalf("window starting at +%s spent %d tokens, over the %d limit",
				grants[i].Sub(start), spent, limit)
		}
	}
}
