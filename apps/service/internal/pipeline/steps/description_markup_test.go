package steps

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

func contextBackground() context.Context { return context.Background() }

func listingWithDescription(d *string) models.Listing {
	return models.Listing{Title: "Engineer", Description: d}
}

var tagStripper = regexp.MustCompile(`<[^>]+>`)

func TestMarkupDescriptionStructure(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			name: "paragraphs split on blank lines",
			text: "We are hiring.\n\nApply today.",
			want: "<p>We are hiring.</p><p>Apply today.</p>",
		},
		{
			name: "soft wraps join into one paragraph",
			text: "We are hiring a graduate engineer to join\nour platform team in Melbourne.",
			want: "<p>We are hiring a graduate engineer to join our platform team in Melbourne.</p>",
		},
		{
			name: "bullet run becomes a list",
			text: "Requirements:\n- Go\n- SQL\n- Kubernetes",
			want: "<h3>Requirements:</h3><ul><li>Go</li><li>SQL</li><li>Kubernetes</li></ul>",
		},
		{
			name: "numbered items are bullets too",
			text: "Process:\n1. Apply\n2. Interview",
			want: "<h3>Process:</h3><ul><li>Apply</li><li>Interview</li></ul>",
		},
		{
			name: "heading followed by prose",
			text: "About the role\nYou will build things.",
			want: "<h3>About the role</h3><p>You will build things.</p>",
		},
		{
			name: "a lone short line is a paragraph, not a heading",
			text: "Apply now",
			want: "<p>Apply now</p>",
		},
		{
			name: "a sentence is never a heading",
			text: "We build software.\nJoin us.",
			want: "<p>We build software. Join us.</p>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MarkupDescription(tc.text); got != tc.want {
				t.Fatalf("MarkupDescription(%q)\n got: %s\nwant: %s", tc.text, got, tc.want)
			}
		})
	}
}

// The markup step replaced a model that was told never to reword its input. The
// replacement has to hold that line absolutely: every word in, every word out.
func TestMarkupDescriptionPreservesEveryWord(t *testing.T) {
	text := `Senior Graduate Engineer

About us
We build payment infrastructure for banks across Australia and New Zealand.

Requirements:
- 3+ years with Go or Java
- Experience with distributed systems
- Right to work in Australia

Benefits include flexible hours,
paid parental leave and an annual
learning budget.`

	got := MarkupDescription(text)

	// Bullet markers are structure, not words: they are consumed into <li>.
	var wantWords []string
	for _, line := range strings.Split(text, "\n") {
		wantWords = append(wantWords, strings.Fields(bulletPattern.ReplaceAllString(line, ""))...)
	}
	gotWords := strings.Fields(tagStripper.ReplaceAllString(got, " "))

	if len(wantWords) != len(gotWords) {
		t.Fatalf("word count changed: got %d, want %d\n%s", len(gotWords), len(wantWords), got)
	}
	for i := range wantWords {
		if wantWords[i] != gotWords[i] {
			t.Fatalf("word %d changed: got %q, want %q", i, gotWords[i], wantWords[i])
		}
	}
}

// Descriptions are attacker-influenced text rendered into a job board, so the
// markup step must not let raw HTML through.
func TestMarkupDescriptionEscapesHTML(t *testing.T) {
	got := MarkupDescription(`Use <script>alert(1)</script> & "quotes"`)

	if strings.Contains(got, "<script>") {
		t.Fatalf("raw script tag survived markup: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("script tag was not escaped: %s", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Fatalf("ampersand was not escaped: %s", got)
	}
}

func TestMarkupDescriptionEmptyInput(t *testing.T) {
	if got := MarkupDescription("   \n\n  "); got != "" {
		t.Fatalf("blank input produced %q, want empty", got)
	}
}

// A description carrying only inline tags has no block structure to detect, so
// it used to fall through to the markup path and have its own tags escaped into
// visible "&lt;strong&gt;" text on the board.
func TestInlineOnlyHTMLIsNotMarkedUp(t *testing.T) {
	cases := []string{
		"We want <strong>Go</strong> engineers.",
		`Read the <a href="https://x.test">docs</a>.`,
		"Some <em>emphasis</em> here.",
	}

	for _, original := range cases {
		t.Run(original, func(t *testing.T) {
			desc := original
			got, err := NewAiParserStep().FixDescriptionFormat(
				contextBackground(), listingWithDescription(&desc))
			if err != nil {
				t.Fatalf("FixDescriptionFormat: %v", err)
			}
			if *got.Description != original {
				t.Fatalf("inline HTML was mangled:\n got: %s\nwant: %s", *got.Description, original)
			}
		})
	}
}

// Prose comparing numbers is not markup and must still be formatted.
func TestAngleBracketsInProseAreNotMistakenForHTML(t *testing.T) {
	desc := "We need <5 years experience.\n\nSalary <$100k considered."
	got, err := NewAiParserStep().FixDescriptionFormat(
		contextBackground(), listingWithDescription(&desc))
	if err != nil {
		t.Fatalf("FixDescriptionFormat: %v", err)
	}
	if !strings.Contains(*got.Description, "<p>") {
		t.Fatalf("plain prose was treated as HTML and left unformatted: %s", *got.Description)
	}
	if !strings.Contains(*got.Description, "&lt;5 years") {
		t.Fatalf("the literal < was not escaped: %s", *got.Description)
	}
}

// Text derived from HTML puts a break after every element, so a heading is
// alone in its own block with nothing after it to look at. Detection that only
// ever looked within a block could not see what the heading introduced, and
// every LinkedIn description came out as an unbroken run of paragraphs.
func TestAHeadingAloneInItsBlockIsStillAHeading(t *testing.T) {
	got := MarkupDescription("What you will do:\n\nBuild and maintain the platform.")

	if !strings.Contains(got, "<h3>What you will do:</h3>") {
		t.Errorf("a colon-terminated line introducing the next block is a heading: %q", got)
	}
}

// The looser test -- a short capitalised line above another capitalised line --
// is safe within a block but not across them, where every one-line fact would
// qualify. "Job Requisition ID: 41249" is prose, not a section heading.
func TestAOneLineFactIsNotPromotedToAHeading(t *testing.T) {
	got := MarkupDescription("Job Requisition ID: 41249\n\nWhat will your day look like?")

	if strings.Contains(got, "<h3>Job Requisition ID: 41249</h3>") {
		t.Errorf("a one-line fact must stay a paragraph: %q", got)
	}
	if !strings.Contains(got, "<p>Job Requisition ID: 41249</p>") {
		t.Errorf("expected the fact as a paragraph: %q", got)
	}
}

// A heading needs something to head. The last line of a description has
// nothing after it, whatever punctuation it carries.
func TestATrailingColonLineAtTheEndIsNotAHeading(t *testing.T) {
	got := MarkupDescription("Build the platform.\n\nApply here:")

	if strings.Contains(got, "<h3>") {
		t.Errorf("a heading with nothing after it is just a paragraph: %q", got)
	}
}
