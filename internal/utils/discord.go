package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

// sendToDiscord sends a log message to the Discord webhook
func SendToDiscord(message string) error {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Get webhook URL from environment variable
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		log.Fatal("DISCORD_WEBHOOK_URL is not set in .env file")
	}

	// Prepare payload
	payload := map[string]string{"content": message}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Send POST request to Discord
	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for errors in response
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send log, status code: %d", resp.StatusCode)
	}
	return nil
}
