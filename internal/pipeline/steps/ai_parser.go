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
	budget        *tokenBudget
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
		budget:        newTokenBudget(tokensPerMinuteFromEnv()),
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
	// Two operations per listing: local markup, then the merged enrichment
	// call.
	s.totalJobs = int32(len(listings) * operationsPerListing)
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

			updatedListing, err = s.EnrichListing(ctx, updatedListing)
			if err != nil {
				recordErr("Failed to enrich listing: %v", err)
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

// htmlTagPattern matches any HTML tag, opening or closing. It deliberately
// requires a letter after the "<" so that plain prose like "<5 years" or
// "salary <$100k" is not mistaken for markup.
var htmlTagPattern = regexp.MustCompile(`(?i)</?[a-z][a-z0-9]*(\s[^<>]*)?/?>`)

// invisibleRunePattern matches the whitespace-lookalike runes that leak in from
// scraped pages: non-breaking space, narrow no-break space, figure space,
// zero-width space, and the BOM.
//
// The zero-width joiner and non-joiner used to be in here and deliberately are
// not any more. They are not whitespace lookalikes, they are meaningful: the
// joiner binds an emoji sequence into a single glyph, so replacing it with a
// space tore "\U0001f469\u200d\U0001f4bb" into two glyphs on the board, and the
// non-joiner is load-bearing in Persian and several Indic scripts.
var invisibleRunePattern = regexp.MustCompile("[\u00a0\u202f\u2007\u200b\ufeff]")

// CleanDescription strips the invisible junk the old LLM prompt was asked to
// remove. This is a deterministic substitution, so paying a model to echo a
// 1000-token description back with the same substitution applied was the single
// largest line on the OpenAI bill for no gain in quality.
func CleanDescription(description string) string {
	cleaned := invisibleRunePattern.ReplaceAllString(description, " ")
	cleaned = strings.ReplaceAll(cleaned, "\\n", "")
	return strings.TrimSpace(cleaned)
}

// FixDescriptionFormat normalises the description and gives it HTML structure.
//
// This used to be an API call that echoed the entire description back wrapped
// in tags. It was by far the most expensive of the three AI operations -- the
// description was billed on the way in and again, at four times the rate, on
// the way out -- and it dominated the token-per-minute budget that was getting
// runs rate limited. Paragraphs, lists and headings are punctuation, not
// meaning, so MarkupDescription derives them locally instead: free, instant,
// and incapable of rewording a posting it was only asked to wrap.
func (s *AIParserStep) FixDescriptionFormat(ctx context.Context, listing models.Listing) (models.Listing, error) {
	atomic.AddInt32(&s.completedJobs, 1)
	atomic.AddInt32(&s.skippedJobs, 1)

	if listing.Description == nil || strings.TrimSpace(*listing.Description) == "" {
		return listing, nil
	}

	cleaned := CleanDescription(*listing.Description)

	// Anything that already carries HTML keeps its own markup. The test is any
	// tag, not just block tags: a description using only <strong> or <a> has no
	// block structure to detect, but running it through the markup path would
	// escape those tags and render them to the board as literal text.
	if htmlTagPattern.MatchString(cleaned) {
		// Cleaned again afterwards: the sanitiser decodes entities, so a source
		// writing "&nbsp;" becomes a literal non-breaking space that the first
		// pass could not see.
		*listing.Description = CleanDescription(SanitiseDescriptionHTML(cleaned))
		return listing, nil
	}

	*listing.Description = MarkupDescription(cleaned)
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

// enrichmentInstructions is the static half of the prompt. It is held apart
// from the listing data and sent as the system message so that every request
// in a run shares a byte-identical prefix, which is the shape prompt caching
// needs. Previously these ~900 tokens were re-sent inside the user message of
// every call, which made them the single largest line in the token budget.
const enrichmentInstructions = "You classify job listings and summarise them. Respond with a JSON object holding the keys industry, job_type, discipline and summary.\n\n" +
	"1. INDUSTRY, the employer's sector. One of: CONSULTING, BANKS, BIG_TECH, TECH, QUANT_TRADING, ENERGY, GOVERNMENT, MANUFACTURING, TELECOMMUNICATIONS, RETAIL, TRANSPORTATION, OTHER_INDUSTRY.\n" +
	"2. JOBTYPE, one of: INTERN, GRADUATE, OTHER.\n" +
	"3. DISCIPLINE, the technical discipline of the work itself. One of:\n" +
	"   SOFTWARE - writing or shipping software: software engineering, web, mobile, embedded, devops, cloud, platform, SRE, QA.\n" +
	"   DATA - data science, data engineering, analytics, machine learning, AI research.\n" +
	"   CYBER - cyber security, information security, penetration testing.\n" +
	"   ENGINEERING - non-software engineering: civil, mechanical, electrical, chemical, mining, aerospace.\n" +
	"   OTHER_DISCIPLINE - anything else, including consulting, finance, sales, marketing, HR, legal, supply chain, and general graduate programs with no technical component.\n\n" +
	"Judge DISCIPLINE by the role, not the employer: a marketing role at a software company is OTHER_DISCIPLINE. If the title is generic (e.g. \"Graduate Program\") use the company and description to decide, and prefer OTHER_DISCIPLINE when the work is not clearly technical.\n\n" +
	"4. SUMMARY, one concise sentence describing the job, naming the concrete skills or tools it uses and what they are applied to. Prefer the form 'Apply your [skills] skills on [projects]'. If the role is not technical, describe it plainly in the same length instead of inventing a tech stack."

// enrichmentResponse is the model's reply to the merged prompt.
type enrichmentResponse struct {
	Industry   string `json:"industry"`
	JobType    string `json:"job_type"`
	Discipline string `json:"discipline"`
	Summary    string `json:"summary"`
}

// EnrichListing derives the three classification axes and the one-line summary
// in a single request.
//
// These were two separate calls, which meant two prompts and -- more expensively
// -- two copies of the same description for every listing. Both answers are read
// off the same text, so asking for them together halves the number of requests
// and removes the duplicated description entirely.
func (s *AIParserStep) EnrichListing(ctx context.Context, listing models.Listing) (models.Listing, error) {
	// One listing is one enrichment operation however it turns out, so the
	// count is taken once here rather than at each of the four exits. It used
	// to be taken on the skip and success paths only, and makeChatRequest took
	// it a second time for every call that reached the API -- so a listing the
	// cache could not serve counted twice and one whose call failed counted not
	// at all. The run that surfaced this reported 1561/1076 operations, 145%.
	defer atomic.AddInt32(&s.completedJobs, 1)

	classified := listing.IndustryField != "" && listing.Type != nil && listing.Discipline != ""
	// With no description there is no summary to be had, so a classified
	// listing has nothing left to fetch.
	summarised := (listing.OneLinerDesc != nil && strings.TrimSpace(*listing.OneLinerDesc) != "") ||
		!hasDescription(listing)

	if classified && summarised {
		atomic.AddInt32(&s.skippedJobs, 1)
		utils.Debug("Enrichment already known for %q, skipping API call.", listing.Title)
		return listing, nil
	}

	chatRequest := ChatRequest{
		Model:  "gpt-4o-mini",
		Stream: false,
		// The classification JSON plus one sentence.
		MaxTokens:      200,
		ResponseFormat: enrichmentSchema(),
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "system", Content: enrichmentInstructions},
			{Role: "user", Content: "Title: " + listing.Title +
				"\nCompany: " + listing.Company.Name +
				enrichmentDescriptionSuffix(listing)},
		},
	}

	chatResponse, err := s.makeChatRequest(ctx, chatRequest, "EnrichListing")
	if err != nil {
		return listing, err
	}
	if len(chatResponse.Choices) == 0 {
		return listing, fmt.Errorf("no enrichment returned for %q", listing.Title)
	}

	content := chatResponse.Choices[0].Message.Content

	if !classified {
		industry, jobType, discipline := parseClassification(content, listing.Title)
		listing.IndustryField = industry
		listing.Type = &jobType
		listing.Discipline = discipline
	}

	// Only keep a summary that had a description behind it. The schema makes
	// summary a required field, so a listing with nothing but a title still
	// gets one back -- invented from the title alone. Classification can be
	// made from title and company, but a sentence naming "the concrete skills
	// or tools it uses" cannot, and a plausible invented one is worse on the
	// board than none at all.
	if !summarised && hasDescription(listing) {
		var parsed enrichmentResponse
		if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &parsed); err == nil {
			if summary := strings.TrimSpace(parsed.Summary); summary != "" {
				listing.OneLinerDesc = &summary
			}
		} else {
			utils.Warn("Enrichment reply for %q was not JSON (%v); leaving the summary unset.", listing.Title, err)
		}
	}

	return listing, nil
}

// enrichmentSchema builds the strict JSON schema for the enrichment
// call, derived from the same maps used to interpret the reply so the two
// cannot drift apart.
func enrichmentSchema() map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "job_enrichment",
			"strict": true,
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"industry":   map[string]any{"type": "string", "enum": sortedKeys(industryMap)},
					"job_type":   map[string]any{"type": "string", "enum": sortedKeys(jobTypeMap)},
					"discipline": map[string]any{"type": "string", "enum": sortedKeys(disciplineMap)},
					"summary":    map[string]any{"type": "string"},
				},
				"required":             []string{"industry", "job_type", "discipline", "summary"},
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

// hasDescription reports whether the listing carries description text.
func hasDescription(listing models.Listing) bool {
	return listing.Description != nil && strings.TrimSpace(*listing.Description) != ""
}

// enrichmentDescriptionSuffix appends an excerpt of the description to the
// enrichment prompt when one is available.
func enrichmentDescriptionSuffix(listing models.Listing) string {
	if listing.Description == nil || strings.TrimSpace(*listing.Description) == "" {
		return ""
	}

	return "\nDescription: " + truncateForPrompt(*listing.Description, classificationDescriptionLimit)
}

// oneLinerDescriptionLimit caps how much of a description is sent for the
// one-liner. The prompt only needs the tech stack and the projects, which sit
// near the top of a posting; the tail is boilerplate about benefits and equal
// opportunity that we were paying full input price to send.
const oneLinerDescriptionLimit = 2000

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

// maxCollectedErrors bounds the sample of failures kept for the returned error.
const maxCollectedErrors = 10

// operationsPerListing is how many units of work each listing represents:
// formatting the description and enriching it.
const operationsPerListing = 2

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

// tokensPerMinuteFromEnv reads the account's tokens-per-minute ceiling,
// falling back to the tier this project runs on.
func tokensPerMinuteFromEnv() int {
	if raw := os.Getenv("OPENAI_TOKENS_PER_MINUTE"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
		utils.Warn("OPENAI_TOKENS_PER_MINUTE=%q is not a positive integer; using %d.", raw, defaultTokensPerMinute)
	}
	return defaultTokensPerMinute
}

// openaiBaseURL is where chat completions are sent, overridable for tests.
func openaiBaseURL() string {
	if base := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")); base != "" {
		return base
	}
	return "https://api.openai.com/v1"
}

func (s *AIParserStep) makeChatRequest(ctx context.Context, chatRequest ChatRequest, operationType string) (ChatResponse, error) {
	var chatResponse ChatResponse

	utils.Debug("Making %s API call", operationType)

	requestBody, err := json.Marshal(chatRequest)
	if err != nil {
		return chatResponse, err
	}

	// OPENAI_BASE_URL is the conventional override, and it lets a test drive a
	// real reply through this function without reaching the network.
	openaiEndpoint := strings.TrimSuffix(openaiBaseURL(), "/") + "/chat/completions"
	apiKey := os.Getenv("OPENAI_API_KEY")

	client := &http.Client{Timeout: 2 * time.Minute}

	var resp *http.Response

	promptChars := 0
	for _, m := range chatRequest.Messages {
		promptChars += len(m.Content)
	}
	cost := estimateTokens(promptChars, chatRequest.MaxTokens)

	for attempt := 1; attempt <= maxChatAttempts; attempt++ {
		// Wait for room in the token window, then take a concurrency slot.
		//
		// The order matters. Waiting for tokens while holding a slot parks one
		// of only six slots for as long as a minute, so the other workers
		// cannot even queue and throughput collapses to whatever those parked
		// goroutines eventually do. Budget first, slot second: the budget
		// paces the run and the semaphore only caps requests actually in
		// flight.
		//
		// This sits inside the retry loop because a retry spends tokens too.
		// Charging only the first attempt would let a run that is retrying
		// heavily drift back over the limit, which is the failure this is
		// meant to prevent.
		if s.budget != nil {
			if err := s.budget.Acquire(ctx, cost); err != nil {
				return chatResponse, err
			}
		}
		// The slot is scoped to this attempt: taken here, handed back the
		// moment the response headers are in, whether that is a success, a
		// failure or a 429 about to be retried.
		if err := s.acquireSemaphore(ctx); err != nil {
			return chatResponse, fmt.Errorf("failed to acquire semaphore: %w", err)
		}

		// Build a fresh request per attempt. Reusing one request and cloning it
		// does not rewind the body: http.Request.Clone copies the reader as-is,
		// so every retry after the first sent an empty body against a
		// Content-Length that still described the full payload, and failed
		// outright. Rate limits were therefore fatal rather than retried.
		req, err := http.NewRequestWithContext(ctx, "POST", openaiEndpoint, bytes.NewReader(requestBody))
		if err != nil {
			s.releaseSemaphore()
			return chatResponse, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		r, err := client.Do(req)
		s.releaseSemaphore()
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

	// Read, never add: the operation this call belongs to is counted by its
	// caller, and counting it here too is what put the progress line above 100%.
	current := atomic.LoadInt32(&s.completedJobs)
	total := atomic.LoadInt32(&s.totalJobs)
	utils.Log("AI Parser progress: %d/%d (%.1f%%) - %s returned",
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
