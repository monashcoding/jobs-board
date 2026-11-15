package steps

import (
	"context"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// LinkedIn is the only scraper that leaves Type nil, and this step runs before
// the AI parser fills it in. Keying on job type meant the same job scraped from
// LinkedIn and GradConnection hashed differently and survived as two listings,
// which later looked identical once the parser had filled the gap.
func TestDeduplicationCollapsesUnknownJobType(t *testing.T) {
	intern := models.INTERN
	gradconURL := "https://lifeattiktok.com/search/7659609380478257413"
	linkedinURL := "https://www.linkedin.com/jobs/view/4437008897"

	listings := []models.Listing{
		{
			Title:   "Agent Development Engineer Intern (Media Network) - 2027 Start",
			Company: models.Company{Name: "TikTok"}, Type: &intern,
			Locations: []models.LocationType{models.NSW},
			Source:    models.GRADCONNECTION, ApplicationURL: &gradconURL,
			SourceURLs: []string{"https://au.gradconnection.com/a"},
		},
		{
			Title:   "Agent Development Engineer Intern (Media Network) - 2027 Start",
			Company: models.Company{Name: "TikTok"}, Type: nil,
			Locations: []models.LocationType{models.NSW},
			Source:    models.LINKEDIN, ApplicationURL: &linkedinURL,
			SourceURLs: []string{linkedinURL},
		},
	}

	out, err := NewDeduplicationStep().Process(context.Background(), listings)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected the pair to collapse to 1 listing, got %d", len(out))
	}
	if out[0].Type == nil || *out[0].Type != models.INTERN {
		t.Error("the surviving listing must keep the known job type")
	}
}

// A nil type must not merge roles that are genuinely different.
func TestDeduplicationKeepsDistinctJobTypes(t *testing.T) {
	intern, graduate := models.INTERN, models.GRADUATE
	// Distinct application URLs, or step 2 collapses the pair before the
	// title/company/location rule is reached.
	internURL := "https://example.com/apply/intern"
	graduateURL := "https://example.com/apply/graduate"

	listings := []models.Listing{
		{Title: "Software Engineer", Company: models.Company{Name: "Example"}, Type: &intern,
			Locations: []models.LocationType{models.VIC}, ApplicationURL: &internURL,
			SourceURLs: []string{"https://example.com/a"}},
		{Title: "Software Engineer", Company: models.Company{Name: "Example"}, Type: &graduate,
			Locations: []models.LocationType{models.VIC}, ApplicationURL: &graduateURL,
			SourceURLs: []string{"https://example.com/b"}},
	}

	out, err := NewDeduplicationStep().Process(context.Background(), listings)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("an intern and a graduate opening must stay distinct, got %d", len(out))
	}
}
