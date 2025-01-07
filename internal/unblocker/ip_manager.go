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

	utils.Success("Started Bright Data IP manager for zone: %s", config.ZoneName)
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
	// Each unique session ID will get a unique IP, can be used to target the same IP repeatedly or force rotation.
	// Setting a custom session ID helps rotate IPs.
	// Reference: https://docs.brightdata.com/proxy-networks/config-options
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
	}

	username := m.constructUsername(m.activeSession)
	proxyURL := fmt.Sprintf("http://%s:%s@brd.superproxy.io:33335",
		username,
		m.config.Password)

	return proxyURL, nil
}

// RotateProxy handles proxy failures by invalidating the current session
func (m *ipManager) RotateProxy(ctx context.Context, proxyURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	utils.Log("Rotating session: %s", m.activeSession)
	m.activeSession = "" // Force new session on next GetProxy call
	return nil
}
