package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AIParserStep struct {
	completedJobs int32
	totalJobs     int32
	skippedJobs   int32
	semaphore     chan struct{}
	maxConcurrent int
	sleepDuration time.Duration
	mongodb       *database.MongoDB
}

// NewAiParserStep builds the step without a database handle, so it re-derives
// every field from scratch. Prefer NewAiParserStepWithCache: without the cache
// each run pays OpenAI again for thousands of listings it already enriched on
// the previous run.
func NewAiParserStep() *AIParserStep {
	return NewAiParserStepWithCache(nil)
}

func NewAiParserStepWithCache(mongodb *database.MongoDB) *AIParserStep {
	maxConcurrent := 6
	return &AIParserStep{
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
		sleepDuration: 1 * time.Second,
		mongodb:       mongodb,
	}
}

// cachedEnrichment is the set of AI-derived fields already stored for a
// fingerprint. Each one is derived from data that the fingerprint itself pins
// down (title, company) or that changes rarely (the description), so reusing
// them is sound and turns a steady-state run into a handful of API calls
// instead of three per listing.
type cachedEnrichment struct {
	OneLinerDesc  *string              `bson:"one_liner"`
	IndustryField models.IndustryField `bson:"industry_field"`
	Type          *models.JobType      `bson:"type"`
	Discipline    models.Discipline    `bson:"discipline"`
	Fingerprint   string               `bson:"fingerprint"`
}

// applyCachedEnrichment fills in the AI fields for listings we have already
// processed in a previous run. The per-operation functions below all no-op when
// their target field is already populated, so a fully cached listing makes zero
// API calls.
func (s *AIParserStep) applyCachedEnrichment(ctx context.Context, listings []models.Listing) {
	if s.mongodb == nil {
		utils.Warn("AI Parser running without a DB cache: every listing will be re-sent to OpenAI.")
		return
	}

	fingerprints := make([]string, len(listings))
	for i, listing := range listings {
		fingerprints[i] = GenerateListingFingerprint(listing)
	}

	coll := s.mongodb.GetDatabase().Collection("active_jobs")
	cursor, err := coll.Find(ctx,
		bson.M{"fingerprint": bson.M{"$in": fingerprints}},
		options.Find().SetProjection(bson.M{
			"fingerprint":    1,
			"one_liner":      1,
			"industry_field": 1,
			"type":           1,
			"discipline":     1,
		}))
	if err != nil {
		utils.Warn("Failed to load AI enrichment cache, falling back to full reprocessing: %v", err)
		return
	}
	defer cursor.Close(ctx)

	cache := make(map[string]cachedEnrichment)
	var stored []cachedEnrichment
	if err := cursor.All(ctx, &stored); err != nil {
		utils.Warn("Failed to decode AI enrichment cache, falling back to full reprocessing: %v", err)
		return
	}
	for _, doc := range stored {
		cache[doc.Fingerprint] = doc
	}

	hits := 0
	for i := range listings {
		doc, ok := cache[fingerprints[i]]
		if !ok {
			continue
		}

		if listings[i].OneLinerDesc == nil && doc.OneLinerDesc != nil && *doc.OneLinerDesc != "" {
			listings[i].OneLinerDesc = doc.OneLinerDesc
		}
		if listings[i].IndustryField == "" {
			listings[i].IndustryField = doc.IndustryField
		}
		if listings[i].Type == nil {
			listings[i].Type = doc.Type
		}
		if listings[i].Discipline == "" {
			listings[i].Discipline = doc.Discipline
		}

		if isFullyEnriched(listings[i]) {
			hits++
		}
	}

	utils.Success("AI enrichment cache: %d/%d listings already enriched, skipping their API calls.", hits, len(listings))
}

// isFullyEnriched reports whether every AI-derived field is already populated.
func isFullyEnriched(listing models.Listing) bool {
	return listing.OneLinerDesc != nil && *listing.OneLinerDesc != "" &&
		listing.IndustryField != "" &&
		listing.Type != nil &&
		listing.Discipline != ""
}

func (s *AIParserStep) Name() string {
	return "AI_Parser"
}

func (s *AIParserStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	// Reuse what a previous run already derived before deciding what to send to
	// OpenAI. In steady state the overwhelming majority of a scrape is listings
	// we saw yesterday, and re-deriving those was the bulk of the API bill.
	s.applyCachedEnrichment(ctx, listings)

	parsedListings := make([]models.Listing, len(listings))
	s.totalJobs = int32(len(listings) * 3)
	s.completedJobs = 0
	s.skippedJobs = 0

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

	utils.Success("AI Parser finished: %d/%d operations served without an API call.",
		atomic.LoadInt32(&s.skippedJobs), s.totalJobs)

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
	// MaxTokens caps what we pay for on the output side, which is 4x the input
	// price. Every prompt here has a short expected answer, so the cap only ever
	// bites on a runaway response.
	MaxTokens int `json:"max_tokens,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// blockTagPattern detects a description that already carries HTML block
// structure. Sources overwhelmingly return HTML, so this is the common case.
var blockTagPattern = regexp.MustCompile(`(?i)<(p|div|h[1-6]|ul|ol|li|br|table|section)\b`)

// invisibleRunePattern matches the whitespace-lookalike runes that leak in from
// scraped pages: non-breaking space, narrow no-break space, zero-width space,
// zero-width non-joiner/joiner, and the BOM.
var invisibleRunePattern = regexp.MustCompile("[\u00a0\u202f\u2007\u200b\u200c\u200d\ufeff]")

// CleanDescription strips the invisible junk the old LLM prompt was asked to
// remove. This is a deterministic substitution, so paying a model to echo a
// 1000-token description back with the same substitution applied was the single
// largest line on the OpenAI bill for no gain in quality.
func CleanDescription(description string) string {
	cleaned := invisibleRunePattern.ReplaceAllString(description, " ")
	cleaned = strings.ReplaceAll(cleaned, "\\n", "")
	return strings.TrimSpace(cleaned)
}

// FixDescriptionFormat normalises the description. It only falls back to the
// model when the description arrives as plain text and genuinely needs HTML
// structure added; when the source already returned HTML -- which is nearly
// always -- the cleanup is done locally for free.
func (s *AIParserStep) FixDescriptionFormat(ctx context.Context, listing models.Listing) (models.Listing, error) {
	if listing.Description == nil || strings.TrimSpace(*listing.Description) == "" {
		atomic.AddInt32(&s.completedJobs, 1)
		return listing, nil
	}

	cleaned := CleanDescription(*listing.Description)
	*listing.Description = cleaned

	if blockTagPattern.MatchString(cleaned) {
		atomic.AddInt32(&s.completedJobs, 1)
		atomic.AddInt32(&s.skippedJobs, 1)
		utils.Debug("Description for %q already has HTML structure, skipping FixDescription API call.", listing.Title)
		return listing, nil
	}

	chatRequest := ChatRequest{
		Model:     "gpt-4o-mini",
		Stream:    false,
		MaxTokens: 4096,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Add appropriate HTML tags to the following job description; for headings use <h3>. Only return the modified description as plain text without any additional words or explanation: " + cleaned},
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
	// The classification depends only on title and company, both of which are
	// components of the fingerprint. If a previous run already resolved all
	// three axes for this fingerprint the answer cannot have changed.
	if listing.IndustryField != "" && listing.Type != nil && listing.Discipline != "" {
		atomic.AddInt32(&s.completedJobs, 1)
		atomic.AddInt32(&s.skippedJobs, 1)
		utils.Debug("Classification already known for %q, skipping API call.", listing.Title)
		return listing, nil
	}

	chatRequest := ChatRequest{
		Model:     "gpt-4o-mini",
		Stream:    false,
		MaxTokens: 16,
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

// oneLinerDescriptionLimit caps how much of a description is sent for the
// one-liner. The prompt only needs the tech stack and the projects, which sit
// near the top of a posting; the tail is boilerplate about benefits and equal
// opportunity that we were paying full input price to send.
const oneLinerDescriptionLimit = 6000

// truncateForPrompt trims on a rune boundary so a multi-byte character is never
// cut in half.
func truncateForPrompt(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

func (s *AIParserStep) GenerateOneLinerDesc(ctx context.Context, listing models.Listing) (models.Listing, error) {
	if listing.OneLinerDesc == nil {
		if listing.Description == nil || strings.TrimSpace(*listing.Description) == "" {
			atomic.AddInt32(&s.completedJobs, 1)
			atomic.AddInt32(&s.skippedJobs, 1)
			return listing, nil
		}

		chatRequest := ChatRequest{
			Model:     "gpt-4o-mini",
			Stream:    false,
			MaxTokens: 80,
			Messages: []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{
				{Role: "system", Content: "You are a helpful assistant."},
				{Role: "user", Content: "Generate a concise one-liner description which outlines the tech stack of the job in the form: 'Apply your [tech stack] skills on [projects]'. Respond with only the one-liner description: \n\n" + "\nDescription: " + truncateForPrompt(*listing.Description, oneLinerDescriptionLimit)},
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
		atomic.AddInt32(&s.skippedJobs, 1)
		current := atomic.LoadInt32(&s.completedJobs)
		utils.Log("AI Parser progress: %d/%d (%.1f%%)", current, s.totalJobs, (float64(current)/float64(s.totalJobs))*100)
	}

	return listing, nil
}

const maxChatAttempts = 5

// retryAfterOrBackoff prefers the server's own Retry-After hint and otherwise
// backs off exponentially with jitter, so a burst of goroutines that all get
// rate limited at once do not retry in lockstep.
func retryAfterOrBackoff(retryAfter string, attempt int) time.Duration {
	if retryAfter != "" {
		if seconds, err := strconv.ParseFloat(retryAfter, 64); err == nil && seconds > 0 {
			return time.Duration(seconds * float64(time.Second))
		}
	}

	backoff := time.Duration(1<<uint(attempt-1)) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	return backoff + time.Duration(rand.Int63n(int64(time.Second)))
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

	client := &http.Client{Timeout: 2 * time.Minute}

	var resp *http.Response
	for attempt := 1; attempt <= maxChatAttempts; attempt++ {
		// Build a fresh request per attempt. Reusing one request and cloning it
		// does not rewind the body: http.Request.Clone copies the reader as-is,
		// so every retry after the first sent an empty body against a
		// Content-Length that still described the full payload, and failed
		// outright. Rate limits were therefore fatal rather than retried.
		req, err := http.NewRequestWithContext(ctx, "POST", openaiEndpoint, bytes.NewReader(requestBody))
		if err != nil {
			return chatResponse, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		r, err := client.Do(req)
		if err != nil {
			return chatResponse, err
		}

		if r.StatusCode != http.StatusTooManyRequests {
			resp = r
			break
		}

		// OpenAI returns 429 both for genuine rate limiting and for an
		// exhausted quota. Surface the body so the two are distinguishable in
		// the logs instead of looking like the same transient problem.
		reason, _ := ioutil.ReadAll(io.LimitReader(r.Body, 512))
		r.Body.Close()

		if attempt == maxChatAttempts {
			return chatResponse, fmt.Errorf("rate limited after %d attempts for %s: %s",
				maxChatAttempts, operationType, strings.TrimSpace(string(reason)))
		}

		wait := retryAfterOrBackoff(r.Header.Get("Retry-After"), attempt)
		utils.Warn("Rate limit hit for %s, retrying in %s (attempt %d/%d): %s",
			operationType, wait, attempt, maxChatAttempts, strings.TrimSpace(string(reason)))

		select {
		case <-ctx.Done():
			return chatResponse, ctx.Err()
		case <-time.After(wait):
		}
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
