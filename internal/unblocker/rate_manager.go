// File: internal/unblocker/rate_manager.go

package unblocker

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type RateLimitConfig struct {
	RequestsPerMinute int
	BurstSize         int
	BaseDelay         time.Duration
	MaxJitter         time.Duration
}

type domainStats struct {
	lastRequest time.Time
	requests    int
	window      time.Time
}

type rateManager struct {
	config RateLimitConfig
	stats  map[string]*domainStats
	mu     sync.RWMutex
}

func newRateManager(config RateLimitConfig) RateLimitManager {
	if config.RequestsPerMinute == 0 {
		config.RequestsPerMinute = 30 // Default to 30 requests per minute
	}
	if config.BaseDelay == 0 {
		config.BaseDelay = time.Second * 2
	}
	if config.MaxJitter == 0 {
		config.MaxJitter = time.Second
	}

	return &rateManager{
		config: config,
		stats:  make(map[string]*domainStats),
	}
}

func (m *rateManager) Wait(ctx context.Context) error {
	delay := m.config.BaseDelay
	if m.config.MaxJitter > 0 {
		jitter := time.Duration(rand.Int63n(int64(m.config.MaxJitter)))
		delay += jitter
	}

	utils.Log("Rate limit: waiting for %v", delay)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func (m *rateManager) ReportRequest(ctx context.Context, domain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	stats, exists := m.stats[domain]
	if !exists {
		stats = &domainStats{
			window: now,
		}
		m.stats[domain] = stats
	}

	// Reset window if needed
	if now.Sub(stats.window) >= time.Minute {
		stats.window = now
		stats.requests = 0
	}

	// Update stats
	stats.requests++
	stats.lastRequest = now

	if stats.requests > m.config.RequestsPerMinute {
		utils.Error("Rate limit exceeded for domain: %s", domain)
	} else {
		utils.Log("Request recorded for domain: %s (%d/%d this minute)",
			domain, stats.requests, m.config.RequestsPerMinute)
	}

	return nil
}
