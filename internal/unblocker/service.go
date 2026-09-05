// File: internal/unblocker/service.go

package unblocker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type service struct {
	config         Config
	browserManager BrowserProfileManager
	cookieManager  CookieManager
	ipManager      IPRotationManager
	rateManager    RateLimitManager
	headerManager  HeaderManager
	client         *http.Client
}

// requestTimeout bounds a single fetch, headers and body together.
//
// The client used to have no timeout at all and the requests never carried a
// context, so a proxied fetch that stopped responding hung the whole scrape
// with nothing able to interrupt it -- not the pipeline's cancel, not SIGTERM.
// A minute is far longer than any job page needs and still lets a run finish.
const requestTimeout = 60 * time.Second

// proxyContextKey carries the proxy chosen for one request to the shared
// transport's Proxy hook.
type proxyContextKey struct{}

// NewService creates a new unblocker service
func NewService(config Config) (Service, error) {
	// One transport for the whole service, not one per request.
	//
	// Each request used to build its own http.Transport and assign it to the
	// shared client. A transport owns a connection pool and the goroutines
	// serving it, and nothing ever closed the old ones, so a long scrape left
	// hundreds of them alive holding idle proxy connections -- leaking memory,
	// file descriptors and goroutines the further into a run it got. Resolving
	// the proxy per request instead keeps one pool, which is also the only way
	// the idle limits below mean anything.
	transport := &http.Transport{
		Proxy: func(r *http.Request) (*url.URL, error) {
			proxyURL, _ := r.Context().Value(proxyContextKey{}).(*url.URL)
			return proxyURL, nil
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	s := &service{
		config: config,
		client: &http.Client{
			Timeout:   requestTimeout,
			Transport: transport,
		},
	}

	// Initialize enabled features
	if err := s.initializeFeatures(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *service) initializeFeatures() error {
	if s.config.Features.UseBrowserProfiles {
		s.browserManager = newBrowserManager()
	}

	if s.config.Features.UseCookieManagement {
		s.cookieManager = newCookieManager()
	}

	if s.config.Features.UseIPRotation {
		s.ipManager = newIPRotationManager(s.config.BrightData)
	}

	if s.config.Features.UseRateLimiting {
		s.rateManager = newRateManager(*s.config.Rate)
	}

	if s.config.Features.UseHeaderManagement {
		s.headerManager = newHeaderManager(s.config.Headers)
	}

	return nil
}

func (s *service) MakeRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Use default retry config if none provided
	retryConfig := s.config.Retry
	if retryConfig == nil {
		retryConfig = DefaultRetryConfig()
	}

	var lastError error
	var currentBackoff time.Duration

	// Try the request up to MaxRetries times
	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff duration
			currentBackoff = retryConfig.calculateBackoff(attempt-1, currentBackoff)
			utils.Log("Retry attempt %d/%d after %v delay", attempt, retryConfig.MaxRetries, currentBackoff)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(currentBackoff):
				// Continue with retry
			}
		}

		// Prepare and execute request
		resp, err := s.executeRequest(ctx, req)
		if err != nil {
			lastError = fmt.Errorf("attempt %d failed: %w", attempt, err)
			// Don't try to access resp if it's nil
			if resp != nil {
				utils.Error("[CODE %d] Error fetching %s - %v", resp.StatusCode, req.URL.Path, err)
			} else {
				utils.Error("Error fetching %s - %v", req.URL.Path, err)
			}
			s.rotateProxy(ctx)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			utils.Success("Success fetching %s", req.URL.Path)
		} else {
			utils.Error("[CODE %d] Error fetching %s", resp.StatusCode, req.URL.Path)
		}

		// Check if we should retry based on response
		if retryConfig.shouldRetry(resp) {
			lastError = fmt.Errorf("attempt %d failed with status %d", attempt, resp.StatusCode)

			// Use Retry-After header if available
			if retryAfter := getRetryAfterDuration(resp); retryAfter > 0 {
				currentBackoff = retryAfter
				utils.Log("Using Retry-After header value: %v", currentBackoff)
			}

			resp.Body.Close()
			s.rotateProxy(ctx)
			continue
		}

		// Success! Return the response
		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %v", lastError)
}

// rotateProxy discards the current Bright Data session so the next attempt
// goes out through a different exit IP. Without this, every retry reuses the
// IP that just got blocked and the backoff is spent waiting for a result that
// cannot change.
func (s *service) rotateProxy(ctx context.Context) {
	if s.ipManager == nil {
		return
	}

	if err := s.ipManager.RotateProxy(ctx, ""); err != nil {
		utils.Warn("Failed to rotate proxy session: %v", err)
	}
}

// executeRequest handles the actual execution of the request with all middleware
func (s *service) executeRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	utils.Log("Making Request: %s %s", req.Method, req.URL.String())

	// Bind the caller's context to the request here rather than at each of the
	// scrapers' call sites, so cancelling the pipeline aborts whatever is in
	// flight and any scraper added later inherits that for free.
	req = req.WithContext(ctx)

	var unblockerInfo []string

	// Apply rate limiting if enabled
	if s.rateManager != nil {
		delay, err := s.rateManager.Wait(ctx)
		if err != nil {
			return nil, err
		}
		unblockerInfo = append(unblockerInfo, fmt.Sprintf("[RATE] Waited %.2fs", delay.Seconds()))
	}

	// Get and apply browser profile if enabled
	var profileID string
	if s.browserManager != nil {
		profile, err := s.browserManager.GetProfile(ctx)
		if err != nil {
			return nil, err
		}
		profileID = profile.ID
		req.Header.Set("User-Agent", profile.UserAgent)
		unblockerInfo = append(unblockerInfo, fmt.Sprintf("[BROWSER] Profile ID %s", profileID))
	}

	// Apply cookies if enabled
	if s.cookieManager != nil {
		cookies, err := s.cookieManager.GetCookies(ctx, req.URL.Host)
		if err != nil {
			return nil, err
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		unblockerInfo = append(unblockerInfo, fmt.Sprintf("[COOKIES] %d in use", len(cookies)))
	}

	// Apply headers if enabled
	if s.headerManager != nil {
		headers, err := s.headerManager.GetHeaders(ctx)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header[k] = v
		}
		unblockerInfo = append(unblockerInfo, fmt.Sprintf("[HEADERS] %d in use", len(headers)))
	}

	// Configure proxy if enabled
	var sessionID string
	if s.ipManager != nil {
		proxyURL, err := s.ipManager.GetProxy(ctx)
		if err != nil {
			return nil, err
		}

		proxyUrlParsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}

		// Extract session ID from proxy URL
		if parts := strings.Split(proxyUrlParsed.User.Username(), "-session-"); len(parts) > 1 {
			sessionID = strings.Split(parts[1], "-")[0]
		}

		// The shared transport reads this back out in its Proxy hook.
		req = req.WithContext(context.WithValue(req.Context(), proxyContextKey{}, proxyUrlParsed))
		unblockerInfo = append(unblockerInfo, fmt.Sprintf("[PROXY] Session ID %s", sessionID))
	}

	// Log concise unblocker info
	if len(unblockerInfo) > 0 {
		utils.Log("Unblockers: %s", strings.Join(unblockerInfo, ", "))
	}

	// Make the request
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	// Handle post-request tasks
	if s.cookieManager != nil {
		s.cookieManager.SetCookies(ctx, req.URL.Host, resp.Cookies())
	}
	if s.browserManager != nil && profileID != "" {
		s.browserManager.ReleaseProfile(ctx, profileID)
	}
	if s.headerManager != nil {
		s.headerManager.UpdateHeaders(ctx, resp.Header)
	}

	return resp, nil
}
