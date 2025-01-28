package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type AIParserStep struct{}

func NewAiParserStep() *AIParserStep {
	return &AIParserStep{}
}

func (s *AIParserStep) Name() string {
	return "AI_Parser"
}

func (s *AIParserStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	parsedListings := []models.Listing{} // process and insert listings

	for _, listing := range listings {
		listing, err := FixDescriptionFormat(listing)
		if err != nil {
			utils.Warn("Failed to Fix Description Format: %v", err)
		}

		// Append the normalised listing to the result slice
		parsedListings = append(parsedListings, listing)
	}

	return parsedListings, nil
}

type ChatRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	N int `json:"n"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func FixDescriptionFormat(listing models.Listing) (models.Listing, error) {
	utils.Debug("Sending description through AI assistant")

	// Skip descriptions with HTML tags already
	if !strings.HasPrefix(*listing.Description, "<") || !strings.HasPrefix(*listing.Description, "\u003c") {
		// Set endpoint and API key
		endpoint := os.Getenv("AZ_OPENAI_ENDPOINT")
		apiKey := os.Getenv("AZ_API_KEY")

		// Construct the request body
		chatRequest := ChatRequest{
			Messages: []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{
				{Role: "system", Content: "You are a helpful assistant."},
				{Role: "user", Content: "Fix the description by adding appropriate HTML tags. Only return the modified description as plain text without any additional words or explanation: " + *listing.Description},
			},
			N: 1,
		}

		// Convert the request to JSON
		requestBody, err := json.Marshal(chatRequest)
		if err != nil {
			utils.Error("Error marshalling request: %v", err)
		}

		// Make the HTTP request
		client := &http.Client{}
		req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
		if err != nil {
			utils.Error("Error creating request: %v", err)
		}

		// Set the necessary headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("api-key", apiKey)

		// Execute the request
		resp, err := client.Do(req)
		if err != nil {
			utils.Error("Error making request: %v", err)
		}
		// Log remaining tokens
		utils.Debug("Remaining Tokens: %s", resp.Header["X-Ratelimit-Remaining-Tokens"])
		defer resp.Body.Close()

		// Read and print the response
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			utils.Error("Error reading response: %v", err)
		}

		// Parse the response JSON
		var chatResponse ChatResponse
		err = json.Unmarshal(body, &chatResponse)
		if err != nil {
			utils.Error("Error unmarshalling response: %v", err)
		}

		// Output the assistant's response
		if len(chatResponse.Choices) > 0 {
			// utils.Debug("Original description:", *listing.Description)

			desc := strings.TrimPrefix(chatResponse.Choices[0].Message.Content, "```html")
			desc = strings.TrimSuffix(desc, "```")
			desc = strings.TrimSpace(desc)
			*listing.Description = desc
		} else {
			utils.Debug("No response received.")
		}
	}
	// Replace all /n
	*listing.Description = strings.ReplaceAll(*listing.Description, "\n", "")
	// utils.Debug("Assistant's Response:", *listing.Description)

	return listing, nil
}
