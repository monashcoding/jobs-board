package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// SendScraperLogsEmbed sends log entries to Discord as embeds, handling batching if needed
func SendScraperLogsEmbed(logs []LogEntry) error {
	// Get webhook URL from environment variable
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("DISCORD_WEBHOOK_URL is not set in .env file")
	}

	// Discord has limits on embed size
	const maxDescriptionLength = 4000
	const maxEmbedPerMessage = 10 // Discord allows up to 10 embeds per message

	// Batch the logs into chunks that fit within Discord's limits
	var batches []string
	var currentBatch strings.Builder
	currentBatch.WriteString("```\n") // Start with code block formatting

	for _, entry := range logs {
		logLine := fmt.Sprintf("%s\n",
			strings.TrimSpace(entry.Message))

		// If adding this line would exceed the description limit, start a new batch
		if currentBatch.Len()+len(logLine)+4 > maxDescriptionLength { // +4 for the closing ```
			currentBatch.WriteString("```")
			batches = append(batches, currentBatch.String())

			currentBatch.Reset()
			currentBatch.WriteString("```\n")
		}

		currentBatch.WriteString(logLine)
	}

	// Add the final batch if not empty
	if currentBatch.Len() > 4 { // More than just ```\n
		currentBatch.WriteString("```")
		batches = append(batches, currentBatch.String())
	}

	// Send batches as separate messages if needed
	for i := 0; i < len(batches); i += maxEmbedPerMessage {
		end := i + maxEmbedPerMessage
		if end > len(batches) {
			end = len(batches)
		}

		// Create embeds for this batch
		embeds := make([]map[string]interface{}, 0, end-i)

		for j := i; j < end; j++ {
			partNumber := ""
			if len(batches) > 1 {
				partNumber = fmt.Sprintf(" (Part %d/%d)", j+1, len(batches))
			}

			embeds = append(embeds, map[string]interface{}{
				"title":       "MPLOY Scraper Log" + partNumber,
				"description": batches[j],
				"color":       10181046, // Purple color
				"footer": map[string]interface{}{
					"text": "MPLOY Scraper v" + os.Getenv("VERSION"),
				},
				"timestamp": time.Now().Format(time.RFC3339),
			})
		}

		// Create payload with multiple embeds
		payload := map[string]interface{}{
			"embeds": embeds,
		}

		// Send this batch using the shared webhook function
		if err := sendDiscordWebhook(webhookURL, payload); err != nil {
			return err
		}

		// Rate limit our API calls to avoid Discord's rate limits
		if i+maxEmbedPerMessage < len(batches) {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

// SendScraperStartEmbed sends a Discord embed notification when the scraper starts
func SendScraperStartEmbed() error {
	// Get webhook URL from environment variable
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("DISCORD_WEBHOOK_URL is not set in .env file")
	}
	// Current time in RFC3339 format
	startTime := time.Now().Format(time.RFC3339)

	// Create embed payload
	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       "🚀 Scraper Started",
				"description": "Job scraping process has been initiated",
				"color":       3447003, // Blue color
				"fields": []map[string]interface{}{
					{
						"name":   "Start Time",
						"value":  startTime,
						"inline": true,
					},
				},
				"footer": map[string]interface{}{
					"text": "MPLOY Scraper v" + os.Getenv("VERSION"),
				},
				"timestamp": startTime,
			},
		},
	}

	return sendDiscordWebhook(webhookURL, payload)
}

// SendScraperCompleteEmbed sends a Discord embed notification when the scraper completes
func SendScraperCompleteEmbed(startTime time.Time, totalListings int) error {
	// Get webhook URL from environment variable
	webhookURL := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookURL == "" {
		return fmt.Errorf("DISCORD_WEBHOOK_URL is not set in .env file")
	}
	// Calculate duration
	duration := time.Since(startTime)
	durationString := fmt.Sprintf("%d minutes %d seconds",
		int(duration.Minutes()),
		int(duration.Seconds())%60)

	// Current time in RFC3339 format
	endTime := time.Now().Format(time.RFC3339)

	// Create embed payload
	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       "✅ Scraper Completed",
				"description": "Job scraping process has completed successfully",
				"color":       5763719, // Green color
				"fields": []map[string]interface{}{
					{
						"name":   "End Time",
						"value":  endTime,
						"inline": true,
					},
					{
						"name":   "Duration",
						"value":  durationString,
						"inline": true,
					},
					{
						"name":   "Total Listings",
						"value":  fmt.Sprintf("%d", totalListings),
						"inline": true,
					},
				},
				"footer": map[string]interface{}{
					"text": "MPLOY Scraper v" + os.Getenv("VERSION"),
				},
				"timestamp": endTime,
			},
		},
	}

	return sendDiscordWebhook(webhookURL, payload)
}

// sendDiscordWebhook sends a payload to a Discord webhook
func sendDiscordWebhook(webhookURL string, payload map[string]interface{}) error {
	// Marshal the payload to JSON
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
		return fmt.Errorf("failed to send webhook, status code: %d", resp.StatusCode)
	}
	return nil
}
