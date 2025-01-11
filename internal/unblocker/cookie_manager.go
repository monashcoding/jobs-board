// File: internal/unblocker/cookie_manager.go

package unblocker

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type CookieRule struct {
	Required []string // Cookies that must be present
	Ignored  []string // Cookies that should be ignored
	MaxAge   time.Duration
}

type cookieManager struct {
	cookies map[string][]*http.Cookie // domain -> cookies
	rules   map[string]CookieRule     // domain -> rules
	mu      sync.RWMutex
}

func newCookieManager() CookieManager {
	return &cookieManager{
		cookies: make(map[string][]*http.Cookie),
		rules:   make(map[string]CookieRule),
	}
}

func (m *cookieManager) GetCookies(ctx context.Context, domain string) ([]*http.Cookie, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cookies := m.cookies[domain]
	utils.Log("Retrieved %d cookies for domain: %s", len(cookies), domain)
	return cookies, nil
}

func (m *cookieManager) SetCookies(ctx context.Context, domain string, cookies []*http.Cookie) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Apply domain rules if they exist
	if rule, exists := m.rules[domain]; exists {
		cookies = m.applyCookieRules(cookies, rule)
	}

	// Remove expired cookies
	var validCookies []*http.Cookie
	now := time.Now()
	for _, cookie := range cookies {
		if cookie.Expires.IsZero() || cookie.Expires.After(now) {
			validCookies = append(validCookies, cookie)
		}
	}

	m.cookies[domain] = validCookies
	utils.Log("Stored %d cookies for domain: %s", len(validCookies), domain)
	return nil
}

func (m *cookieManager) applyCookieRules(cookies []*http.Cookie, rule CookieRule) []*http.Cookie {
	var filteredCookies []*http.Cookie

	// Create map of ignored cookies for faster lookup
	ignored := make(map[string]bool)
	for _, name := range rule.Ignored {
		ignored[name] = true
	}

	for _, cookie := range cookies {
		if !ignored[cookie.Name] {
			// Apply max age if specified
			if rule.MaxAge > 0 {
				cookie.Expires = time.Now().Add(rule.MaxAge)
			}
			filteredCookies = append(filteredCookies, cookie)
		}
	}

	return filteredCookies
}
