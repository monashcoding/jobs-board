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
	maxConcurrent := 6
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
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (s *AIParserStep) FixDescriptionFormat(ctx context.Context, listing models.Listing) (models.Listing, error) {
	if listing.Description == nil || strings.TrimSpace(*listing.Description) == "" {
		atomic.AddInt32(&s.completedJobs, 1)
		return listing, nil
	}

	chatRequest := ChatRequest{
		Model:  "gpt-4o-mini",
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

	if len(chatResponse.Choices) > 0 {
		desc := strings.TrimPrefix(chatResponse.Choices[0].Message.Content, "```html")
		desc = strings.TrimSuffix(desc, "```")
		desc = strings.TrimSpace(desc)
		*listing.Description = desc
	} else {
		utils.Error("No response received.")
	}

	*listing.Description = strings.ReplaceAll(*listing.Description, "\\n", "")
	return listing, nil
}

var industryMap = map[string]models.IndustryField{
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

var jobTypeMap = map[string]models.JobType{
	"INTERN":   models.INTERN,
	"GRADUATE": models.GRADUATE,
	"OTHER":    models.OTHER,
}

var disciplineMap = map[string]models.Discipline{
	"SOFTWARE":         models.SOFTWARE,
	"DATA":             models.DATA,
	"CYBER":            models.CYBER,
	"ENGINEERING":      models.ENGINEERING,
	"OTHER_DISCIPLINE": models.OTHER_DISCIPLINE,
}

// ApplyClassification parses an INDUSTRY#JOBTYPE#DISCIPLINE response. Every
// axis falls back to its OTHER value independently, so a model that garbles one
// field does not cost us the other two. title is used for logging only.
func ApplyClassification(content string, title string) (models.IndustryField, models.JobType, models.Discipline) {
	parts := strings.Split(content, "#")

	industryField := models.OTHER_INDUSTRY
	jobType := models.OTHER
	discipline := models.OTHER_DISCIPLINE

	if len(parts) < 2 {
		utils.Warn("Unexpected format in AI response %q for %q. Falling back to default values.", content, title)
		return industryField, jobType, discipline
	}

	if mapped, ok := industryMap[strings.TrimSpace(parts[0])]; ok {
		industryField = mapped
	} else {
		utils.Warn("Unknown industry %q for %q. Defaulting to OTHER_INDUSTRY.", parts[0], title)
	}

	if mapped, ok := jobTypeMap[strings.TrimSpace(parts[1])]; ok {
		jobType = mapped
	} else {
		utils.Warn("Unknown job type %q for %q. Defaulting to OTHER.", parts[1], title)
	}

	// Tolerate the older two-part response shape rather than losing the
	// industry and job type when the model omits the discipline.
	if len(parts) < 3 {
		utils.Warn("AI response %q for %q has no discipline. Defaulting to OTHER_DISCIPLINE.", content, title)
		return industryField, jobType, discipline
	}

	if mapped, ok := disciplineMap[strings.TrimSpace(parts[2])]; ok {
		discipline = mapped
	} else {
		utils.Warn("Unknown discipline %q for %q. Defaulting to OTHER_DISCIPLINE.", parts[2], title)
	}

	return industryField, jobType, discipline
}

func (s *AIParserStep) GetIndustryFieldandType(ctx context.Context, listing models.Listing) (models.Listing, error) {
	chatRequest := ChatRequest{
		Model:  "gpt-4o-mini",
		Stream: false,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Classify the listing on three axes.\n\n" +
				"1. INDUSTRY, the employer's sector. One of: CONSULTING, BANKS, BIG_TECH, TECH, QUANT_TRADING, ENERGY, GOVERNMENT, MANUFACTURING, TELECOMMUNICATIONS, OTHER_INDUSTRY.\n" +
				"2. JOBTYPE, one of: INTERN, GRADUATE, OTHER.\n" +
				"3. DISCIPLINE, the technical discipline of the work itself. One of:\n" +
				"   SOFTWARE - writing or shipping software: software engineering, web, mobile, embedded, devops, cloud, platform, SRE, QA.\n" +
				"   DATA - data science, data engineering, analytics, machine learning, AI research.\n" +
				"   CYBER - cyber security, information security, penetration testing.\n" +
				"   ENGINEERING - non-software engineering: civil, mechanical, electrical, chemical, mining, aerospace.\n" +
				"   OTHER_DISCIPLINE - anything else, including consulting, finance, sales, marketing, HR, legal, supply chain, and general graduate programs with no technical component.\n\n" +
				"Judge DISCIPLINE by the role, not the employer: a marketing role at a software company is OTHER_DISCIPLINE. If the title is generic (e.g. \"Graduate Program\") use the company and description to decide, and prefer OTHER_DISCIPLINE when the work is not clearly technical.\n\n" +
				"Output exactly, with no other text, in the form: INDUSTRY#JOBTYPE#DISCIPLINE\n\nTitle: " + listing.Title + "\nCompany: " + listing.Company.Name},
		},
	}

	chatResponse, err := s.makeChatRequest(ctx, chatRequest, "GetIndustryField")
	if err != nil {
		return listing, err
	}

	if len(chatResponse.Choices) > 0 {
		industryField, jobType, discipline := ApplyClassification(chatResponse.Choices[0].Message.Content, listing.Title)

		if listing.IndustryField == "" {
			listing.IndustryField = industryField
		}

		if listing.Type == nil {
			listing.Type = &jobType
		}

		listing.Discipline = discipline
	} else {
		utils.Error("No response received from Chat.")
	}

	return listing, nil
}

func (s *AIParserStep) GenerateOneLinerDesc(ctx context.Context, listing models.Listing) (models.Listing, error) {
	if listing.OneLinerDesc == nil {
		chatRequest := ChatRequest{
			Model:  "gpt-4o-mini",
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

		if len(chatResponse.Choices) > 0 {
			oneLinerDescStr := strings.TrimSpace(chatResponse.Choices[0].Message.Content)
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

	openaiEndpoint := "https://api.openai.com/v1/chat/completions"
	apiKey := os.Getenv("OPENAI_API_KEY")

	client := &http.Client{}
	req, err := http.NewRequest("POST", openaiEndpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return chatResponse, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	var resp *http.Response
	for attempt := 1; attempt <= 3; attempt++ {
		r, err := client.Do(req.Clone(ctx))
		if err != nil {
			return chatResponse, err
		}
		if r.StatusCode == http.StatusTooManyRequests {
			r.Body.Close()
			wait := time.Duration(attempt) * 2 * time.Second
			utils.Warn("Rate limit hit for %s, retrying in %s (attempt %d/3)", operationType, wait, attempt)
			time.Sleep(wait)
			continue
		}
		resp = r
		break
	}
	if resp == nil {
		return chatResponse, fmt.Errorf("rate limit exceeded after 3 retries for %s", operationType)
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

	if resp.StatusCode != http.StatusOK {
		return chatResponse, fmt.Errorf("OpenAI API returned status %d: %s", resp.StatusCode, string(body))
	}

	err = json.Unmarshal(body, &chatResponse)
	if err != nil {
		return chatResponse, err
	}

	if len(chatResponse.Choices) == 0 || chatResponse.Choices[0].Message.Content == "" {
		utils.Error("No response received from OpenAI.")
		utils.Debug("Response body: %s", body)
	}

	return chatResponse, nil
}
