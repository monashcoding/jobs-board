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

// Config holds all configuration for the unblocker service
type Config struct {
	Features   Features
	BrightData *BrightDataConfig // From your existing ip_manager.go
	Browser    *BrowserConfig    // From your existing browser_manager.go
	Rate       *RateLimitConfig  // From your existing rate_manager.go
}

// Service interface defines the main methods for the unblocker
type Service interface {
	MakeRequest(ctx context.Context, req *http.Request) (*http.Response, error)
}

// Feature interfaces are defined here for easy reference
type BrowserProfileManager interface {
	GetProfile(ctx context.Context) (*BrowserProfile, error)
	ReleaseProfile(ctx context.Context, profileID string) error
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
	Wait(ctx context.Context) error
	ReportRequest(ctx context.Context, domain string) error
}
