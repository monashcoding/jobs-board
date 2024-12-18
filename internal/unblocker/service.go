// File: internal/unblocker/service.go

package unblocker

import (
	"context"
	"net/http"
	"net/url"

	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type service struct {
	config         Config
	browserManager BrowserProfileManager
	cookieManager  CookieManager
	ipManager      IPRotationManager
	rateManager    RateLimitManager
	client         *http.Client
}

// NewService creates a new unblocker service
func NewService(config Config) (Service, error) {
	s := &service{
		config: config,
		client: &http.Client{},
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

	return nil
}

func (s *service) MakeRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Apply rate limiting if enabled
	if s.rateManager != nil {
		if err := s.rateManager.Wait(ctx); err != nil {
			return nil, err
		}
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
		// Add other profile headers...
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
	}

	// Configure proxy if enabled
	if s.ipManager != nil {
		proxyURL, err := s.ipManager.GetProxy(ctx)
		if err != nil {
			return nil, err
		}

		proxyUrlParsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}

		// Create a new transport with the proxy
		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyUrlParsed),
		}

		s.client.Transport = transport
	}

	// Make the request
	resp, err := s.client.Do(req)
	if err != nil {
		if s.ipManager != nil {
			s.ipManager.ReportFailure(ctx, req.URL.String())
		}
		return nil, err
	}

	// Report success and update state
	if s.ipManager != nil {
		s.ipManager.ReportSuccess(ctx, req.URL.String())
	}

	if s.rateManager != nil {
		s.rateManager.ReportRequest(ctx, req.URL.Host)
	}

	// Save cookies from response if enabled
	if s.cookieManager != nil {
		s.cookieManager.SetCookies(ctx, req.URL.Host, resp.Cookies())
	}

	// Release browser profile if used
	if s.browserManager != nil && profileID != "" {
		s.browserManager.ReleaseProfile(ctx, profileID)
	}

	utils.Log("Request completed successfully")
	return resp, nil
}
