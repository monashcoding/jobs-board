// File: internal/unblocker/header_manager.go

package unblocker

import (
	"context"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type headerManager struct {
	config        *HeaderConfig
	referrals     []string
	customHeaders map[string]string
	mu            sync.RWMutex
	lastRotation  time.Time
}

func newHeaderManager(config *HeaderConfig) HeaderManager {
	if config == nil {
		config = &HeaderConfig{
			ReferralSites: []string{
				"https://www.google.com",
				"https://www.bing.com",
				"https://www.linkedin.com",
			},
			CustomHeaders: make(map[string]string),
		}
	}

	utils.Log("Initializing header manager with %d referral sites", len(config.ReferralSites))

	return &headerManager{
		config:        config,
		referrals:     config.ReferralSites,
		customHeaders: config.CustomHeaders,
		lastRotation:  time.Now(),
	}
}

func (h *headerManager) GetHeaders(ctx context.Context) (http.Header, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	headers := make(http.Header)

	// Add standard browser headers
	headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	headers.Set("Accept-Language", "en-US,en;q=0.9")
	headers.Set("Accept-Encoding", "gzip, deflate, br")
	headers.Set("Connection", "keep-alive")
	headers.Set("DNT", "1")
	headers.Set("Upgrade-Insecure-Requests", "1")
	headers.Set("Sec-Fetch-Dest", "document")
	headers.Set("Sec-Fetch-Mode", "navigate")
	headers.Set("Sec-Fetch-Site", "none")
	headers.Set("Sec-Fetch-User", "?1")

	// Add custom headers
	for k, v := range h.customHeaders {
		headers.Set(k, v)
	}

	// Add referral if configured
	if len(h.referrals) > 0 {
		referral := h.referrals[rand.Intn(len(h.referrals))]
		headers.Set("Referer", referral)
		utils.Log("Using referrer: %s", referral)
	}

	utils.Log("Generated headers with %d custom values", len(h.customHeaders))
	return headers, nil
}

func (h *headerManager) UpdateHeaders(ctx context.Context, newHeaders http.Header) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	utils.Log("Updating custom headers")

	// Update any dynamic headers (e.g., required tokens or session IDs)
	for k, v := range newHeaders {
		if len(v) > 0 {
			h.customHeaders[k] = v[0]
		}
	}

	return nil
}
