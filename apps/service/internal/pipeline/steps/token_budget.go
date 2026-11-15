package steps

import (
	"context"
	"sync"
	"time"
)

// OpenAI enforces a rolling tokens-per-minute ceiling per organisation. The
// parser used to fire as fast as six workers could go, discover the ceiling as
// a 429, and retry -- which is a slow, failure-prone way to learn a number we
// already know. Worse, every worker learned it at the same instant and retried
// together.
//
// tokenBudget spends against that ceiling deliberately instead. A request waits
// until its estimated cost fits in the window, so the limit is approached and
// never crossed. Rate limiting stops being an error to recover from and becomes
// backpressure.

// tokenBudgetWindow is the period the API's limit is measured over.
const tokenBudgetWindow = time.Minute

// defaultTokensPerMinute is the account's tokens-per-minute ceiling for
// gpt-4o-mini. Overridable via OPENAI_TOKENS_PER_MINUTE for a different tier.
const defaultTokensPerMinute = 200_000

// tokenBudgetHeadroom is the share of the ceiling actually spent. Cost is
// estimated from character counts, so the estimate is approximate and the
// remainder absorbs the error rather than turning it into a 429.
const tokenBudgetHeadroom = 0.85

// tokenBudget is a rolling-window rate limiter measured in tokens.
type tokenBudget struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	spends   []tokenSpend
	nowFunc  func() time.Time
	sleepFor func(context.Context, time.Duration) error

	// onGrant, when set, reports the instant the budget charged a spend. Tests
	// need the budget's own stamp: a caller reading the clock after Acquire
	// returns can be descheduled in between and record a later time, which
	// makes correctly paced grants look bunched together.
	onGrant func(at time.Time, tokens int)
}

type tokenSpend struct {
	at     time.Time
	tokens int
}

func newTokenBudget(tokensPerMinute int) *tokenBudget {
	limit := int(float64(tokensPerMinute) * tokenBudgetHeadroom)
	if limit < 1 {
		limit = 1
	}
	return &tokenBudget{
		limit:    limit,
		window:   tokenBudgetWindow,
		nowFunc:  time.Now,
		sleepFor: sleepWithContext,
	}
}

// Acquire blocks until spending cost tokens keeps the window under the limit,
// then records the spend. A single request larger than the whole budget is
// allowed through once the window is clear rather than deadlocking forever.
func (b *tokenBudget) Acquire(ctx context.Context, cost int) error {
	if cost < 1 {
		cost = 1
	}

	for {
		at, wait, ok := b.reserve(cost)
		if ok {
			// Reported outside the lock: a hook that took another lock while
			// this one was held would be a deadlock waiting to happen.
			if b.onGrant != nil {
				b.onGrant(at, cost)
			}
			return nil
		}
		if err := b.sleepFor(ctx, wait); err != nil {
			return err
		}
	}
}

// reserve records the spend and reports when it was charged, or reports how
// long to wait for the oldest spend to age out of the window.
func (b *tokenBudget) reserve(cost int) (time.Time, time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.nowFunc()
	cutoff := now.Add(-b.window)

	kept := b.spends[:0]
	used := 0
	for _, s := range b.spends {
		if s.at.After(cutoff) {
			kept = append(kept, s)
			used += s.tokens
		}
	}
	b.spends = kept

	// Nothing outstanding: let an oversized request through rather than wait
	// for room that can never appear.
	if used+cost <= b.limit || used == 0 {
		b.spends = append(b.spends, tokenSpend{at: now, tokens: cost})
		return now, 0, true
	}

	wait := b.spends[0].at.Add(b.window).Sub(now)
	if wait < time.Millisecond {
		wait = time.Millisecond
	}
	return time.Time{}, wait, false
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// estimateTokens approximates the cost of a request. English averages close to
// four characters per token; the estimate only has to be good enough to pace
// against, and tokenBudgetHeadroom covers the error.
func estimateTokens(promptChars, maxOutputTokens int) int {
	return promptChars/4 + maxOutputTokens
}
