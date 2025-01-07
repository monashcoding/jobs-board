// File: internal/unblocker/rate_manager.go

package unblocker

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type rateManager struct {
	config RateLimitConfig
	mu     sync.RWMutex
}

func newRateManager(config RateLimitConfig) RateLimitManager {
	if config.BaseDelay == 0 {
		config.BaseDelay = time.Second * 2
	}
	if config.MaxJitter == 0 {
		config.MaxJitter = time.Second
	}

	return &rateManager{
		config: config,
	}
}

func (m *rateManager) Wait(ctx context.Context) (time.Duration, error) {
	delay := m.config.BaseDelay
	if m.config.MaxJitter > 0 {
		jitter := time.Duration(rand.Int63n(int64(m.config.MaxJitter)))
		delay += jitter
	}

	utils.Log("Rate limit: waiting for %v", delay)

	select {
	case <-ctx.Done():
		return delay, ctx.Err()
	case <-time.After(delay):
		return delay, nil
	}
}
