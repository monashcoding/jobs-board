// File: internal/unblocker/ip_manager.go

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

// BrightDataConfig holds the configuration for Bright Data proxies
type BrightDataConfig struct {
	CustomerID  string
	ZoneName    string
	Password    string
	Country     string // Optional country code (e.g., "us", "uk")
	State       string // Optional US state code
	City        string // Optional city name
	UseLocalDNS bool   // Whether to use local DNS resolution
}

type ipManager struct {
	config        *BrightDataConfig
	activeSession string
	sessionExpiry time.Time
	mu            sync.RWMutex
}

// newIPRotationManager creates a new Bright Data IP manager
func newIPRotationManager(config *BrightDataConfig) IPRotationManager {
	if config == nil {
		utils.Error("No Bright Data configuration provided")
		return nil
	}

	if config.CustomerID == "" || config.ZoneName == "" || config.Password == "" {
		utils.Error("Missing required Bright Data credentials")
		return nil
	}

	utils.Log("Initializing Bright Data IP manager for zone: %s", config.ZoneName)
	return &ipManager{
		config: config,
	}
}

// generateSessionID creates a random session ID
func generateSessionID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// constructUsername builds the Bright Data proxy username with all parameters
func (m *ipManager) constructUsername(session string) string {
	// Base username with required parameters
	username := fmt.Sprintf("brd-customer-%s-zone-%s-session-%s",
		m.config.CustomerID,
		m.config.ZoneName,
		session)

	// Add optional parameters if configured
	if m.config.Country != "" {
		username += fmt.Sprintf("-country-%s", m.config.Country)
	}

	if m.config.State != "" && m.config.Country == "us" {
		username += fmt.Sprintf("-state-%s", m.config.State)
	}

	if m.config.City != "" {
		username += fmt.Sprintf("-city-%s", m.config.City)
	}

	if m.config.UseLocalDNS {
		username += "-dns-local"
	}

	return username
}

// GetProxy returns a Bright Data proxy URL with a valid session
func (m *ipManager) GetProxy(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create new session if needed
	if m.activeSession == "" || time.Now().After(m.sessionExpiry) {
		m.activeSession = generateSessionID()
		m.sessionExpiry = time.Now().Add(time.Minute * 10) // 10-minute sessions
		utils.Log("Created new Bright Data session: %s", m.activeSession)
	}

	username := m.constructUsername(m.activeSession)
	proxyURL := fmt.Sprintf("http://%s:%s@brd.superproxy.io:22225",
		username,
		m.config.Password)

	utils.Log("Using Bright Data proxy with session: %s", m.activeSession)
	return proxyURL, nil
}

// ReportSuccess logs successful proxy usage
func (m *ipManager) ReportSuccess(ctx context.Context, proxyURL string) error {
	utils.Log("Successful request with session: %s", m.activeSession)
	return nil
}

// ReportFailure handles proxy failures by invalidating the current session
func (m *ipManager) ReportFailure(ctx context.Context, proxyURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	utils.Log("Failed request with session: %s, rotating session", m.activeSession)
	m.activeSession = "" // Force new session on next GetProxy call
	return nil
}
