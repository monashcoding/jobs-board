// File: internal/unblocker/browser_manager.go

package unblocker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type BrowserProfile struct {
	ID string
	// Common browser properties
	UserAgent    string
	AcceptHeader string
	Language     string
	Platform     string
	// Screen and window properties
	ScreenWidth  int
	ScreenHeight int
	ColorDepth   int
	PixelDepth   int
	// Browser capabilities
	JavaEnabled   bool
	CookieEnabled bool
	// Session tracking
	LastUsed time.Time
}

type browserManager struct {
	profiles       []*BrowserProfile
	activeProfiles map[string]*BrowserProfile
	mu             sync.RWMutex
}

var commonUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
}

func newBrowserManager() BrowserProfileManager {
	manager := &browserManager{
		profiles:       make([]*BrowserProfile, 0),
		activeProfiles: make(map[string]*BrowserProfile),
	}

	// Initialize profiles
	for i := 0; i < 5; i++ {
		profile := manager.createProfile()
		manager.profiles = append(manager.profiles, profile)
	}

	utils.Log("Initialized browser manager with %d profiles", len(manager.profiles))
	return manager
}

func (m *browserManager) createProfile() *BrowserProfile {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	id := hex.EncodeToString(bytes)

	return &BrowserProfile{
		ID:            id,
		UserAgent:     commonUserAgents[time.Now().UnixNano()%int64(len(commonUserAgents))],
		AcceptHeader:  "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		Language:      "en-US,en;q=0.9",
		Platform:      "Windows",
		ScreenWidth:   1920,
		ScreenHeight:  1080,
		ColorDepth:    24,
		PixelDepth:    24,
		JavaEnabled:   true,
		CookieEnabled: true,
	}
}

func (m *browserManager) GetProfile(ctx context.Context) (*BrowserProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find least recently used profile
	var selectedProfile *BrowserProfile
	for _, profile := range m.profiles {
		if _, isActive := m.activeProfiles[profile.ID]; !isActive {
			if selectedProfile == nil || profile.LastUsed.Before(selectedProfile.LastUsed) {
				selectedProfile = profile
			}
		}
	}

	if selectedProfile == nil {
		utils.Error("No available browser profiles")
		return nil, fmt.Errorf("no available browser profiles")
	}

	selectedProfile.LastUsed = time.Now()
	m.activeProfiles[selectedProfile.ID] = selectedProfile

	utils.Log("Assigned browser profile: %s", selectedProfile.ID)
	return selectedProfile, nil
}

func (m *browserManager) ReleaseProfile(ctx context.Context, profileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.activeProfiles, profileID)
	utils.Log("Released browser profile: %s", profileID)
	return nil
}
