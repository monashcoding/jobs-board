package airwallex

import (
	"strings"
	"testing"
)

// Ashby returns the description as HTML. It used to be flattened -- tags became
// spaces, then strings.Fields collapsed every run of whitespace -- so the
// paragraphs, headings and lists were gone before anything downstream saw them,
// and all four Airwallex listings reached the board as one unbroken block.
func TestAirwallexDescriptionKeepsItsMarkup(t *testing.T) {
	const descriptionHTML = `<p><strong>About Airwallex</strong></p>` +
		`<p>We are a unified payments platform.</p>` +
		`<ul><li>Build backend services</li><li>Work across teams</li></ul>`

	details := &jobDetailResponse{}
	details.Data.JobPosting.DescriptionHtml = descriptionHTML

	listing := (&AirwallexScraper{}).convertToListing(jobPosting{
		ID:    "abc123",
		Title: "Graduate Software Engineer",
	}, details)

	if listing.Description == nil {
		t.Fatal("the description was dropped entirely")
	}
	got := *listing.Description

	if got != descriptionHTML {
		t.Errorf("description was altered:\n got: %q\nwant: %q", got, descriptionHTML)
	}
	for _, tag := range []string{"<p>", "<ul>", "<li>", "<strong>"} {
		if !strings.Contains(got, tag) {
			t.Errorf("lost %s from the description: %q", tag, got)
		}
	}
}

// A posting with no detail payload must not invent a description.
func TestAirwallexDescriptionIsAbsentWithoutDetails(t *testing.T) {
	listing := (&AirwallexScraper{}).convertToListing(jobPosting{ID: "abc123", Title: "Intern"}, nil)

	if listing.Description != nil {
		t.Errorf("expected no description, got %q", *listing.Description)
	}
}
