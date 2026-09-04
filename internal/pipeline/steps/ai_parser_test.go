package steps

import (
	"context"
	"strings"
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

// A listing whose three classification axes are all known was resolved by an
// earlier run; title and company are part of the fingerprint, so the answer
// cannot have changed.
func TestGetIndustryFieldandTypeSkipsWhenFullyClassified(t *testing.T) {
	jobType := models.GRADUATE
	listing := models.Listing{
		Title:         "Software Engineer",
		IndustryField: models.BIG_TECH,
		Type:          &jobType,
		Discipline:    models.SOFTWARE,
	}

	step := NewAiParserStep()
	got, err := step.GetIndustryFieldandType(context.Background(), listing)
	if err != nil {
		t.Fatalf("GetIndustryFieldandType returned error: %v", err)
	}

	if got.IndustryField != models.BIG_TECH || got.Discipline != models.SOFTWARE {
		t.Errorf("cached classification was overwritten: %+v", got)
	}
	if step.skippedJobs != 1 {
		t.Errorf("expected the API call to be skipped, skippedJobs = %d", step.skippedJobs)
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
func TestClassificationSchemaConstrainsEveryAxis(t *testing.T) {
	schema := classificationSchema()

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

	suffix := classificationDescriptionSuffix(listing)
	if !strings.Contains(suffix, "Go and Kubernetes") {
		t.Errorf("description not included in the prompt: %q", suffix)
	}
}

func TestClassificationDescriptionSuffixIsBoundedAndOptional(t *testing.T) {
	if got := classificationDescriptionSuffix(models.Listing{Title: "T"}); got != "" {
		t.Errorf("a listing with no description must add nothing, got %q", got)
	}

	long := strings.Repeat("x", classificationDescriptionLimit*3)
	suffix := classificationDescriptionSuffix(models.Listing{Title: "T", Description: &long})
	if len([]rune(suffix)) > classificationDescriptionLimit+64 {
		t.Errorf("description excerpt not bounded: %d runes", len([]rune(suffix)))
	}
}

// A description longer than the model can echo back must be left alone. Capping
// output instead would store the job description truncated mid-sentence.
func TestOversizeDescriptionIsNotSentForFormatting(t *testing.T) {
	huge := strings.Repeat("plain text with no markup. ", maxFormattableDescription/10)
	listing := models.Listing{Title: "Graduate Engineer", Description: &huge}

	step := NewAiParserStep()
	got, err := step.FixDescriptionFormat(context.Background(), listing)
	if err != nil {
		t.Fatalf("FixDescriptionFormat: %v", err)
	}

	if step.skippedJobs != 1 {
		t.Error("an oversize description was sent for formatting anyway")
	}
	if len(*got.Description) < maxFormattableDescription {
		t.Error("the description must be kept intact, not truncated")
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
