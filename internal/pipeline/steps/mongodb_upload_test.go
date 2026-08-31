package steps

import (
	"testing"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
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
