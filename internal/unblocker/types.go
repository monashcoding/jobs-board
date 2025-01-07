// File: internal/unblocker/types.go

package unblocker

import (
	"context"
	"net/http"
	"time"
)

// Features represents which anti-detection mechanisms to use
type Features struct {
	UseBrowserProfiles  bool
	UseHeaderManagement bool
	UseCookieManagement bool
	UseIPRotation       bool
	UseRateLimiting     bool
}

// All configuration types
type Config struct {
	Features   Features
	BrightData *BrightDataConfig
	Browser    *BrowserConfig
	Rate       *RateLimitConfig
	Headers    *HeaderConfig
	Cookies    *CookieConfig
	Retry      *RetryConfig
}

// BrightDataConfig holds the configuration for Bright Data proxies
// Controls proxy behavior and location targeting using BrightData's proxy network.
type BrightDataConfig struct {
	CustomerID  string // Your BrightData customer ID (e.g. hl_29d8sj2d)
	ZoneName    string // Your BrightData zone name (e.g. datacenter_proxy1)
	Password    string // Your BrightData password
	Country     string // Optional: Target country (e.g., "us", "uk")
	State       string // Optional: US state code (only when Country="us")
	City        string // Optional: Target city
	UseLocalDNS bool   // Whether to use local DNS resolution
}

// BrowserConfig manages browser fingerprinting and rotation strategies. Profile settings include:
// - User agents
// - Screen resolutions
// - Browser plugins
// - Language preferences
// - Platform information
type BrowserConfig struct {
	Profiles         []BrowserProfile // List of browser profiles to rotate
	RotationStrategy string           // Strategy for profile rotation
	RotationInterval time.Duration    // How often to rotate profiles
}

// HeaderConfig Controls HTTP headers and referrer behavior.
type HeaderConfig struct {
	ReferralSites []string          // List of referrer URLs to rotate
	CustomHeaders map[string]string // Custom headers to include
}

// CookieConfig Handles cookie persistence and rules. Cookie rules can:
// - Specify required cookies
// - Define ignored cookies
// - Set cookie expiration rules
// - Handle domain-specific requirements
type CookieConfig struct {
	PersistCookies bool                  // Whether to maintain cookies between sessions
	DomainRules    map[string]CookieRule // Domain-specific cookie handling rules
}

// RateLimitConfig controls request timing and frequency. Common configurations:
// - Aggressive: 60 rpm, 5s base delay, 2s jitter
// - Moderate: 30 rpm, 10s base delay, 5s jitter
// - Conservative: 10 rpm, 30s base delay, 10s jitter
type RateLimitConfig struct {
	RequestsPerMinute int           // Maximum requests per minute
	BurstSize         int           // Maximum burst size for requests
	BaseDelay         time.Duration // Minimum delay between requests
	MaxJitter         time.Duration // Random delay addition (0 to MaxJitter)
}

// Feature interfaces
type BrowserProfileManager interface {
	GetProfile(ctx context.Context) (*BrowserProfile, error)
	ReleaseProfile(ctx context.Context, profileID string) error
}

type HeaderManager interface {
	GetHeaders(ctx context.Context) (http.Header, error)
	UpdateHeaders(ctx context.Context, headers http.Header) error
}

type CookieManager interface {
	GetCookies(ctx context.Context, domain string) ([]*http.Cookie, error)
	SetCookies(ctx context.Context, domain string, cookies []*http.Cookie) error
}

type IPRotationManager interface {
	GetProxy(ctx context.Context) (string, error)
	ReportSuccess(ctx context.Context, proxyURL string) error
	ReportFailure(ctx context.Context, proxyURL string) error
}

type RateLimitManager interface {
	Wait(ctx context.Context) (time.Duration, error)
	ReportRequest(ctx context.Context, domain string) error
}

// Service defines the main interface for the unblocker service
type Service interface {
	MakeRequest(ctx context.Context, req *http.Request) (*http.Response, error)
}
