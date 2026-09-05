// File: internal/utils/brightdata.go
package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// BrightDataWhitelistRequest is the payload for the BrightData whitelist API
type BrightDataWhitelistRequest struct {
	Zone string `json:"zone"`
	IP   string `json:"ip"`
}

// BrightDataWhitelistResponse is the response from the BrightData whitelist API
type BrightDataWhitelistResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// WhitelistCurrentIP calls the BrightData API to whitelist the current IP
func WhitelistCurrentIP() error {
	// Get environment variables
	apiToken := os.Getenv("BRIGHTDATA_API_TOKEN")
	zoneName := os.Getenv("BRIGHTDATA_ZONE_NAME")

	// Validate required env vars
	if apiToken == "" {
		return fmt.Errorf("BRIGHTDATA_API_TOKEN is not set")
	}
	if zoneName == "" {
		return fmt.Errorf("BRIGHTDATA_ZONE_NAME is not set")
	}

	// Get public IP using external API services
	publicIP, err := GetPublicIP()
	if err != nil {
		return fmt.Errorf("failed to get public IP: %w", err)
	}
	Log("Using public IP: %s for BrightData whitelist", publicIP)

	// Create request payload
	requestData := BrightDataWhitelistRequest{
		Zone: zoneName,
		IP:   publicIP,
	}

	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("failed to marshal request data: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest(
		"POST",
		"https://api.brightdata.com/zone/whitelist",
		bytes.NewBuffer(requestBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	// Make request. The timeout matters: this runs before the pipeline starts
	// and Fatals on failure, so an unbounded call here hangs startup outright
	// with nothing logged after it.
	client := &http.Client{Timeout: brightDataTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	Success("Successfully whitelisted public IP %s for BrightData zone %s", publicIP, zoneName)
	return nil
}

// GetPublicIP retrieves the public IP address using a third-party API
// brightDataTimeout bounds the calls made before the pipeline starts. These ran
// on clients with no timeout, so a provider that accepted the connection and
// then went quiet hung the scraper at startup indefinitely.
const brightDataTimeout = 15 * time.Second

// ipLookupClient is bounded so one unresponsive provider cannot stop the loop
// below from trying the next one -- the whole point of having four.
var ipLookupClient = &http.Client{Timeout: brightDataTimeout}

func GetPublicIP() (string, error) {
	// List of IP API services to try
	ipAPIs := []string{
		"https://api.ipify.org",
		"https://ipinfo.io/ip",
		"https://api.ip.sb/ip",
		"https://icanhazip.com",
	}

	var lastErr error
	for _, api := range ipAPIs {
		resp, err := ipLookupClient.Get(api)
		if err != nil {
			lastErr = err
			Log("Failed to get IP from %s: %v", api, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("received non-200 response code: %d", resp.StatusCode)
			Log("Failed to get IP from %s: %v", api, lastErr)
			continue
		}

		// Read the response body which should contain just the IP address
		ipBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			Log("Failed to read response from %s: %v", api, err)
			continue
		}

		// Convert to string and trim any whitespace
		ip := strings.TrimSpace(string(ipBytes))

		// Validate that we got a proper IP address
		if net.ParseIP(ip) != nil {
			return ip, nil
		}

		lastErr = fmt.Errorf("received invalid IP: %s", ip)
		Log("Invalid IP from %s: %s", api, ip)
	}

	return "", fmt.Errorf("failed to get public IP from any API service: %w", lastErr)
}
