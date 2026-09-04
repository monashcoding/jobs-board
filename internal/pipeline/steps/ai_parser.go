package steps

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"sort"
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
	OneLinerDesc          *string              `bson:"one_liner"`
	IndustryField         models.IndustryField `bson:"industry_field"`
	Type                  *models.JobType      `bson:"type"`
	Discipline            models.Discipline    `bson:"discipline"`
	Fingerprint           string               `bson:"fingerprint"`
	Description           *string              `bson:"description"`
	SourceDescriptionHash string               `bson:"source_description_hash"`
}

// HashSourceDescription fingerprints a description exactly as scraped. It is
// recorded alongside the formatted description so a later run can tell whether
// the source text actually changed, which the stored description cannot answer
// on its own: what we store has already been reformatted.
func HashSourceDescription(description string) string {
	sum := sha256.Sum256([]byte(description))
	return hex.EncodeToString(sum[:])
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
			"fingerprint":             1,
			"one_liner":               1,
			"industry_field":          1,
			"type":                    1,
			"discipline":              1,
			"description":             1,
			"source_description_hash": 1,
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
		// Several documents can share a fingerprint: re-keying collapses
		// duplicates onto one identity, and they are only removed in a later
		// migration phase. Until then, keeping whichever the driver returned
		// last would sometimes pick the emptier row and silently re-derive
		// fields we already had, so prefer the most complete one.
		if existing, clash := cache[doc.Fingerprint]; clash && enrichmentScore(existing) >= enrichmentScore(doc) {
			continue
		}
		cache[doc.Fingerprint] = doc
	}

	var reusedOneLiner, reusedClassification, reusedDescription int

	for i := range listings {
		doc, ok := cache[fingerprints[i]]
		if !ok {
			continue
		}

		if listings[i].OneLinerDesc == nil && doc.OneLinerDesc != nil && *doc.OneLinerDesc != "" {
			listings[i].OneLinerDesc = doc.OneLinerDesc
			reusedOneLiner++
		}
		// The three axes are one answer from one call, so reuse them together
		// or not at all. Filling them piecemeal poisoned the result: a document
		// stored before the schema existed has a job type but no discipline, so
		// the classification ran anyway, produced a correct answer, and then
		// discarded the job type because the cache had already filled it with
		// the stale one. Job type is a hard gate, so a listing wrongly stored as
		// OTHER could never recover and stayed off the board permanently.
		if doc.IndustryField != "" && doc.Type != nil && doc.Discipline != "" {
			if listings[i].IndustryField == "" {
				listings[i].IndustryField = doc.IndustryField
			}
			if listings[i].Type == nil {
				listings[i].Type = doc.Type
			}
			if listings[i].Discipline == "" {
				listings[i].Discipline = doc.Discipline
			}
			reusedClassification++
		}

		// Reuse the stored formatted description when the source text is
		// byte-identical to what produced it. Sources that return plain text --
		// LinkedIn always, GradConnection about a third of the time -- otherwise
		// pay for the formatting call on every run forever, because the raw text
		// is rescraped each time while only the formatted version is stored.
		if reuseStoredDescription(&listings[i], doc) {
			reusedDescription++
		}
	}

	utils.Success(
		"AI enrichment cache: reused %d one-liners, %d classifications, %d formatted descriptions across %d listings.",
		reusedOneLiner, reusedClassification, reusedDescription, len(listings))
}

// enrichmentScore counts how many cacheable fields a stored document actually
// carries, so the richest of a set of colliding documents wins deterministically.
func enrichmentScore(doc cachedEnrichment) int {
	score := 0
	if doc.OneLinerDesc != nil && *doc.OneLinerDesc != "" {
		score++
	}
	if doc.IndustryField != "" {
		score++
	}
	if doc.Type != nil {
		score++
	}
	if doc.Discipline != "" {
		score++
	}
	if doc.SourceDescriptionHash != "" && doc.Description != nil && *doc.Description != "" {
		score++
	}
	return score
}

// reuseStoredDescription swaps in the previously formatted description when the
// scraped source text is unchanged, and reports whether it did.
func reuseStoredDescription(listing *models.Listing, doc cachedEnrichment) bool {
	if listing.Description == nil || listing.SourceDescriptionHash == "" {
		return false
	}
	if doc.SourceDescriptionHash == "" || doc.SourceDescriptionHash != listing.SourceDescriptionHash {
		return false
	}
	if doc.Description == nil || strings.TrimSpace(*doc.Description) == "" {
		return false
	}

	stored := *doc.Description
	listing.Description = &stored
	return true
}

func (s *AIParserStep) Name() string {
	return "AI_Parser"
}

func (s *AIParserStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	// Record the source text fingerprint before anything reformats the
	// description in place, so the cache lookup below has something to compare
	// against and the upload step has something to store.
	for i := range listings {
		if listings[i].Description != nil {
			listings[i].SourceDescriptionHash = HashSourceDescription(*listings[i].Description)
		}
	}

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

	// Errors are collected under a mutex rather than sent to a buffered
	// channel. Each listing can produce three errors, so a channel sized to the
	// listing count deadlocked as soon as more than a third of the operations
	// failed: nothing drains it until after wg.Wait(), so the send blocked, its
	// goroutine never returned, and the run hung forever instead of finishing
	// with errors. An exhausted OpenAI quota fails every call in the run, which
	// is exactly the case that tripped it.
	var (
		errMu  sync.Mutex
		errs   []error
		failed int
	)

	recordErr := func(format string, err error) {
		utils.Warn(format, err)
		errMu.Lock()
		defer errMu.Unlock()
		failed++
		// Keep a bounded sample. A failing run produces thousands of near
		// identical errors and only the shape of them is useful.
		if len(errs) < maxCollectedErrors {
			errs = append(errs, err)
		}
	}

	for i, listing := range listings {
		wg.Add(1)
		go func(i int, listing models.Listing) {
			defer wg.Done()

			updatedListing, err := s.FixDescriptionFormat(ctx, listing)
			if err != nil {
				recordErr("Failed to fix description format: %v", err)
			}

			updatedListing, err = s.GetIndustryFieldandType(ctx, updatedListing)
			if err != nil {
				recordErr("Failed to get industry field: %v", err)
			}

			updatedListing, err = s.GenerateOneLinerDesc(ctx, updatedListing)
			if err != nil {
				recordErr("Failed to generate one liner description: %v", err)
			}

			parsedListings[i] = updatedListing
		}(i, listing)
	}

	wg.Wait()

	utils.Success("AI Parser finished: %d/%d operations served without an API call.",
		atomic.LoadInt32(&s.skippedJobs), s.totalJobs)

	if failed == 0 {
		return parsedListings, nil
	}

	// Report the failure count alongside a sample. Listings that failed keep
	// their zero values, which the upload step leaves alone rather than
	// writing over what a previous run stored.
	var finalErr error
	for _, err := range errs {
		if finalErr == nil {
			finalErr = err
		} else {
			finalErr = fmt.Errorf("%v; %v", finalErr, err)
		}
	}

	// A handful of failures must not discard the run. Returning an error here
	// aborts the pipeline, so board scoring and the upload never happen and
	// every listing is thrown away -- three rate limited one liners out of
	// 1971 operations were enough to lose a whole scrape. Only give up when
	// enough of the run failed that the board would be built from mostly
	// unclassified listings.
	if !runIsBroken(failed, int(s.totalJobs)) {
		utils.Warn("AI Parser: %d/%d operations failed, continuing with what succeeded: %v",
			failed, s.totalJobs, finalErr)
		return parsedListings, nil
	}

	return parsedListings, fmt.Errorf("%d/%d AI operations failed: %w", failed, s.totalJobs, finalErr)
}

func (s *AIParserStep) acquireSemaphore(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.semaphore <- struct{}{}:
		return nil
	}
}

// releaseSemaphore frees the slot before pacing. Sleeping while still holding
// the slot pinned throughput to maxConcurrent requests per sleepDuration --
// roughly six a second -- so a cold run spent tens of minutes asleep. The pause
// still spaces this goroutine's next request out; it just no longer blocks the
// other five.
func (s *AIParserStep) releaseSemaphore() {
	<-s.semaphore
	time.Sleep(s.sleepDuration)
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
	// ResponseFormat constrains generation to a schema where one applies.
	ResponseFormat map[string]any `json:"response_format,omitempty"`
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

	// This prompt echoes the whole description back, so the output cap has to
	// clear the input or the description is silently stored truncated. A few
	// postings are enormous (the longest seen is ~18k tokens); rather than pay
	// to round-trip those, leave them cleaned but unformatted.
	if len(cleaned) > maxFormattableDescription {
		atomic.AddInt32(&s.completedJobs, 1)
		atomic.AddInt32(&s.skippedJobs, 1)
		utils.Warn("Description for %q is %d chars, too long to reformat safely; keeping it unformatted.", listing.Title, len(cleaned))
		return listing, nil
	}

	chatRequest := ChatRequest{
		Model:  "gpt-4o-mini",
		Stream: false,
		// The model's ceiling. A cap is only a limit, not a charge, so setting
		// it high costs nothing and removes any chance of truncation.
		MaxTokens: 16384,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: "You mark up job descriptions as HTML. You never reword, summarise, shorten or omit any part of the text you are given."},
			{Role: "user", Content: "Wrap the following job description in HTML tags: paragraphs in <p>, headings in <h3>, lists in <ul>/<li>. " +
				"Reproduce every word of the original exactly; do not summarise, shorten, reorder or add commentary. " +
				"Return only the resulting HTML.\n\n" + cleaned},
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
	// RETAIL and TRANSPORTATION exist in models.IndustryField but were absent
	// here and from the prompt, so retail and transport employers could only
	// ever be classified OTHER_INDUSTRY.
	"RETAIL":         models.RETAIL,
	"TRANSPORTATION": models.TRANSPORTATION,
	"OTHER_INDUSTRY": models.OTHER_INDUSTRY,
}

// sortedKeys returns a map's keys in a stable order, so the schema sent to the
// model does not churn between runs.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// classificationResponse is the structured reply. Constraining generation to the
// enums below is what stops the model inventing values: it previously returned
// "INTERNS" for a vacation program and put a discipline ("ENGINEERING") in the
// industry slot, and because job type is a hard gate on board eligibility, each
// near miss silently dropped a real job off the board.
type classificationResponse struct {
	Industry   string `json:"industry"`
	JobType    string `json:"job_type"`
	Discipline string `json:"discipline"`
}

// classificationSchema builds the strict JSON schema for the classification
// call, derived from the same maps used to interpret the reply so the two
// cannot drift apart.
func classificationSchema() map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "job_classification",
			"strict": true,
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"industry":   map[string]any{"type": "string", "enum": sortedKeys(industryMap)},
					"job_type":   map[string]any{"type": "string", "enum": sortedKeys(jobTypeMap)},
					"discipline": map[string]any{"type": "string", "enum": sortedKeys(disciplineMap)},
				},
				"required":             []string{"industry", "job_type", "discipline"},
				"additionalProperties": false,
			},
		},
	}
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

	// Tolerate the older two-part response shape rather than losing the
	// industry and job type when the model omits the discipline.
	rawDiscipline := ""
	if len(parts) >= 3 {
		rawDiscipline = parts[2]
	} else {
		utils.Warn("AI response %q for %q has no discipline. Defaulting to OTHER_DISCIPLINE.", content, title)
	}

	return mapClassification(parts[0], parts[1], rawDiscipline, title)
}

// mapClassification resolves the three raw axis values against the enums. Each
// axis falls back independently, so one garbled field does not cost the others.
func mapClassification(rawIndustry, rawJobType, rawDiscipline, title string) (models.IndustryField, models.JobType, models.Discipline) {
	industryField := models.OTHER_INDUSTRY
	jobType := models.OTHER
	discipline := models.OTHER_DISCIPLINE

	if mapped, ok := industryMap[strings.TrimSpace(rawIndustry)]; ok {
		industryField = mapped
	} else {
		utils.Warn("Unknown industry %q for %q. Defaulting to OTHER_INDUSTRY.", rawIndustry, title)
	}

	if mapped, ok := jobTypeMap[strings.TrimSpace(rawJobType)]; ok {
		jobType = mapped
	} else {
		utils.Warn("Unknown job type %q for %q. Defaulting to OTHER.", rawJobType, title)
	}

	if strings.TrimSpace(rawDiscipline) == "" {
		return industryField, jobType, discipline
	}

	if mapped, ok := disciplineMap[strings.TrimSpace(rawDiscipline)]; ok {
		discipline = mapped
	} else {
		utils.Warn("Unknown discipline %q for %q. Defaulting to OTHER_DISCIPLINE.", rawDiscipline, title)
	}

	return industryField, jobType, discipline
}

// parseClassification reads the structured reply, falling back to the older
// INDUSTRY#JOBTYPE#DISCIPLINE shape if the content is not JSON. The fallback
// matters for a response that predates the schema or arrives truncated; it costs
// nothing when the schema is honoured, which is the normal case.
func parseClassification(content string, title string) (models.IndustryField, models.JobType, models.Discipline) {
	var parsed classificationResponse

	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &parsed); err != nil {
		utils.Warn("Classification reply for %q was not JSON (%v); falling back to the delimited format.", title, err)
		return ApplyClassification(content, title)
	}

	return mapClassification(parsed.Industry, parsed.JobType, parsed.Discipline, title)
}

// classificationDescriptionSuffix appends an excerpt of the description to the
// classification prompt when one is available.
func classificationDescriptionSuffix(listing models.Listing) string {
	if listing.Description == nil || strings.TrimSpace(*listing.Description) == "" {
		return ""
	}

	return "\nDescription: " + truncateForPrompt(*listing.Description, classificationDescriptionLimit)
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
		Model:  "gpt-4o-mini",
		Stream: false,
		// Enough for the JSON object and no more. A truncated reply is not
		// valid against the schema, so this must not be trimmed further.
		MaxTokens:      100,
		ResponseFormat: classificationSchema(),
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Classify the listing on three axes.\n\n" +
				"1. INDUSTRY, the employer's sector. One of: CONSULTING, BANKS, BIG_TECH, TECH, QUANT_TRADING, ENERGY, GOVERNMENT, MANUFACTURING, TELECOMMUNICATIONS, RETAIL, TRANSPORTATION, OTHER_INDUSTRY.\n" +
				"2. JOBTYPE, one of: INTERN, GRADUATE, OTHER.\n" +
				"3. DISCIPLINE, the technical discipline of the work itself. One of:\n" +
				"   SOFTWARE - writing or shipping software: software engineering, web, mobile, embedded, devops, cloud, platform, SRE, QA.\n" +
				"   DATA - data science, data engineering, analytics, machine learning, AI research.\n" +
				"   CYBER - cyber security, information security, penetration testing.\n" +
				"   ENGINEERING - non-software engineering: civil, mechanical, electrical, chemical, mining, aerospace.\n" +
				"   OTHER_DISCIPLINE - anything else, including consulting, finance, sales, marketing, HR, legal, supply chain, and general graduate programs with no technical component.\n\n" +
				"Judge DISCIPLINE by the role, not the employer: a marketing role at a software company is OTHER_DISCIPLINE. If the title is generic (e.g. \"Graduate Program\") use the company and description to decide, and prefer OTHER_DISCIPLINE when the work is not clearly technical.\n\n" +
				"Respond with a JSON object holding the keys industry, job_type and discipline.\n\nTitle: " + listing.Title +
				"\nCompany: " + listing.Company.Name +
				classificationDescriptionSuffix(listing)},
		},
	}

	chatResponse, err := s.makeChatRequest(ctx, chatRequest, "GetIndustryField")
	if err != nil {
		return listing, err
	}

	if len(chatResponse.Choices) > 0 {
		industryField, jobType, discipline := parseClassification(chatResponse.Choices[0].Message.Content, listing.Title)

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

// maxFormattableDescription caps what is worth sending to be marked up. Beyond
// this the round trip costs more than the formatting is worth, and the reply
// risks running past even the model's output ceiling.
const maxFormattableDescription = 24000

// classificationDescriptionLimit is how much of the description accompanies the
// classification prompt. The prompt asks the model to fall back to the
// description when a title is generic ("Graduate Program"), which it could not
// do while only the title and company were sent -- and because discipline and
// job type are both hard gates on board eligibility, a role it could not place
// was dropped from the board entirely. The opening of a posting carries the
// role summary, which is the part that decides the discipline.
const classificationDescriptionLimit = 2000

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
				{Role: "user", Content: "Write one concise sentence describing this job, naming the concrete skills or tools it uses and what they are applied to. " +
					"Prefer the form 'Apply your [skills] skills on [projects]'. If the role is not technical, describe it plainly in the same length instead of inventing a tech stack. " +
					"Respond with only that sentence.\n\nDescription: " + truncateForPrompt(*listing.Description, oneLinerDescriptionLimit)},
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

// maxCollectedErrors bounds the sample of failures kept for the returned error.
const maxCollectedErrors = 10

// maxChatAttempts bounds retries per operation. The token-per-minute limit is
// a rolling 60s window, so the attempts have to span enough time for that
// window to drain; five one-second waits gave up after five seconds and failed
// while the window was still full.
const maxChatAttempts = 8

// maxFailedOperationPercent is the share of AI operations that may fail before
// the run is treated as broken rather than merely incomplete.
const maxFailedOperationPercent = 10

// runIsBroken reports whether enough of a run failed that its output should be
// thrown away rather than uploaded. Below the threshold the successful
// listings are still worth keeping: the ones that failed keep their zero
// values, and the upload step leaves stored fields alone rather than
// overwriting them with blanks.
func runIsBroken(failed, total int) bool {
	if failed == 0 {
		return false
	}
	if total <= 0 {
		return true
	}
	return failed*100 > total*maxFailedOperationPercent
}

// retryAfterOrBackoff prefers the server's own Retry-After hint and otherwise
// backs off exponentially with jitter, so a burst of goroutines that all get
// rate limited at once do not retry in lockstep.
func retryAfterOrBackoff(retryAfter string, attempt int) time.Duration {
	if retryAfter != "" {
		if seconds, err := strconv.ParseFloat(retryAfter, 64); err == nil && seconds > 0 {
			// Honour the hint, but still scale it by the attempt and add
			// jitter. OpenAI answers a saturated token-per-minute window with
			// a flat "Retry-After: 1", so obeying it literally puts every
			// worker back on the wire at the same instant, against a window
			// that is still full. That is the lockstep this function exists
			// to avoid.
			hint := time.Duration(seconds * float64(time.Second))
			return hint*time.Duration(attempt) + time.Duration(rand.Int63n(int64(time.Second)))
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
