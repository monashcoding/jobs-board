package steps

import (
	"context"
	"strings"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// What a job description is allowed to carry onto the board.
func TestSanitiseKeepsTheMarkupADescriptionNeeds(t *testing.T) {
	cases := []string{
		"<p>We build payment systems.</p>",
		"<h3>What you will do</h3>",
		"<ul><li>Monitor health</li><li>Tune queries</li></ul>",
		"<ol><li>Apply</li><li>Interview</li></ol>",
		"<p>We use <strong>Go</strong> and <em>Kubernetes</em>.</p>",
		"<blockquote>A great place to work.</blockquote>",
		"<p>Sydney<br/>Hybrid</p>",
		`<p>Read the <a href="https://example.com/apply">docs</a>.</p>`,
	}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			// Already-clean markup must come back byte-identical, so sanitising
			// does not rewrite every description in the collection.
			if got := SanitiseDescriptionHTML(input); got != input {
				t.Errorf("clean markup was altered:\n got: %q\nwant: %q", got, input)
			}
		})
	}
}

// A live scrape carries 50 descriptions with an <iframe>, 49 with a style
// attribute and 4 with a data: URL. None of those belong in a job posting, and
// the board renders whatever it is given.
func TestSanitiseDropsWhatDoesNotBelongInAPosting(t *testing.T) {
	cases := []struct {
		name  string
		input string
		gone  string
		kept  string
	}{
		{
			name:  "script",
			input: `<p>Apply now.</p><script>steal()</script>`,
			gone:  "script",
			kept:  "Apply now.",
		},
		{
			name:  "iframe",
			input: `<p>Watch this.</p><iframe src="https://video.test/x"></iframe>`,
			gone:  "iframe",
			kept:  "Watch this.",
		},
		{
			name:  "inline event handler",
			input: `<p onclick="steal()">Apply now.</p>`,
			gone:  "onclick",
			kept:  "Apply now.",
		},
		{
			name:  "style attribute",
			input: `<p style="position:fixed;top:0">Apply now.</p>`,
			gone:  "style",
			kept:  "Apply now.",
		},
		{
			name:  "javascript URL",
			input: `<p><a href="javascript:steal()">Apply</a></p>`,
			gone:  "javascript:",
			kept:  "Apply",
		},
		{
			name:  "data URL",
			input: `<p><a href="data:text/html;base64,PHNjcmlwdD4=">Apply</a></p>`,
			gone:  "data:",
			kept:  "Apply",
		},
		{
			name:  "image, which is an external load and often a tracking pixel",
			input: `<p>About us <img src="https://track.test/pixel.gif"/></p>`,
			gone:  "<img",
			kept:  "About us",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitiseDescriptionHTML(tc.input)

			if strings.Contains(got, tc.gone) {
				t.Errorf("%s survived sanitising: %q", tc.gone, got)
			}
			// The words are the content. Filtering markup must never take the
			// posting with it.
			if !strings.Contains(got, tc.kept) {
				t.Errorf("lost the description's text %q: %q", tc.kept, got)
			}
		})
	}
}

// A relative href resolves against the board's own domain and dead ends --
// GradConnection emits "/track-link/..." -- so the destination goes and the
// words stay.
func TestSanitiseUnwrapsRelativeLinksButKeepsTheirText(t *testing.T) {
	got := SanitiseDescriptionHTML(`<p>Read <a href="/track-link/1c96080e">the details</a> first.</p>`)

	if strings.Contains(got, "track-link") {
		t.Errorf("a relative link survived: %q", got)
	}
	if !strings.Contains(got, "the details") {
		t.Errorf("the link's text was dropped with it: %q", got)
	}
}

// An unrecognised wrapper is unwrapped, never deleted with its contents. A
// description is content first.
func TestSanitiseNeverDropsTheWordsInsideAnUnknownTag(t *testing.T) {
	got := SanitiseDescriptionHTML(
		`<section><article><p>We are hiring a graduate engineer.</p></article></section>`)

	if !strings.Contains(got, "We are hiring a graduate engineer.") {
		t.Errorf("the posting was dropped along with its wrapper: %q", got)
	}
}

// Plain-text descriptions never reach the sanitiser: they go through the markup
// path, which escapes everything. This checks the two paths agree that markup
// cannot be injected either way.
func TestNeitherDescriptionPathLetsMarkupThrough(t *testing.T) {
	step := &AIParserStep{}

	for _, input := range []string{
		"Salary <100k. Contact <script>alert(1)</script> us.",
		`<p>Apply</p><script>alert(1)</script>`,
	} {
		t.Run(input, func(t *testing.T) {
			in := input
			got, err := step.FixDescriptionFormat(context.Background(), models.Listing{Description: &in})
			if err != nil {
				t.Fatalf("FixDescriptionFormat returned error: %v", err)
			}
			if strings.Contains(*got.Description, "<script>") {
				t.Errorf("a script tag reached the board: %q", *got.Description)
			}
		})
	}
}
