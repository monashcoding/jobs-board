package steps

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

func TestApplyClassification(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		industry   models.IndustryField
		jobType    models.JobType
		discipline models.Discipline
	}{
		{
			name:       "full three part response",
			content:    "BIG_TECH#GRADUATE#SOFTWARE",
			industry:   models.BIG_TECH,
			jobType:    models.GRADUATE,
			discipline: models.SOFTWARE,
		},
		{
			name:       "whitespace is tolerated",
			content:    " BANKS # INTERN # DATA ",
			industry:   models.BANKS,
			jobType:    models.INTERN,
			discipline: models.DATA,
		},
		{
			// A non-software engineering role still classifies, so it can be
			// kept and filtered on separately from software.
			name:       "non-software engineering",
			content:    "ENERGY#GRADUATE#ENGINEERING",
			industry:   models.ENERGY,
			jobType:    models.GRADUATE,
			discipline: models.ENGINEERING,
		},
		{
			name:       "non-technical role at a tech company",
			content:    "TECH#GRADUATE#OTHER_DISCIPLINE",
			industry:   models.TECH,
			jobType:    models.GRADUATE,
			discipline: models.OTHER_DISCIPLINE,
		},
		{
			// The model dropping the third field must not cost us the first two.
			name:       "legacy two part response keeps industry and job type",
			content:    "CONSULTING#INTERN",
			industry:   models.CONSULTING,
			jobType:    models.INTERN,
			discipline: models.OTHER_DISCIPLINE,
		},
		{
			name:       "unknown discipline falls back alone",
			content:    "GOVERNMENT#GRADUATE#BIOLOGY",
			industry:   models.GOVERNMENT,
			jobType:    models.GRADUATE,
			discipline: models.OTHER_DISCIPLINE,
		},
		{
			name:       "unknown industry falls back alone",
			content:    "AGRICULTURE#GRADUATE#SOFTWARE",
			industry:   models.OTHER_INDUSTRY,
			jobType:    models.GRADUATE,
			discipline: models.SOFTWARE,
		},
		{
			name:       "garbage response falls back on every axis",
			content:    "I'm sorry, I can't help with that.",
			industry:   models.OTHER_INDUSTRY,
			jobType:    models.OTHER,
			discipline: models.OTHER_DISCIPLINE,
		},
		{
			name:       "empty response falls back on every axis",
			content:    "",
			industry:   models.OTHER_INDUSTRY,
			jobType:    models.OTHER,
			discipline: models.OTHER_DISCIPLINE,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			industry, jobType, discipline := ApplyClassification(tc.content, "Test Listing")

			if industry != tc.industry {
				t.Errorf("industry = %q, want %q", industry, tc.industry)
			}
			if jobType != tc.jobType {
				t.Errorf("jobType = %q, want %q", jobType, tc.jobType)
			}
			if discipline != tc.discipline {
				t.Errorf("discipline = %q, want %q", discipline, tc.discipline)
			}
		})
	}
}

func TestCleanDescription(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips non breaking and narrow spaces",
			input: "Apply\u00a0by\u202f5pm",
			want:  "Apply by 5pm",
		},
		{
			name:  "strips zero width characters and BOM",
			input: "\ufeffGraduate\u200bProgram",
			want:  "Graduate Program",
		},
		{
			name:  "strips literal escaped newlines",
			input: `<p>Role\ndetails</p>`,
			want:  "<p>Roledetails</p>",
		},
		{
			name:  "leaves clean html untouched",
			input: "<p>Build things.</p>",
			want:  "<p>Build things.</p>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanDescription(tc.input); got != tc.want {
				t.Errorf("CleanDescription(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// Descriptions that already carry HTML structure must not reach the API: that
// call was the largest single item on the OpenAI bill and it added nothing.
func TestFixDescriptionFormatSkipsApiCallForHtml(t *testing.T) {
	description := "<p>Work on distributed systems.</p>"
	listing := models.Listing{Title: "Software Engineer", Description: &description}

	step := NewAiParserStep()
	got, err := step.FixDescriptionFormat(context.Background(), listing)
	if err != nil {
		t.Fatalf("FixDescriptionFormat returned error: %v", err)
	}

	if *got.Description != "<p>Work on distributed systems.</p>" {
		t.Errorf("description not cleaned locally: %q", *got.Description)
	}
	if step.skippedJobs != 1 {
		t.Errorf("expected the API call to be skipped, skippedJobs = %d", step.skippedJobs)
	}
}

// A listing whose classification and summary are both known was resolved by an
// earlier run; title and company are part of the fingerprint, so the answer
// cannot have changed.
func TestEnrichListingSkipsWhenFullyEnriched(t *testing.T) {
	jobType := models.GRADUATE
	summary := "Apply your Go skills on payment systems"
	listing := models.Listing{
		Title:         "Software Engineer",
		IndustryField: models.BIG_TECH,
		Type:          &jobType,
		Discipline:    models.SOFTWARE,
		OneLinerDesc:  &summary,
	}

	step := NewAiParserStep()
	got, err := step.EnrichListing(context.Background(), listing)
	if err != nil {
		t.Fatalf("EnrichListing returned error: %v", err)
	}

	if got.IndustryField != models.BIG_TECH || got.Discipline != models.SOFTWARE {
		t.Errorf("cached classification was overwritten: %+v", got)
	}
	if got.OneLinerDesc == nil || *got.OneLinerDesc != summary {
		t.Errorf("cached summary was overwritten: %+v", got.OneLinerDesc)
	}
	if step.skippedJobs != 1 {
		t.Errorf("expected the API call to be skipped, skippedJobs = %d", step.skippedJobs)
	}
}

// A listing that is classified but has no summary still needs the call: the
// enrichment is only skippable when every field it produces is already known.
func TestEnrichListingStillCallsWhenOnlyPartlyKnown(t *testing.T) {
	jobType := models.GRADUATE
	desc := "Build payment systems in Go."
	listing := models.Listing{
		Title:         "Software Engineer",
		IndustryField: models.BIG_TECH,
		Type:          &jobType,
		Discipline:    models.SOFTWARE,
		Description:   &desc,
	}

	step := NewAiParserStep()
	_, _ = step.EnrichListing(context.Background(), listing)

	if step.skippedJobs != 0 {
		t.Error("a missing summary must not be skipped as though it were cached")
	}
}

func TestTruncateForPrompt(t *testing.T) {
	if got := truncateForPrompt("short", 100); got != "short" {
		t.Errorf("unexpected truncation: %q", got)
	}

	// Multi-byte runes must not be cut in half.
	got := truncateForPrompt("héllo wörld", 5)
	if len([]rune(got)) != 5 {
		t.Errorf("expected 5 runes, got %q", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncation produced invalid utf-8: %q", got)
	}
}

// A run in which most operations fail must return an error, not hang. The
// previous implementation sent up to three errors per listing into a channel
// sized to the listing count and drained it only after wg.Wait(), so a
// widespread failure -- an exhausted OpenAI quota returns 429 for every call --
// blocked the senders forever and wedged the scraper.
func TestProcessDoesNotDeadlockWhenEveryOperationFails(t *testing.T) {
	// No API key and an already-cancelled context: every operation fails fast,
	// producing far more errors than there are listings.
	t.Setenv("OPENAI_API_KEY", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	listings := make([]models.Listing, 50)
	for i := range listings {
		description := "plain text description with no markup at all"
		listings[i] = models.Listing{
			Title:       "Software Engineer",
			Company:     models.Company{Name: "Example"},
			Description: &description,
		}
	}

	step := NewAiParserStep()

	done := make(chan struct{})
	var err error
	var got []models.Listing
	go func() {
		got, err = step.Process(ctx, listings)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Process deadlocked instead of returning with errors")
	}

	if err == nil {
		t.Error("expected an error when every operation fails")
	}
	if len(got) != len(listings) {
		t.Errorf("expected %d listings back, got %d", len(listings), len(got))
	}
}

// The pacing sleep must not hold the concurrency slot, or throughput collapses
// to maxConcurrent requests per sleep interval.
func TestReleaseSemaphoreFreesSlotBeforeSleeping(t *testing.T) {
	step := NewAiParserStep()
	step.sleepDuration = 200 * time.Millisecond

	if err := step.acquireSemaphore(context.Background()); err != nil {
		t.Fatalf("acquireSemaphore: %v", err)
	}

	freed := make(chan struct{})
	go func() {
		step.releaseSemaphore()
		close(freed)
	}()

	// The slot must become available well before releaseSemaphore returns.
	deadline := time.After(step.sleepDuration / 2)
	for {
		select {
		case <-deadline:
			t.Fatal("slot was still held during the pacing sleep")
		default:
		}
		if len(step.semaphore) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	<-freed
}

func TestHashSourceDescriptionIsStableAndDistinguishing(t *testing.T) {
	a := HashSourceDescription("Graduate Software Engineer at Example.")
	b := HashSourceDescription("Graduate Software Engineer at Example.")
	c := HashSourceDescription("Graduate Software Engineer at Example!")

	if a != b {
		t.Error("the same source text must hash identically")
	}
	if a == c {
		t.Error("different source text must hash differently")
	}
}

// Sources that return plain text (LinkedIn always, GradConnection about a third
// of the time) need the formatting call. Without reusing the stored result they
// pay for it on every run forever, because the raw text is rescraped each time
// while only the formatted version is stored.
func TestReuseStoredDescriptionWhenSourceUnchanged(t *testing.T) {
	raw := "Plain text job description with no markup."
	formatted := "<p>Plain text job description with no markup.</p>"

	listing := models.Listing{
		Description:           &raw,
		SourceDescriptionHash: HashSourceDescription(raw),
	}
	doc := cachedEnrichment{
		Description:           &formatted,
		SourceDescriptionHash: HashSourceDescription(raw),
	}

	if !reuseStoredDescription(&listing, doc) {
		t.Fatal("expected the stored description to be reused")
	}
	if *listing.Description != formatted {
		t.Errorf("description not swapped for the stored one: %q", *listing.Description)
	}

	// And the reused description must then skip the API call entirely.
	step := NewAiParserStep()
	if _, err := step.FixDescriptionFormat(context.Background(), listing); err != nil {
		t.Fatalf("FixDescriptionFormat: %v", err)
	}
	if step.skippedJobs != 1 {
		t.Errorf("reused description still triggered an API call")
	}
}

func TestReuseStoredDescriptionRejectsChangedSource(t *testing.T) {
	stored := "<p>Old posting.</p>"
	updated := "The posting was rewritten at the source."

	listing := models.Listing{
		Description:           &updated,
		SourceDescriptionHash: HashSourceDescription(updated),
	}
	doc := cachedEnrichment{
		Description:           &stored,
		SourceDescriptionHash: HashSourceDescription("Old posting."),
	}

	if reuseStoredDescription(&listing, doc) {
		t.Error("a changed source description must not reuse the stored formatting")
	}
	if *listing.Description != updated {
		t.Errorf("the freshly scraped description must be kept: %q", *listing.Description)
	}
}

// Documents written before this field existed carry no hash, and must simply
// miss rather than reuse a description that may not match.
func TestReuseStoredDescriptionRejectsMissingHash(t *testing.T) {
	raw := "Plain text description."
	stored := "<p>Plain text description.</p>"

	listing := models.Listing{
		Description:           &raw,
		SourceDescriptionHash: HashSourceDescription(raw),
	}
	doc := cachedEnrichment{Description: &stored} // no stored hash

	if reuseStoredDescription(&listing, doc) {
		t.Error("a document with no stored hash must not be treated as a match")
	}
}

// End-to-end across the two files that have to agree: the parser records the
// source hash, the upload step persists it, and the next run reuses the stored
// formatting instead of paying for it again. This is the whole point of the
// hash, and it breaks silently if either side stops holding up its end.
func TestTwoRunCycleSkipsFormattingOnSecondRun(t *testing.T) {
	const rawText = "Plain text description from LinkedIn with no markup."

	// --- run 1: nothing stored yet, so the description must be formatted ---
	raw := rawText
	run1 := models.Listing{Title: "SWE", Description: &raw}
	run1.SourceDescriptionHash = HashSourceDescription(*run1.Description)

	if reuseStoredDescription(&run1, cachedEnrichment{}) {
		t.Fatal("run 1 must not find anything to reuse")
	}

	// Stand in for the model's formatting result.
	formatted := "<p>" + rawText + "</p>"
	run1.Description = &formatted

	stored := buildListingSet(run1, "fp", "1.0.0", time.Now())
	storedHash, _ := stored["source_description_hash"].(string)
	if storedHash == "" {
		t.Fatal("run 1 must persist the source hash")
	}
	storedDescription, _ := stored["description"].(*string)

	// --- run 2: the same raw text is scraped again ---
	raw2 := rawText
	run2 := models.Listing{Title: "SWE", Description: &raw2}
	run2.SourceDescriptionHash = HashSourceDescription(*run2.Description)

	doc := cachedEnrichment{
		Description:           storedDescription,
		SourceDescriptionHash: storedHash,
	}
	if !reuseStoredDescription(&run2, doc) {
		t.Fatal("run 2 must reuse the stored formatted description")
	}

	step := NewAiParserStep()
	if _, err := step.FixDescriptionFormat(context.Background(), run2); err != nil {
		t.Fatalf("FixDescriptionFormat: %v", err)
	}
	if step.skippedJobs != 1 {
		t.Error("run 2 still made an API call")
	}
}

func TestParseClassificationReadsStructuredReply(t *testing.T) {
	industry, jobType, discipline := parseClassification(
		`{"industry":"QUANT_TRADING","job_type":"INTERN","discipline":"SOFTWARE"}`,
		"Quantitative Trader Internship",
	)

	if industry != models.QUANT_TRADING || jobType != models.INTERN || discipline != models.SOFTWARE {
		t.Errorf("got %v/%v/%v", industry, jobType, discipline)
	}
}

// The delimited fallback keeps working for a reply that predates the schema.
func TestParseClassificationFallsBackToDelimitedFormat(t *testing.T) {
	industry, jobType, discipline := parseClassification("BIG_TECH#GRADUATE#SOFTWARE", "SWE")

	if industry != models.BIG_TECH || jobType != models.GRADUATE || discipline != models.SOFTWARE {
		t.Errorf("got %v/%v/%v", industry, jobType, discipline)
	}
}

// RETAIL and TRANSPORTATION were unreachable: present in models.IndustryField
// but in neither the prompt nor the lookup, so those employers always landed in
// OTHER_INDUSTRY and lost their score.
func TestRetailAndTransportationAreClassifiable(t *testing.T) {
	for raw, want := range map[string]models.IndustryField{
		"RETAIL":         models.RETAIL,
		"TRANSPORTATION": models.TRANSPORTATION,
	} {
		got, _, _ := mapClassification(raw, "GRADUATE", "SOFTWARE", "T")
		if got != want {
			t.Errorf("mapClassification(%q) = %v, want %v", raw, got, want)
		}
	}
}

// The schema is what stops the model emitting "INTERNS" or putting a discipline
// in the industry slot. Job type is a hard gate on board eligibility, so an
// unconstrained near miss silently drops a real job off the board.
func TestEnrichmentSchemaConstrainsEveryAxis(t *testing.T) {
	schema := enrichmentSchema()

	inner, ok := schema["json_schema"].(map[string]any)
	if !ok {
		t.Fatal("json_schema missing")
	}
	if inner["strict"] != true {
		t.Error("schema must be strict, or the enums are advisory only")
	}

	body := inner["schema"].(map[string]any)
	if body["additionalProperties"] != false {
		t.Error("additionalProperties must be false")
	}

	props := body["properties"].(map[string]any)
	for _, axis := range []string{"industry", "job_type", "discipline"} {
		values := props[axis].(map[string]any)["enum"].([]string)
		if len(values) == 0 {
			t.Errorf("%s has no enum constraint", axis)
		}
	}

	// The enums must be the same set used to interpret the reply, or the model
	// can return a value the parser then rejects.
	jobTypes := props["job_type"].(map[string]any)["enum"].([]string)
	if len(jobTypes) != len(jobTypeMap) {
		t.Errorf("job_type enum has %d values, jobTypeMap has %d", len(jobTypes), len(jobTypeMap))
	}
	for _, v := range jobTypes {
		if _, ok := jobTypeMap[v]; !ok {
			t.Errorf("schema offers %q, which the parser does not accept", v)
		}
	}

	industries := props["industry"].(map[string]any)["enum"].([]string)
	if len(industries) != len(industryMap) {
		t.Errorf("industry enum has %d values, industryMap has %d", len(industries), len(industryMap))
	}
}

// "INTERNS" is what actually came back for a vacation program. Constrained
// generation prevents it; the mapping still has to fail safe if it appears.
func TestUnknownJobTypeStillFallsBackSafely(t *testing.T) {
	industry, jobType, discipline := mapClassification("BANKS", "INTERNS", "SOFTWARE", "Vacation Program")

	if jobType != models.OTHER {
		t.Errorf("expected OTHER, got %v", jobType)
	}
	// The other two axes must survive a bad job type.
	if industry != models.BANKS || discipline != models.SOFTWARE {
		t.Errorf("a bad job type cost the other axes: %v/%v", industry, discipline)
	}
}

// The classification prompt tells the model to fall back to the description for
// a generic title. It could not do that while only title and company were sent,
// and since discipline and job type are both hard gates on board eligibility, a
// role it could not place was dropped from the board.
func TestClassificationPromptCarriesTheDescription(t *testing.T) {
	description := "You will build backend services in Go and Kubernetes."
	listing := models.Listing{Title: "Graduate Program", Description: &description}

	suffix := enrichmentDescriptionSuffix(listing)
	if !strings.Contains(suffix, "Go and Kubernetes") {
		t.Errorf("description not included in the prompt: %q", suffix)
	}
}

func TestClassificationDescriptionSuffixIsBoundedAndOptional(t *testing.T) {
	if got := enrichmentDescriptionSuffix(models.Listing{Title: "T"}); got != "" {
		t.Errorf("a listing with no description must add nothing, got %q", got)
	}

	long := strings.Repeat("x", classificationDescriptionLimit*3)
	suffix := enrichmentDescriptionSuffix(models.Listing{Title: "T", Description: &long})
	if len([]rune(suffix)) > classificationDescriptionLimit+64 {
		t.Errorf("description excerpt not bounded: %d runes", len([]rune(suffix)))
	}
}

// Descriptions used to be capped because the model had to echo them back and a
// long one risked being stored truncated mid-sentence. Markup is local now, so
// there is no cap and no call -- but the invariant it protected still holds:
// every word survives, at any size.
func TestOversizeDescriptionIsMarkedUpIntactWithoutAnAPICall(t *testing.T) {
	huge := strings.Repeat("plain text with no markup.\n\n", 2000)
	// FixDescriptionFormat writes through the *string, so snapshot the count
	// before calling or the "before" and "after" are the same string.
	wantWords := len(strings.Fields(huge))
	listing := models.Listing{Title: "Graduate Engineer", Description: &huge}

	step := NewAiParserStep()
	got, err := step.FixDescriptionFormat(context.Background(), listing)
	if err != nil {
		t.Fatalf("FixDescriptionFormat: %v", err)
	}

	if step.skippedJobs != 1 {
		t.Error("formatting must never reach the API now that markup is local")
	}

	gotWords := len(strings.Fields(tagStripper.ReplaceAllString(*got.Description, " ")))
	if gotWords != wantWords {
		t.Errorf("description was not kept intact: got %d words, want %d", gotWords, wantWords)
	}
}

// A source that already returned HTML keeps its own markup: re-wrapping it
// would escape the tags it arrived with and render them as visible text.
func TestExistingHTMLDescriptionIsLeftAlone(t *testing.T) {
	original := "<p>Already <strong>formatted</strong>.</p>"
	listing := models.Listing{Title: "Engineer", Description: &original}

	got, err := NewAiParserStep().FixDescriptionFormat(context.Background(), listing)
	if err != nil {
		t.Fatalf("FixDescriptionFormat: %v", err)
	}
	if *got.Description != original {
		t.Errorf("existing HTML was rewritten: got %q", *got.Description)
	}
}

// Re-keying collapses duplicate documents onto one fingerprint, and they are
// only deleted in a later migration phase. Until then the cache sees several
// rows for one key and must not let an empty one win, or it re-derives fields
// that were already stored.
func TestCacheKeepsTheRichestOfCollidingDocuments(t *testing.T) {
	oneLiner := "Apply your Go skills on distributed systems"
	jobType := models.GRADUATE
	formatted := "<p>Formatted.</p>"

	rich := cachedEnrichment{
		Fingerprint:           "fp",
		OneLinerDesc:          &oneLiner,
		IndustryField:         models.BIG_TECH,
		Type:                  &jobType,
		Discipline:            models.SOFTWARE,
		Description:           &formatted,
		SourceDescriptionHash: "hash",
	}
	empty := cachedEnrichment{Fingerprint: "fp"}

	if enrichmentScore(rich) <= enrichmentScore(empty) {
		t.Fatalf("scoring is wrong: rich=%d empty=%d", enrichmentScore(rich), enrichmentScore(empty))
	}

	// Whichever order the driver returns them in, the rich document must win.
	for _, order := range [][]cachedEnrichment{{rich, empty}, {empty, rich}} {
		cache := map[string]cachedEnrichment{}
		for _, doc := range order {
			if existing, clash := cache[doc.Fingerprint]; clash && enrichmentScore(existing) >= enrichmentScore(doc) {
				continue
			}
			cache[doc.Fingerprint] = doc
		}
		if cache["fp"].OneLinerDesc == nil {
			t.Error("the emptier colliding document won")
		}
	}
}

// A document stored before the schema existed has a job type but no discipline.
// Filling the job type from that document while the classification re-runs made
// the stale value permanent: the fresh call assigns a type only when none is
// set, so a listing wrongly stored as OTHER could never recover -- and job type
// is a hard gate on board eligibility.
func TestPartialCachedClassificationIsNotApplied(t *testing.T) {
	stale := models.OTHER
	doc := cachedEnrichment{
		Fingerprint:   "fp",
		IndustryField: models.BANKS,
		Type:          &stale,
		Discipline:    "", // never scored
	}

	listing := models.Listing{Title: "Vacation Program"}

	// Mirror what applyCachedEnrichment does for one listing.
	if doc.IndustryField != "" && doc.Type != nil && doc.Discipline != "" {
		if listing.Type == nil {
			listing.Type = doc.Type
		}
	}

	if listing.Type != nil {
		t.Error("a partial cached classification must not be applied; the stale job type would become permanent")
	}
}

func TestCompleteCachedClassificationIsApplied(t *testing.T) {
	jobType := models.INTERN
	doc := cachedEnrichment{
		Fingerprint:   "fp",
		IndustryField: models.BANKS,
		Type:          &jobType,
		Discipline:    models.SOFTWARE,
	}

	listing := models.Listing{Title: "Vacation Program"}

	if doc.IndustryField != "" && doc.Type != nil && doc.Discipline != "" {
		if listing.IndustryField == "" {
			listing.IndustryField = doc.IndustryField
		}
		if listing.Type == nil {
			listing.Type = doc.Type
		}
		if listing.Discipline == "" {
			listing.Discipline = doc.Discipline
		}
	}

	if listing.Type == nil || *listing.Type != models.INTERN || listing.Discipline != models.SOFTWARE {
		t.Error("a complete cached classification must be reused in full")
	}
}

// A saturated token-per-minute window makes OpenAI answer with a flat
// "Retry-After: 1". Obeying that literally sent every worker back at the same
// instant, so the waits have to grow and be jittered even when the hint exists.
func TestRetryAfterHintIsScaledAndJittered(t *testing.T) {
	const hint = "1"

	first := retryAfterOrBackoff(hint, 1)
	if first < time.Second || first >= 2*time.Second {
		t.Fatalf("attempt 1 wait = %s, want [1s, 2s)", first)
	}

	later := retryAfterOrBackoff(hint, 4)
	if later < 4*time.Second {
		t.Fatalf("attempt 4 wait = %s, want at least 4s", later)
	}

	// Jitter has to actually vary, or the workers stay in lockstep.
	seen := make(map[time.Duration]bool)
	for i := 0; i < 50; i++ {
		seen[retryAfterOrBackoff(hint, 1)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("Retry-After waits never varied across 50 calls, workers stay in lockstep")
	}
}

// Three rate limited operations out of ~2000 used to abort the pipeline, which
// discarded every listing in the run. A small number of failures must leave the
// successful listings intact so board scoring and the upload still run.
func TestRunIsBroken(t *testing.T) {
	cases := []struct {
		name   string
		failed int
		total  int
		want   bool
	}{
		{name: "clean run", failed: 0, total: 1971, want: false},
		{name: "the run that lost a whole scrape", failed: 3, total: 1971, want: false},
		{name: "at the threshold", failed: 197, total: 1971, want: false},
		{name: "past the threshold", failed: 198, total: 1971, want: true},
		{name: "exhausted quota fails everything", failed: 1971, total: 1971, want: true},
		{name: "unknown total is not trusted", failed: 1, total: 0, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIsBroken(tc.failed, tc.total); got != tc.want {
				t.Fatalf("runIsBroken(%d, %d) = %v, want %v", tc.failed, tc.total, got, tc.want)
			}
		})
	}
}

// The schema makes summary required, so a listing with no description still
// gets a sentence back -- invented from the title. Storing that would put a
// confident, unfounded description of the work on the board.
func TestEnrichListingDoesNotInventASummaryWithoutADescription(t *testing.T) {
	jobType := models.GRADUATE
	listing := models.Listing{
		Title:         "Graduate Program",
		Company:       models.Company{Name: "Example"},
		IndustryField: models.BIG_TECH,
		Type:          &jobType,
		Discipline:    models.SOFTWARE,
	}

	step := NewAiParserStep()
	got, err := step.EnrichListing(context.Background(), listing)
	if err != nil {
		t.Fatalf("EnrichListing: %v", err)
	}

	if got.OneLinerDesc != nil {
		t.Errorf("a summary was invented with no description to base it on: %q", *got.OneLinerDesc)
	}
	if step.skippedJobs != 1 {
		t.Error("with no description there is nothing to fetch, so the call must be skipped")
	}
}

// Classification is made from title and company, so a listing with no
// description still needs the call. Only the summary depends on description
// text, and skipping the whole operation would leave the listing unclassified
// -- which means no discipline, which means it silently never reaches the board.
func TestEnrichListingStillClassifiesWithoutADescription(t *testing.T) {
	listing := models.Listing{
		Title:   "Graduate Software Engineer",
		Company: models.Company{Name: "Example"},
	}

	step := NewAiParserStep()
	_, _ = step.EnrichListing(context.Background(), listing)

	if step.skippedJobs != 0 {
		t.Error("an unclassified listing was skipped, so it would never gain a discipline")
	}
}

// Every listing is one enrichment operation, whatever becomes of it. The count
// used to be taken on the skip and success paths only, while makeChatRequest
// took it again for every call that reached the API: a listing the cache could
// not serve counted twice, and one whose call failed counted not at all. The
// run that surfaced this logged 1561/1076 operations -- 145% -- which makes the
// progress line useless for judging whether a scrape is healthy.
func TestEnrichListingCountsEachListingExactlyOnce(t *testing.T) {
	jobType := models.GRADUATE
	summary := "Apply your Go skills on payment systems"
	desc := "Build payment systems in Go."

	cached := models.Listing{
		Title:         "Software Engineer",
		IndustryField: models.BIG_TECH,
		Type:          &jobType,
		Discipline:    models.SOFTWARE,
		OneLinerDesc:  &summary,
	}
	needsCall := models.Listing{
		Title:       "Software Engineer",
		Description: &desc,
	}

	// A reply good enough to take the success path all the way through.
	enriched := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":`+
			`"{\"industry\":\"Big Tech\",\"job_type\":\"Graduate\",`+
			`\"discipline\":\"Software\",\"summary\":\"Build payment systems in Go\"}"}}]}`)
	}))
	defer enriched.Close()

	// A 500 no retry can rescue: the failure path, which used to go uncounted.
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()

	tests := []struct {
		name    string
		listing models.Listing
		baseURL string
	}{
		{"served from cache, no call made", cached, enriched.URL},
		{"call made and answered", needsCall, enriched.URL},
		{"call made and failed", needsCall, failing.URL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "test-key")
			t.Setenv("OPENAI_BASE_URL", tt.baseURL)

			step := NewAiParserStep()
			_, _ = step.EnrichListing(context.Background(), tt.listing)

			if got := atomic.LoadInt32(&step.completedJobs); got != 1 {
				t.Errorf("expected the listing to count as exactly one operation, completedJobs = %d", got)
			}
		})
	}
}

// The progress line is emitted from the transport, which must report the count
// its callers keep rather than adding one of its own.
func TestMakeChatRequestDoesNotCountTheOperationItself(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer server.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", server.URL)

	step := NewAiParserStep()
	_, _ = step.makeChatRequest(context.Background(), ChatRequest{Model: "gpt-4o-mini"}, "EnrichListing")

	if got := atomic.LoadInt32(&step.completedJobs); got != 0 {
		t.Errorf("makeChatRequest must not advance the progress count, completedJobs = %d", got)
	}
}

// The zero-width joiner is not whitespace junk, it is what binds an emoji
// sequence into one glyph. Replacing it with a space tore the sequence into
// separate glyphs on the board; the non-joiner is load-bearing in Persian and
// several Indic scripts for the same reason.
func TestCleanDescriptionKeepsMeaningfulJoiners(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"emoji joined by a zero width joiner", "Meet the team \U0001f469‍\U0001f4bb today"},
		{"zero width non joiner between letters", "می‌خواهم"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanDescription(tc.input); got != tc.input {
				t.Errorf("CleanDescription(%q) = %q, want it unchanged", tc.input, got)
			}
		})
	}
}

// Four of the six sources store their descriptions as HTML, so the markup was
// going to OpenAI with the text -- paid for on the bill, and again on quality,
// since it displaces real content inside the prompt's description cap.
func TestDescriptionForPromptRemovesMarkup(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "block tags become word boundaries",
			input: "<p>We build payment systems.</p><p>Apply now.</p>",
			want:  "We build payment systems. Apply now.",
		},
		{
			name:  "list items do not fuse together",
			input: "<ul><li>Monitor health</li><li>Tune queries</li></ul>",
			want:  "Monitor health Tune queries",
		},
		{
			name:  "inline tags do not split a sentence",
			input: "<p>We use <strong>Go</strong> and Kubernetes.</p>",
			want:  "We use Go and Kubernetes.",
		},
		{
			name:  "entities are decoded",
			input: "<p>Research &amp; development, 5&nbsp;days.</p>",
			want:  "Research & development, 5 days.",
		},
		{
			name:  "layout whitespace is collapsed",
			input: "<p>Line one</p>\n\n\n   <p>Line two</p>",
			want:  "Line one Line two",
		},
		{
			name:  "plain text passes through unchanged",
			input: "Turn on notifications for nib's Graduate Jobs.",
			want:  "Turn on notifications for nib's Graduate Jobs.",
		},
		{
			name:  "a multi-line tag is still a tag",
			input: "<a\n  href=\"https://example.com\">our portal</a> opens Friday.",
			want:  "our portal opens Friday.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := descriptionForPrompt(tc.input); got != tc.want {
				t.Errorf("descriptionForPrompt(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// Removing tags outright would fuse the words either side of them, which is the
// same defect that made the board unreadable. The model should not be handed a
// mangled description just because it cannot complain about it.
func TestDescriptionForPromptDoesNotFuseWords(t *testing.T) {
	got := descriptionForPrompt("<p>Improves operational resilience.</p><p>What will your day look like?</p>")

	if strings.Contains(got, "resilience.What") {
		t.Errorf("words fused across a tag boundary: %q", got)
	}
}

// The cap is what the description costs and what the model gets to read. Markup
// inside it is budget spent on nothing.
func TestPromptBudgetIsSpentOnProseNotMarkup(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString(fmt.Sprintf(`<p class="mb-4 text-sm leading-relaxed">Sentence number %d.</p>`, i))
	}
	listing := models.Listing{Description: ptr(b.String())}

	suffix := enrichmentDescriptionSuffix(listing)

	// With the markup gone, far more sentences fit inside the same cap.
	sentences := strings.Count(suffix, "Sentence number")
	raw := truncateForPrompt(b.String(), classificationDescriptionLimit)
	if rawSentences := strings.Count(raw, "Sentence number"); sentences <= rawSentences {
		t.Errorf("stripping markup should fit more of the posting in the same budget: %d vs %d",
			sentences, rawSentences)
	}
	if strings.Contains(suffix, "<p") || strings.Contains(suffix, "class=") {
		t.Errorf("markup reached the prompt: %q", suffix[:200])
	}
}

func ptr(s string) *string { return &s }

func TestParseWorkingRightsReadsTheArray(t *testing.T) {
	content := `{"industry":"BANKS","job_type":"GRADUATE","discipline":"SOFTWARE",` +
		`"working_rights":["AUS_CITIZEN_PR","INTERNATIONAL"],"summary":"s"}`

	rights := parseWorkingRights(content, "Graduate Engineer")

	if len(rights) != 2 {
		t.Fatalf("expected 2 rights, got %v", rights)
	}
	if rights[0] != models.AUS_CITIZEN_PR || rights[1] != models.INTERNATIONAL {
		t.Errorf("unexpected rights %v", rights)
	}
}

// An empty array is the answer for most postings: they say nothing about work
// rights. It has to stay empty rather than becoming OTHER_RIGHTS, which on the
// board reads as a restriction the employer never stated.
func TestParseWorkingRightsKeepsSilenceSilent(t *testing.T) {
	content := `{"industry":"TECH","job_type":"GRADUATE","discipline":"SOFTWARE",` +
		`"working_rights":[],"summary":"s"}`

	if rights := parseWorkingRights(content, "Graduate Engineer"); len(rights) != 0 {
		t.Errorf("expected no rights, got %v", rights)
	}
}

func TestParseWorkingRightsIgnoresUnknownAndDuplicateValues(t *testing.T) {
	content := `{"working_rights":["AUS_CITIZEN_PR","OTHER_RIGHTS","AUS_CITIZEN_PR"]}`

	rights := parseWorkingRights(content, "Graduate Engineer")

	if len(rights) != 1 || rights[0] != models.AUS_CITIZEN_PR {
		t.Errorf("expected only AUS_CITIZEN_PR, got %v", rights)
	}
}

// The delimited fallback format predates working rights and never carried them,
// so a non-JSON reply yields nothing rather than a garbled guess.
func TestParseWorkingRightsIgnoresNonJSONReplies(t *testing.T) {
	if rights := parseWorkingRights("BANKS#GRADUATE#SOFTWARE", "T"); rights != nil {
		t.Errorf("expected nil, got %v", rights)
	}
}

func TestEnrichmentSchemaConstrainsWorkingRights(t *testing.T) {
	body := enrichmentSchema()["json_schema"].(map[string]any)["schema"].(map[string]any)
	props := body["properties"].(map[string]any)

	field, ok := props["working_rights"].(map[string]any)
	if !ok {
		t.Fatal("working_rights missing from the schema")
	}
	if field["type"] != "array" {
		t.Errorf("working_rights must be an array, got %v", field["type"])
	}

	values := field["items"].(map[string]any)["enum"].([]string)
	if len(values) != len(workingRightMap) {
		t.Errorf("schema offers %d rights, workingRightMap accepts %d", len(values), len(workingRightMap))
	}
	for _, v := range values {
		if _, ok := workingRightMap[v]; !ok {
			t.Errorf("schema offers %q, which the parser does not accept", v)
		}
	}

	// OTHER_RIGHTS means "a source label we do not recognise". The model has no
	// source label, so offering it would only give it somewhere to put doubt.
	if _, ok := workingRightMap["OTHER_RIGHTS"]; ok {
		t.Error("the model must not be able to return OTHER_RIGHTS")
	}

	required := body["required"].([]string)
	found := false
	for _, r := range required {
		if r == "working_rights" {
			found = true
		}
	}
	if !found {
		t.Error("working_rights must be required, or a strict schema rejects the call")
	}
}

// The prompt is the only thing standing between "Junior Software Engineer" and
// a coin flip: job type is a hard board gate, and ten byte-identical junior
// titles previously came back classified both ways.
func TestPromptDefinesWhereGraduateEnds(t *testing.T) {
	for _, phrase := range []string{"Junior", "Entry Level", "Associate", "MINIMUM", "ceilings", "floors"} {
		if !strings.Contains(enrichmentInstructions, phrase) {
			t.Errorf("the job type definition no longer mentions %q", phrase)
		}
	}
}

// The line this was built for. It sat at character 2274 of a real posting --
// past the 2000-character cut the description is truncated to -- inside a run
// of prose with no full stop, newline or list item anywhere near it.
func TestEligibilityExcerptFindsRightsStatedPastTheTruncation(t *testing.T) {
	description := "<p>" + strings.Repeat("Boilerplate about the team and the role. ", 60) +
		"Enough About Us, Let's Talk About You Australian citizenship is required for this role " +
		"You may have all or some of the following skills</p>"

	excerpt := eligibilityExcerpt(description)

	if !strings.Contains(excerpt, "Australian citizenship is required") {
		t.Errorf("the eligibility statement was not lifted out: %q", excerpt)
	}
	if strings.Contains(truncateForPrompt(descriptionForPrompt(description), classificationDescriptionLimit),
		"Australian citizenship") {
		t.Error("the fixture no longer places the statement past the truncation, so it proves nothing")
	}
}

// A posting that says nothing about work rights must add nothing to the prompt.
func TestEligibilityExcerptIsEmptyWhenNothingIsStated(t *testing.T) {
	description := "<p>We are hiring a graduate engineer to work on our platform team.</p>"

	if excerpt := eligibilityExcerpt(description); excerpt != "" {
		t.Errorf("expected no excerpt, got %q", excerpt)
	}
}

func TestEligibilityExcerptQuotesAroundTheMatchNotFromTheStart(t *testing.T) {
	description := "<p>" + strings.Repeat("filler ", 200) + "Applicants must hold a valid visa.</p>"

	excerpt := eligibilityExcerpt(description)

	if !strings.Contains(excerpt, "must hold a valid visa") {
		t.Errorf("expected the clause around the match, got %q", excerpt)
	}
	if len(excerpt) > eligibilityExcerptLimit {
		t.Errorf("excerpt is %d chars, over the %d budget", len(excerpt), eligibilityExcerptLimit)
	}
}

// Three mentions in consecutive clauses are one passage, and quoting each
// separately would spend the budget restating the same sentence.
func TestEligibilityExcerptMergesOverlappingMentions(t *testing.T) {
	description := "<p>We sponsor visas for citizens of any country, including permanent resident applicants.</p>"

	if n := strings.Count(eligibilityExcerpt(description), " ... "); n != 0 {
		t.Errorf("expected one merged window, got %d joins", n)
	}
}

// However long the posting, and however often it mentions work rights, the
// addition to the prompt stays bounded.
func TestEligibilityExcerptStaysWithinItsBudget(t *testing.T) {
	description := "<p>" + strings.Repeat("Australian citizens only. "+strings.Repeat("filler ", 80), 20) + "</p>"

	if excerpt := eligibilityExcerpt(description); len(excerpt) > eligibilityExcerptLimit {
		t.Errorf("excerpt is %d chars, over the %d budget", len(excerpt), eligibilityExcerptLimit)
	}
}

// A classification is a gate, not a piece of writing. At the API's default
// temperature the same listing came back both ways on consecutive calls.
func TestEnrichmentAsksForADeterministicAnswer(t *testing.T) {
	if deterministic != 0 {
		t.Errorf("classification temperature is %v, want 0", deterministic)
	}
}

// A listing already on the board is re-asked for its working rights and keeps
// the classification that put it there. Job type is a hard board gate, so
// re-deriving it could only agree or take a live listing off the board.
func TestBoardListingGainsRightsWithoutRiskingItsClassification(t *testing.T) {
	// The model is told the role is professional-level and restricted. Only the
	// second of those may reach the listing.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":`+
			`"{\"industry\":\"BIG_TECH\",\"job_type\":\"OTHER\",\"discipline\":\"DATA\",`+
			`\"working_rights\":[\"AUS_CITIZEN_PR\"],\"summary\":\"s\"}"}}]}`)
	}))
	defer server.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", server.URL)

	jobType := models.GRADUATE
	summary := "Apply your Go skills on payment systems"
	desc := "Build payment systems in Go."
	listing := models.Listing{
		Title:         "Graduate Software Engineer",
		Company:       models.Company{Name: "Atlassian"},
		SourceURLs:    []string{"https://example.com/jobs/1"},
		Description:   &desc,
		IndustryField: models.BIG_TECH,
		Type:          &jobType,
		Discipline:    models.SOFTWARE,
		OneLinerDesc:  &summary,
	}

	step := NewAiParserStep()
	step.rightsOnly = map[string]bool{GenerateListingFingerprint(listing): true}

	got, err := step.EnrichListing(context.Background(), listing)
	if err != nil {
		t.Fatalf("EnrichListing returned error: %v", err)
	}

	if got.Type == nil || *got.Type != models.GRADUATE {
		t.Errorf("job type was re-derived, which can take a live listing off the board: %v", got.Type)
	}
	if got.Discipline != models.SOFTWARE {
		t.Errorf("discipline was re-derived: %v", got.Discipline)
	}
	if len(got.WorkingRights) != 1 || got.WorkingRights[0] != models.AUS_CITIZEN_PR {
		t.Errorf("the rights the call was made for were not taken: %v", got.WorkingRights)
	}
}

// Without the marker the same fully-enriched listing must still skip, or every
// listing with no working rights becomes a call on every run forever.
func TestAFullyEnrichedListingWithoutRightsStillSkips(t *testing.T) {
	jobType := models.GRADUATE
	summary := "Apply your Go skills on payment systems"
	listing := models.Listing{
		Title:         "Graduate Software Engineer",
		IndustryField: models.BIG_TECH,
		Type:          &jobType,
		Discipline:    models.SOFTWARE,
		OneLinerDesc:  &summary,
	}

	step := NewAiParserStep()
	if _, err := step.EnrichListing(context.Background(), listing); err != nil {
		t.Fatalf("EnrichListing returned error: %v", err)
	}

	if step.skippedJobs != 1 {
		t.Error("a listing with no rights and no marker must not trigger a call")
	}
}

func TestEnrichmentScoreCountsWhatIsExpensiveToRederive(t *testing.T) {
	summary := "Apply your Go skills on payment systems"
	desc := "Build payment systems in Go."
	jobType := models.GRADUATE
	empty := models.JobType("")

	full := models.Listing{
		OneLinerDesc:          &summary,
		IndustryField:         models.BIG_TECH,
		Type:                  &jobType,
		Discipline:            models.SOFTWARE,
		WorkingRights:         []models.WorkingRight{models.INTERNATIONAL},
		Description:           &desc,
		SourceDescriptionHash: "abc123",
	}

	if got := EnrichmentScore(full); got != 6 {
		t.Errorf("a fully enriched listing scored %d, want 6", got)
	}
	if got := EnrichmentScore(models.Listing{}); got != 0 {
		t.Errorf("a bare listing scored %d, want 0", got)
	}

	// A pointer to "" is not a job type. Documents in that state exist, and
	// counting them would let one beat a document with a real classification.
	if EnrichmentScore(models.Listing{Type: &empty}) != 0 {
		t.Error("an empty job type must not count as enrichment")
	}
	if EnrichmentScore(models.Listing{Type: &jobType}) != 1 {
		t.Error("a real job type must count")
	}

	// A formatted description only counts when the hash that proves what
	// produced it is there too; without it the description cannot be reused.
	if EnrichmentScore(models.Listing{Description: &desc}) != 0 {
		t.Error("a description with no source hash must not count")
	}
}
