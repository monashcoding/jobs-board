package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type AIParserStep struct {
	completedJobs int32         // Atomic counter for completed jobs
	totalJobs     int32         // Total number of jobs to process
	semaphore     chan struct{} // Semaphore to limit concurrent API requests
	maxConcurrent int           // Maximum concurrent requests
	sleepDuration time.Duration // How long to sleep between API calls
}

func NewAiParserStep() *AIParserStep {
	maxConcurrent := 20 // Allow 20 concurrent API calls
	return &AIParserStep{
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
		sleepDuration: 1 * time.Second, // Sleep 1 seconds between API calls
	}
}

func (s *AIParserStep) Name() string {
	return "AI_Parser"
}

func (s *AIParserStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	parsedListings := make([]models.Listing, len(listings)) // Preallocate slice
	s.totalJobs = int32(len(listings) * 3)                  // Each listing has 3 AI operations
	s.completedJobs = 0                                     // Reset counter

	utils.Log("Running %d listings (%d operations) through AI Parser.", len(listings), s.totalJobs)

	utils.Log("Max Concurrent Requests: %d | Sleep: %s between requests",
		s.maxConcurrent, s.sleepDuration.String())

	var wg sync.WaitGroup
	errCh := make(chan error, len(listings)) // Buffered channel for errors

	for i, listing := range listings {
		wg.Add(1)

		// Process each listing in a separate Goroutine
		go func(i int, listing models.Listing) {
			defer wg.Done()

			// Fix description
			updatedListing, err := s.FixDescriptionFormat(ctx, listing)
			if err != nil {
				utils.Warn("Failed to fix description format: %v", err)
				errCh <- err
			}

			// Get industry field
			updatedListing, err = s.GetIndustryFieldandType(ctx, updatedListing)
			if err != nil {
				utils.Warn("Failed to get industry field: %v", err)
				errCh <- err
			}

			// Generate one liner description
			updatedListing, err = s.GenerateOneLinerDesc(ctx, updatedListing)
			if err != nil {
				utils.Warn("Failed to generate one liner description: %v", err)
				errCh <- err
			}

			// Store the processed listing in the correct index
			parsedListings[i] = updatedListing
		}(i, listing)
	}
	// Wait for all Goroutines to complete
	wg.Wait()
	close(errCh)

	// Collect errors
	var finalErr error
	for err := range errCh {
		if finalErr == nil {
			finalErr = err
		} else {
			finalErr = fmt.Errorf("%v; %v", finalErr, err)
		}
	}

	return parsedListings, finalErr
}

// acquireSemaphore acquires a slot in the semaphore, blocking if necessary
func (s *AIParserStep) acquireSemaphore(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.semaphore <- struct{}{}:
		return nil
	}
}

// releaseSemaphore releases a slot in the semaphore
func (s *AIParserStep) releaseSemaphore() {
	time.Sleep(s.sleepDuration)
	<-s.semaphore
}

type ChatRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (s *AIParserStep) FixDescriptionFormat(ctx context.Context, listing models.Listing) (models.Listing, error) {
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
			{Role: "user", Content: "Fix the description by adding appropriate HTML tags, for headings use <h3>. Please remove any weird symbols like \\xa0 and \\u202f. Only return the modified description as plain text without any additional words or explanation: " + *listing.Description},
		},
	}

	// Make the request
	chatResponse, err := s.makeChatRequest(ctx, endpoint, apiKey, chatRequest, "FixDescription")
	if err != nil {
		return listing, err
	}

	// Output the assistant's response
	if len(chatResponse.Choices) > 0 {
		desc := strings.TrimPrefix(chatResponse.Choices[0].Message.Content, "```html")
		desc = strings.TrimSuffix(desc, "```")
		desc = strings.TrimSpace(desc)
		*listing.Description = desc
	} else {
		utils.Error("No response received.")
	}

	// Replace all breaks
	*listing.Description = strings.ReplaceAll(*listing.Description, "\\n", "")

	return listing, nil
}

func (s *AIParserStep) GetIndustryFieldandType(ctx context.Context, listing models.Listing) (models.Listing, error) {
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
			{Role: "user", Content: "Classify the industry field of the listing based on the title and company name. Respond with only one of the following values: CONSULTING, BANKS, BIG_TECH, TECH, QUANT_TRADING, ENERGY, GOVERNMENT, MANUFACTURING, TELECOMMUNICATIONS, OTHER_INDUSTRY followed by whether it is an INTERN, GRADUATE or OTHER job type. Output in the form: INDUSTRY#JOBTYPE \n\nTitle: " + listing.Title + "\nCompany: " + listing.Company.Name},
		},
	}

	// Make the request
	chatResponse, err := s.makeChatRequest(ctx, endpoint, apiKey, chatRequest, "GetIndustryField")
	if err != nil {
		return listing, err
	}

	// Output the assistant's response
	if len(chatResponse.Choices) > 0 {
		splitResponse := strings.Split(chatResponse.Choices[0].Message.Content, "#")

		industryField := "OTHER_INDUSTRY"
		jobType := "OTHER"

		if len(splitResponse) >= 2 {
			industryField = strings.TrimSpace(splitResponse[0])
			jobType = strings.TrimSpace(splitResponse[1])
		} else {
			utils.Warn("Unexpected format in AI response: %q. Falling back to default values.", chatResponse)
		}

		// Define a map for quick lookup
		industryMap := map[string]models.IndustryField{
			"CONSULTING":         models.CONSULTING,
			"BANKS":              models.BANKS,
			"BIG_TECH":           models.BIG_TECH,
			"TECH":               models.TECH,
			"QUANT_TRADING":      models.QUANT_TRADING,
			"ENERGY":             models.ENERGY,
			"GOVERNMENT":         models.GOVERNMENT,
			"MANUFACTURING":      models.MANUFACTURING,
			"TELECOMMUNICATIONS": models.TELECOMMUNICATIONS,
			"OTHER_INDUSTRY":     models.OTHER_INDUSTRY,
		}

		// Lookup the industry field
		if listing.IndustryField == "" {
			listing.IndustryField = industryMap[industryField]
		}

		// Create a map for job types
		jobTypeMap := map[string]models.JobType{
			"INTERN":   models.INTERN,
			"GRADUATE": models.GRADUATE,
			"OTHER":    models.OTHER,
		}

		// If job type is nil, then set jobtype
		if listing.Type == nil {
			jobTypeValue := jobTypeMap[jobType]
			listing.Type = &jobTypeValue
		}

	} else {
		utils.Error("No response received from Chat.")
	}

	return listing, nil
}

func (s *AIParserStep) makeChatRequest(ctx context.Context, endpoint string, apiKey string, chatRequest ChatRequest, operationType string) (ChatResponse, error) {
	var chatResponse ChatResponse

	// Acquire a slot in the semaphore before making the request
	if err := s.acquireSemaphore(ctx); err != nil {
		return chatResponse, fmt.Errorf("failed to acquire semaphore: %w", err)
	}
	// Release the semaphore when done
	defer s.releaseSemaphore()

	// Log that we're starting the API call
	utils.Debug("Making %s API call (concurrency: %d/%d)", operationType, len(s.semaphore), cap(s.semaphore))

	// Convert the request to JSON
	requestBody, err := json.Marshal(chatRequest)
	if err != nil {
		return chatResponse, err
	}

	// Make the HTTP request
	client := &http.Client{}
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return chatResponse, err
	}

	// Set the necessary headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", apiKey)

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return chatResponse, err
	}
	defer resp.Body.Close()

	// Increment completed jobs counter
	current := atomic.AddInt32(&s.completedJobs, 1)
	total := atomic.LoadInt32(&s.totalJobs)

	// Log progress and token usage in a single statement
	utils.Log("AI Parser progress: %d/%d (%.1f%%) - %s completed | Remaining Tokens: %s",
		current, total, (float64(current)/float64(total))*100, operationType, resp.Header["X-Ratelimit-Remaining-Tokens"])

	// Read and parse the response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return chatResponse, err
	}

	err = json.Unmarshal(body, &chatResponse)
	if err != nil {
		return chatResponse, err
	}

	if len(chatResponse.Choices) == 0 {
		utils.Error("No response received from Chat.")
		utils.Debug("Response body: %s", body)
	}

	return chatResponse, nil
}

func (s *AIParserStep) GenerateOneLinerDesc(ctx context.Context, listing models.Listing) (models.Listing, error) {
	// Skip if one liner description is already present
	if listing.OneLinerDesc == nil {
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
				{Role: "user", Content: "Generate a concise one-liner description which outlines the tech stack of the job in the form: 'Apply your [tech stack] skills on [projects]'. Respond with only the one-liner description: \n\n" + "\nDescription: " + *listing.Description},
			},
		}

		// Make the request
		chatResponse, err := s.makeChatRequest(ctx, endpoint, apiKey, chatRequest, "OneLinerDesc")
		if err != nil {
			return listing, err
		}

		// Output the assistant's response
		if err == nil && len(chatResponse.Choices) > 0 {
			oneLinerDesc := chatResponse.Choices[0].Message.Content
			oneLinerDescStr := strings.TrimSpace(oneLinerDesc)
			listing.OneLinerDesc = &oneLinerDescStr
		} else {
			utils.Error("No response received.")
		}
	} else {
		utils.Debug("One liner description already exists.")
		// Still increment the counter for completed jobs
		atomic.AddInt32(&s.completedJobs, 1)
		current := atomic.LoadInt32(&s.completedJobs)
		utils.Log("AI Parser progress: %d/%d (%.1f%%)", current, s.totalJobs, (float64(current)/float64(s.totalJobs))*100)
	}

	return listing, nil
}
