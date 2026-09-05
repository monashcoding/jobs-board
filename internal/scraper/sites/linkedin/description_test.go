package linkedin

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func markup(t *testing.T, body string) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div class="show-more-less-html__markup">` + body + `</div>`))
	if err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}
	return doc.Find("div.show-more-less-html__markup")
}

// LinkedIn serves the description minified, with no whitespace between tags, so
// goquery's Text() fused every word across a tag boundary. These are the exact
// shapes that reached the board: "Description:Job Requisition ID" and
// "resilience.What will your typical day look like?".
func TestDescriptionTextSeparatesBlocks(t *testing.T) {
	got := descriptionText(markup(t,
		`<p><strong>Description:</strong></p><p>Job Requisition ID: 41249</p>`+
			`<p>This role improves operational resilience.</p>`+
			`<p>What will your typical day look like?</p>`))

	for _, fused := range []string{"Description:Job", "resilience.What"} {
		if strings.Contains(got, fused) {
			t.Errorf("words are still fused across a block boundary: found %q in %q", fused, got)
		}
	}
	for _, phrase := range []string{"Description:", "Job Requisition ID: 41249", "operational resilience."} {
		if !strings.Contains(got, phrase) {
			t.Errorf("lost %q from the description: %q", phrase, got)
		}
	}
}

// List items carry no punctuation of their own, so without a marker they fuse
// into an unreadable run and the markup step has nothing to detect a list by.
func TestDescriptionTextMarksListItems(t *testing.T) {
	got := descriptionText(markup(t,
		`<ul><li>Monitor health and capacity</li><li>Tune queries and indexes</li></ul>`))

	if strings.Contains(got, "capacityTune") {
		t.Errorf("list items are still fused: %q", got)
	}
	if n := strings.Count(got, "\n- "); n != 2 {
		t.Errorf("expected 2 bullet markers, got %d in %q", n, got)
	}
}

// A <br> is a line break and nothing else marks it, so it has to become one.
func TestDescriptionTextKeepsLineBreaks(t *testing.T) {
	got := descriptionText(markup(t, `<p>Sydney, NSW<br/>Hybrid working</p>`))

	if strings.Contains(got, "NSWHybrid") {
		t.Errorf("a line break was dropped: %q", got)
	}
}

// Whatever structure is added, no word may be lost on the way through.
func TestDescriptionTextKeepsEveryWord(t *testing.T) {
	got := descriptionText(markup(t,
		`<p>Join Deloitte as a Database Administrator.</p>`+
			`<ul><li>Monitor health</li><li>Tune queries</li></ul>`+
			`<p>Apply now.</p>`))

	for _, word := range strings.Fields(
		"Join Deloitte as a Database Administrator. Monitor health Tune queries Apply now.") {
		if !strings.Contains(got, word) {
			t.Errorf("lost the word %q from %q", word, got)
		}
	}
}
