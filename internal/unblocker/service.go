// File: internal/unblocker/service.go

package unblocker

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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

	if s.config.Features.UseHeaderManagement {
		s.headerManager = newHeaderManager(s.config.Headers)
	}

	return nil
}

func (s *service) MakeRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	utils.Log("Making Request: %s %s", req.Method, req.URL.String())

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
	var referrer string
	if s.headerManager != nil {
		headers, err := s.headerManager.GetHeaders(ctx)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			if k == "Referer" && len(v) > 0 {
				referrer = v[0]
			}
			req.Header[k] = v
		}
		headerCount := len(headers)
		headerInfo := fmt.Sprintf("[HEADERS] %d in use", headerCount)
		if referrer != "" {
			headerInfo += fmt.Sprintf(", referral from %s", referrer)
		}
		unblockerInfo = append(unblockerInfo, headerInfo)
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

		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyUrlParsed),
		}
		s.client.Transport = transport
		unblockerInfo = append(unblockerInfo, fmt.Sprintf("[PROXY] Session ID %s", sessionID))
	}

	// Log concise unblocker info
	if len(unblockerInfo) > 0 {
		utils.Log("Unblockers: %s", strings.Join(unblockerInfo, ", "))
	}

	// Make the request
	utils.Log("Headers: %v", req.Header)

	resp, err := s.client.Do(req)
	if err != nil {
		if s.ipManager != nil {
			s.ipManager.ReportFailure(ctx, req.URL.String())
		}
		return nil, err
	}

	if resp.StatusCode >= 400 {
		utils.Error("Bad Response Code: %d", resp.StatusCode)
	}

	// Log response status
	utils.Log("Response [%d]: %s", resp.StatusCode, req.URL.Path)

	// Handle post-request tasks
	if s.ipManager != nil {
		s.ipManager.ReportSuccess(ctx, req.URL.String())
	}
	if s.rateManager != nil {
		s.rateManager.ReportRequest(ctx, req.URL.Host)
	}
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
