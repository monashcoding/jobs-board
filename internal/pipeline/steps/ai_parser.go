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
	completedJobs int32
	totalJobs     int32
	semaphore     chan struct{}
	maxConcurrent int
	sleepDuration time.Duration
}

func NewAiParserStep() *AIParserStep {
	maxConcurrent := 3 // Lower concurrency for CPU Ollama
	return &AIParserStep{
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
		sleepDuration: 1 * time.Second,
	}
}

func (s *AIParserStep) Name() string {
	return "AI_Parser"
}

func (s *AIParserStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	parsedListings := make([]models.Listing, len(listings))
	s.totalJobs = int32(len(listings) * 3)
	s.completedJobs = 0

	utils.Log("Running %d listings (%d operations) through AI Parser.", len(listings), s.totalJobs)
	utils.Log("Max Concurrent Requests: %d | Sleep: %s between requests",
		s.maxConcurrent, s.sleepDuration.String())

	var wg sync.WaitGroup
	errCh := make(chan error, len(listings))

	for i, listing := range listings {
		wg.Add(1)
		go func(i int, listing models.Listing) {
			defer wg.Done()

			updatedListing, err := s.FixDescriptionFormat(ctx, listing)
			if err != nil {
				utils.Warn("Failed to fix description format: %v", err)
				errCh <- err
			}

			updatedListing, err = s.GetIndustryFieldandType(ctx, updatedListing)
			if err != nil {
				utils.Warn("Failed to get industry field: %v", err)
				errCh <- err
			}

			updatedListing, err = s.GenerateOneLinerDesc(ctx, updatedListing)
			if err != nil {
				utils.Warn("Failed to generate one liner description: %v", err)
				errCh <- err
			}

			parsedListings[i] = updatedListing
		}(i, listing)
	}

	wg.Wait()
	close(errCh)

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

func (s *AIParserStep) acquireSemaphore(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.semaphore <- struct{}{}:
		return nil
	}
}

func (s *AIParserStep) releaseSemaphore() {
	time.Sleep(s.sleepDuration)
	<-s.semaphore
}

type ChatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Stream bool `json:"stream"`
}

type ChatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
}

func (s *AIParserStep) FixDescriptionFormat(ctx context.Context, listing models.Listing) (models.Listing, error) {
	chatRequest := ChatRequest{
		Model:  "llama3.2:3b",
		Stream: false,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Fix the description by adding appropriate HTML tags, for headings use <h3>. Please remove any weird symbols like \\xa0 and \\u202f. Only return the modified description as plain text without any additional words or explanation: " + *listing.Description},
		},
	}

	chatResponse, err := s.makeChatRequest(ctx, chatRequest, "FixDescription")
	if err != nil {
		return listing, err
	}

	if chatResponse.Message.Content != "" {
		desc := strings.TrimPrefix(chatResponse.Message.Content, "```html")
		desc = strings.TrimSuffix(desc, "```")
		desc = strings.TrimSpace(desc)
		*listing.Description = desc
	} else {
		utils.Error("No response received.")
	}

	*listing.Description = strings.ReplaceAll(*listing.Description, "\\n", "")
	return listing, nil
}

func (s *AIParserStep) GetIndustryFieldandType(ctx context.Context, listing models.Listing) (models.Listing, error) {
	chatRequest := ChatRequest{
		Model:  "llama3.2:3b",
		Stream: false,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "...your prompt..."},
		},
	}

	chatResponse, err := s.makeChatRequest(ctx, chatRequest, "GetIndustryField")
	if err != nil {
		return listing, err
	}

	if chatResponse.Message.Content != "" {
		splitResponse := strings.Split(chatResponse.Message.Content, "#")

		industryField := "OTHER_INDUSTRY"
		jobType := "OTHER"

		if len(splitResponse) >= 2 {
			industryField = strings.TrimSpace(splitResponse[0])
			jobType = strings.TrimSpace(splitResponse[1])
		} else {
			utils.Warn("Unexpected format in AI response: %q. Falling back to default values.", chatResponse)
		}

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

		if listing.IndustryField == "" {
			listing.IndustryField = industryMap[industryField]
		}

		jobTypeMap := map[string]models.JobType{
			"INTERN":   models.INTERN,
			"GRADUATE": models.GRADUATE,
			"OTHER":    models.OTHER,
		}

		if listing.Type == nil {
			jobTypeValue := jobTypeMap[jobType]
			listing.Type = &jobTypeValue
		}
	} else {
		utils.Error("No response received from Chat.")
	}

	return listing, nil
}

func (s *AIParserStep) GenerateOneLinerDesc(ctx context.Context, listing models.Listing) (models.Listing, error) {
	if listing.OneLinerDesc == nil {
		chatRequest := ChatRequest{
			Model:  "llama3.2:3b",
			Stream: false,
			Messages: []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{
				{Role: "system", Content: "You are a helpful assistant."},
				{Role: "user", Content: "Generate a concise one-liner description which outlines the tech stack of the job in the form: 'Apply your [tech stack] skills on [projects]'. Respond with only the one-liner description: \n\n" + "\nDescription: " + *listing.Description},
			},
		}

		chatResponse, err := s.makeChatRequest(ctx, chatRequest, "OneLinerDesc")
		if err != nil {
			return listing, err
		}

		if chatResponse.Message.Content != "" {
			oneLinerDescStr := strings.TrimSpace(chatResponse.Message.Content)
			listing.OneLinerDesc = &oneLinerDescStr
		} else {
			utils.Error("No response received.")
		}
	} else {
		utils.Debug("One liner description already exists.")
		atomic.AddInt32(&s.completedJobs, 1)
		current := atomic.LoadInt32(&s.completedJobs)
		utils.Log("AI Parser progress: %d/%d (%.1f%%)", current, s.totalJobs, (float64(current)/float64(s.totalJobs))*100)
	}

	return listing, nil
}

func (s *AIParserStep) makeChatRequest(ctx context.Context, chatRequest ChatRequest, operationType string) (ChatResponse, error) {
	var chatResponse ChatResponse

	if err := s.acquireSemaphore(ctx); err != nil {
		return chatResponse, fmt.Errorf("failed to acquire semaphore: %w", err)
	}
	defer s.releaseSemaphore()

	utils.Debug("Making %s API call (concurrency: %d/%d)", operationType, len(s.semaphore), cap(s.semaphore))

	requestBody, err := json.Marshal(chatRequest)
	if err != nil {
		return chatResponse, err
	}

	ollamaEndpoint := os.Getenv("OLLAMA_ENDPOINT")

	client := &http.Client{}
	req, err := http.NewRequest("POST", ollamaEndpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return chatResponse, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return chatResponse, err
	}
	defer resp.Body.Close()

	current := atomic.AddInt32(&s.completedJobs, 1)
	total := atomic.LoadInt32(&s.totalJobs)
	utils.Log("AI Parser progress: %d/%d (%.1f%%) - %s completed",
		current, total, (float64(current)/float64(total))*100, operationType)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return chatResponse, err
	}

	err = json.Unmarshal(body, &chatResponse)
	if err != nil {
		return chatResponse, err
	}

	if chatResponse.Message.Content == "" {
		utils.Error("No response received from Ollama.")
		utils.Debug("Response body: %s", body)
	}

	return chatResponse, nil
}
