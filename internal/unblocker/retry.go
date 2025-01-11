package unblocker

import (
	"net/http"
	"time"
)

// RetryConfig defines the configuration for retry behavior
type RetryConfig struct {
	MaxRetries      int           // Maximum number of retry attempts
	InitialBackoff  time.Duration // Initial backoff duration
	MaxBackoff      time.Duration // Maximum backoff duration
	BackoffFactor   float64       // Factor to multiply backoff by after each attempt
	RetryableStatus []int         // HTTP status codes that should trigger a retry
}

// DefaultRetryConfig provides sensible default retry settings
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 5 * time.Second,
		MaxBackoff:     2 * time.Minute,
		BackoffFactor:  2.0,
		RetryableStatus: []int{
			http.StatusTooManyRequests,     // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout,      // 504
		},
	}
}

// shouldRetry determines if a request should be retried based on the response
func (c *RetryConfig) shouldRetry(resp *http.Response) bool {
	if resp == nil {
		return true // Retry on network errors (nil response)
	}

	for _, status := range c.RetryableStatus {
		if resp.StatusCode == status {
			return true
		}
	}
	return false
}

// calculateBackoff determines the next backoff duration
func (c *RetryConfig) calculateBackoff(attempt int, currentBackoff time.Duration) time.Duration {
	// If this is the first attempt, use initial backoff
	if attempt == 0 {
		return c.InitialBackoff
	}

	// Calculate next backoff with exponential increase
	next := time.Duration(float64(currentBackoff) * c.BackoffFactor)

	// Respect maximum backoff
	if next > c.MaxBackoff {
		return c.MaxBackoff
	}

	return next
}

// getRetryAfterDuration extracts and parses the Retry-After header
func getRetryAfterDuration(resp *http.Response) time.Duration {
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if seconds, err := time.ParseDuration(retryAfter + "s"); err == nil {
			return seconds
		}
	}
	return 0
}
