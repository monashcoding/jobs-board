package steps

import (
	"testing"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

// listing builds a listing whose source URL is derived from its title, matching
// the real world where each posting has its own stable source page.
func listing(title, company, applicationURL string, locations ...models.LocationType) models.Listing {
	return models.Listing{
		Title:          title,
		Company:        models.Company{Name: company},
		ApplicationURL: &applicationURL,
		SourceURLs:     []string{"https://source.example.com/jobs/" + company + "/" + title},
		Locations:      locations,
	}
}

// sourced overrides the source URL, which is what distinguishes two genuinely
// separate postings that happen to share a title.
func sourced(l models.Listing, sourceURL string) models.Listing {
	l.SourceURLs = []string{sourceURL}
	return l
}

func TestGenerateListingFingerprint(t *testing.T) {
	cases := []struct {
		name string
		a    models.Listing
		b    models.Listing
		same bool
	}{
		{
			// Real churn observed in production: the source replaced Deloitte's
			// careers URL with a bit.ly shortlink, which used to mint a second
			// document and a duplicate Discord thread.
			name: "application URL churn does not change identity",
			a:    listing("Cyber Graduate Program | 2027", "Deloitte", "https://www.deloitte.com/au/en/careers/students/skills-cyber.html?id=au:2el:3or", models.NSW),
			b:    listing("Cyber Graduate Program | 2027", "Deloitte", "https://bit.ly/47mpcuz", models.NSW),
			same: true,
		},
		{
			// Thales migrated ATS from Workday to their own careers site.
			name: "ATS migration does not change identity",
			a:    listing("Graduate Software Engineer", "Thales", "https://thales.wd3.myworkdayjobs.com/careers/job/rydalmere/graduate-software-engineer_r0313813-1", models.NSW),
			b:    listing("Graduate Software Engineer", "Thales", "https://careers.thalesgroup.com/global/en/job/tgptgwglobalr0317371", models.NSW),
			same: true,
		},
		{
			// The same source page with its title reworded, which is why both
			// sides share a source URL here.
			name: "cosmetic title punctuation does not change identity",
			a:    sourced(listing("Cyber Graduate Program | 2027", "Deloitte", "https://example.com/a", models.VIC), "https://au.gradconnection.com/employers/deloitte/jobs/cyber"),
			b:    sourced(listing("Cyber Graduate Program - 2027", "Deloitte", "https://example.com/a", models.VIC), "https://au.gradconnection.com/employers/deloitte/jobs/cyber"),
			same: true,
		},
		{
			name: "title casing does not change identity",
			a:    listing("Graduate Software Engineer", "Atlassian", "https://example.com/a", models.VIC),
			b:    listing("GRADUATE SOFTWARE ENGINEER", "Atlassian", "https://example.com/a", models.VIC),
			same: true,
		},
		{
			name: "location order does not change identity",
			a:    listing("Graduate Engineer", "Telstra", "https://example.com/a", models.VIC, models.NSW),
			b:    listing("Graduate Engineer", "Telstra", "https://example.com/a", models.NSW, models.VIC),
			same: true,
		},
		{
			// Digits survive normalisation, so consecutive intakes stay separate
			// listings and each gets its own thread.
			name: "different intake years are different listings",
			a:    listing("Backend Software Engineer Intern - 2026 Start", "TikTok", "https://example.com/a", models.NSW),
			b:    listing("Backend Software Engineer Intern - 2027 Start", "TikTok", "https://example.com/a", models.NSW),
			same: false,
		},
		{
			// Dropping locations from the key was measured to merge genuinely
			// distinct postings, so they must remain part of the identity.
			name: "same role in a different state is a different listing",
			a:    listing("Graduate Software Developer 2027", "Susquehanna International Group", "https://example.com/a", models.NSW),
			b:    listing("Graduate Software Developer 2027", "Susquehanna International Group", "https://example.com/a", models.VIC),
			same: false,
		},
		{
			// Regression: Jane Street lists several roles called "Software
			// Engineer" that differ only by position id. A fingerprint built
			// from title+company+locations alone collapsed them into one
			// document, losing real jobs.
			name: "same title at the same company with different source URLs stays distinct",
			a:    sourced(listing("Software Engineer", "Jane Street", "https://www.janestreet.com/join-jane-street/position/8140274002/", models.OTHERS), "https://www.janestreet.com/join-jane-street/position/8140274002/"),
			b:    sourced(listing("Software Engineer", "Jane Street", "https://www.janestreet.com/join-jane-street/position/7582902002/", models.OTHERS), "https://www.janestreet.com/join-jane-street/position/7582902002/"),
			same: false,
		},
		{
			name: "trailing slash on the source URL does not change identity",
			a:    sourced(listing("Graduate Engineer", "Telstra", "https://example.com/a", models.VIC), "https://au.gradconnection.com/employers/telstra/jobs/grad/"),
			b:    sourced(listing("Graduate Engineer", "Telstra", "https://example.com/a", models.VIC), "https://au.gradconnection.com/employers/telstra/jobs/grad"),
			same: true,
		},
		{
			name: "different company is a different listing",
			a:    listing("Graduate Software Engineer", "Thales", "https://example.com/a", models.NSW),
			b:    listing("Graduate Software Engineer", "Leidos", "https://example.com/a", models.NSW),
			same: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := GenerateListingFingerprint(tc.a)
			b := GenerateListingFingerprint(tc.b)

			if tc.same && a != b {
				t.Errorf("expected the same fingerprint, got %s and %s", a[:12], b[:12])
			}
			if !tc.same && a == b {
				t.Errorf("expected different fingerprints, both were %s", a[:12])
			}
		})
	}
}

func TestGenerateListingFingerprintIgnoresNilApplicationURL(t *testing.T) {
	withURL := listing("Graduate Engineer", "Telstra", "https://example.com/a", models.VIC)

	withoutURL := withURL
	withoutURL.ApplicationURL = nil

	if GenerateListingFingerprint(withURL) != GenerateListingFingerprint(withoutURL) {
		t.Error("a nil application URL must not change the fingerprint")
	}
}

// A listing whose AI enrichment failed (an exhausted OpenAI quota returns 429
// for every call in the run) must not null out the values a previous run stored.
func TestBuildListingSetOmitsMissingEnrichment(t *testing.T) {
	listing := models.Listing{
		Title:   "Software Engineer",
		Company: models.Company{Name: "Atlassian"},
	}

	set := buildListingSet(listing, "fp", "1.0.0", time.Now())

	for _, field := range []string{"one_liner", "type", "industry_field", "discipline", "board_score", "board_eligible"} {
		if _, ok := set[field]; ok {
			t.Errorf("%q written despite the listing not being enriched; this erases stored data", field)
		}
	}

	if set["title"] != "Software Engineer" {
		t.Errorf("scraped fields should still be written, got %v", set["title"])
	}
}

func TestBuildListingSetWritesPresentEnrichment(t *testing.T) {
	oneLiner := "Apply your Go skills on distributed systems"
	jobType := models.GRADUATE
	listing := models.Listing{
		Title:         "Software Engineer",
		OneLinerDesc:  &oneLiner,
		Type:          &jobType,
		IndustryField: models.BIG_TECH,
		Discipline:    models.SOFTWARE,
		BoardScore:    7,
		BoardEligible: true,
		CompanyTier:   utils.TierHeadline,
	}

	set := buildListingSet(listing, "fp", "1.0.0", time.Now())

	if set["one_liner"] != &oneLiner {
		t.Errorf("one_liner not written: %v", set["one_liner"])
	}
	if set["industry_field"] != models.BIG_TECH {
		t.Errorf("industry_field not written: %v", set["industry_field"])
	}
	if set["discipline"] != models.SOFTWARE {
		t.Errorf("discipline not written: %v", set["discipline"])
	}
	if set["board_score"] != 7 {
		t.Errorf("board_score not written: %v", set["board_score"])
	}
	// The bot orders its weekly recap by this, so it has to survive the upload
	// alongside the score it is scored with.
	if set["company_tier"] != utils.TierHeadline {
		t.Errorf("company_tier not written: %v", set["company_tier"])
	}
}

// An empty one-liner is as destructive as a nil one: it replaces a good stored
// value with a blank string.
func TestBuildListingSetOmitsEmptyOneLiner(t *testing.T) {
	empty := ""
	listing := models.Listing{Title: "Software Engineer", OneLinerDesc: &empty}

	if _, ok := buildListingSet(listing, "fp", "1.0.0", time.Now())["one_liner"]; ok {
		t.Error("empty one_liner written; this erases the stored value")
	}
}

func TestBuildListingSetWritesSourceDescriptionHash(t *testing.T) {
	l := models.Listing{Title: "Software Engineer", SourceDescriptionHash: "abc123"}

	if buildListingSet(l, "fp", "1.0.0", time.Now())["source_description_hash"] != "abc123" {
		t.Error("source_description_hash not written")
	}
}

// A run with the AI parser disabled produces no hash, and must not clear the
// stored one -- that would force every description to be reformatted.
func TestBuildListingSetOmitsEmptySourceDescriptionHash(t *testing.T) {
	l := models.Listing{Title: "Software Engineer"}

	if _, ok := buildListingSet(l, "fp", "1.0.0", time.Now())["source_description_hash"]; ok {
		t.Error("empty source_description_hash written; this invalidates the stored description")
	}
}

// updated_at is stamped on every run, so including it in the hash would make
// every document differ from itself and defeat the comparison entirely.
func TestContentHashIgnoresUpdatedAt(t *testing.T) {
	l := models.Listing{Title: "Software Engineer", Company: models.Company{Name: "Atlassian"}}

	early := ContentHash(buildListingSet(l, "fp", "1.0.0", time.Now()))
	later := ContentHash(buildListingSet(l, "fp", "1.0.0", time.Now().Add(48*time.Hour)))

	if early != later {
		t.Error("a later run with identical content must produce the same hash")
	}
	if early == "" {
		t.Fatal("hash must not be empty for an encodable listing")
	}
}

func TestContentHashChangesWithRealEdits(t *testing.T) {
	now := time.Now()
	base := models.Listing{Title: "Software Engineer", Company: models.Company{Name: "Atlassian"}}
	baseline := ContentHash(buildListingSet(base, "fp", "1.0.0", now))

	oneLiner := "Apply your Go skills on distributed systems"
	edits := map[string]models.Listing{
		"title":     {Title: "Senior Software Engineer", Company: base.Company},
		"one_liner": {Title: base.Title, Company: base.Company, OneLinerDesc: &oneLiner},
		"company":   {Title: base.Title, Company: models.Company{Name: "Canva"}},
	}

	for name, edited := range edits {
		if ContentHash(buildListingSet(edited, "fp", "1.0.0", now)) == baseline {
			t.Errorf("a changed %s must change the hash", name)
		}
	}
}

func TestNeedsWrite(t *testing.T) {
	const hash = "abc123"

	cases := []struct {
		name   string
		stored storedState
		found  bool
		hash   string
		want   bool
	}{
		{"new document", storedState{}, false, hash, true},
		{"unchanged and live", storedState{ContentHash: hash}, true, hash, false},
		{"content changed", storedState{ContentHash: "different"}, true, hash, true},
		{"no stored hash yet", storedState{}, true, hash, true},
		// Content matches but the document must still be brought back to life.
		{"unchanged but outdated", storedState{ContentHash: hash, Outdated: true}, true, hash, true},
		{"unchanged but ageing out", storedState{ContentHash: hash, DaysLived: 3}, true, hash, true},
		// A hash we could not compute must never suppress a write.
		{"unhashable listing", storedState{ContentHash: hash}, true, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsWrite(tc.stored, tc.found, tc.hash); got != tc.want {
				t.Errorf("needsWrite = %v, want %v", got, tc.want)
			}
		})
	}
}

// Board scoring only writes board_eligible for the listings in the run it is
// given, so a listing the scraper stops seeing keeps its last flag forever. A
// board built weeks earlier stayed live under a cap added since -- 83 documents
// eligible after a run that marked 37, 27 of the stragglers one employer.
// Retiring a listing has to retire it from the board too.
func TestUnseenListingLosesBoardEligibility(t *testing.T) {
	update := unseenListingUpdate()

	if update["outdated"] != true {
		t.Error("a listing the scraper no longer sees must be marked outdated")
	}
	if eligible, ok := update["board_eligible"]; !ok || eligible != false {
		t.Errorf("a retired listing must lose board eligibility, got %v", update["board_eligible"])
	}
}

// LinkedIn supplies no working rights and is rescraped from scratch every run,
// so an unconditional write would erase what the AI parser derived the moment
// the classification came back from the cache rather than the API.
func TestEmptyWorkingRightsNeverOverwriteStoredOnes(t *testing.T) {
	l := listing("Graduate Engineer", "Atlassian", "https://apply.example.com", models.VIC)

	if _, ok := buildListingSet(l, "fp", "1.0.0", time.Now())["working_rights"]; ok {
		t.Error("a listing with no working rights must not write the field")
	}

	l.WorkingRights = []models.WorkingRight{models.INTERNATIONAL}
	set := buildListingSet(l, "fp", "1.0.0", time.Now())
	rights, ok := set["working_rights"].([]models.WorkingRight)
	if !ok || len(rights) != 1 || rights[0] != models.INTERNATIONAL {
		t.Errorf("expected the derived rights to be written, got %v", set["working_rights"])
	}
}

// The mark is a request made of a future run. It is cleared by the run that
// answers it, and only by that run: a marked listing the scrape does not reach
// keeps both its mark and the classification it already had.
func TestReclassificationMarkIsClearedOnlyWhenTheAIParserRan(t *testing.T) {
	l := listing("Junior Developer", "Atlassian", "https://apply.example.com", models.VIC)

	if _, ok := buildListingSet(l, "fp", "1.0.0", time.Now())["needs_reclassification"]; ok {
		t.Error("a run that produced no classification must not clear the mark")
	}

	l.Discipline = models.SOFTWARE
	if buildListingSet(l, "fp", "1.0.0", time.Now())["needs_reclassification"] != false {
		t.Error("a run that classified the listing must clear the mark")
	}
}

// Hashing the mark would make adding the field change every document in the
// collection, and every changed document is a Discord thread the bot re-renders.
func TestReclassificationMarkIsNotPartOfTheContentHash(t *testing.T) {
	l := listing("Junior Developer", "Atlassian", "https://apply.example.com", models.VIC)
	l.Discipline = models.SOFTWARE

	set := buildListingSet(l, "fp", "1.0.0", time.Now())
	withMark := ContentHash(set)

	delete(set, "needs_reclassification")
	if ContentHash(set) != withMark {
		t.Error("needs_reclassification must be excluded from the content hash")
	}
}
