package steps

import (
	"context"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// These pin the exact HTML each kind of source description turns into, so a
// change to the description path has to state what it changes rather than
// quietly altering what reaches the board. The inputs are shaped after real
// scraped descriptions, one per source.
func TestDescriptionGolden(t *testing.T) {
	cases := []struct {
		name  string
		about string
		input string
		want  string
	}{
		{
			name:  "prosple keeps its own block markup",
			about: "Prosple, Atlassian and Jane Street all return well-formed HTML. It is better than anything that could be re-derived, so its block structure passes through. Line endings are not part of the claim: Prosple really sends CRLF, and normalising that changes nothing a reader sees.",
			input: "<p><strong>Kearney</strong> is a leading global firm.</p>\n\n<p>We hire graduates every year.</p>",
			want:  "<p><strong>Kearney</strong> is a leading global firm.</p>\n\n<p>We hire graduates every year.</p>",
		},
		{
			name:  "jane street keeps its own headings",
			about: "A source that already marks up headings must keep them rather than have them re-guessed.",
			input: "<h3>About the Program</h3>\n<p>Our goal is to give you a real sense of the work.</p>",
			want:  "<h3>About the Program</h3>\n<p>Our goal is to give you a real sense of the work.</p>",
		},
		{
			name:  "inline-only markup still counts as markup",
			about: "A description carrying only <strong> or <a> has no block structure to find, but running it through the markup path would escape those tags and print them to the board as text.",
			input: "Apply via <a href=\"https://example.com\">our portal</a> before Friday.",
			want:  "Apply via <a href=\"https://example.com\">our portal</a> before Friday.",
		},
		{
			name:  "airwallex ashby markup is kept",
			about: "Ashby returns the description as HTML. It used to be flattened to text and every run of whitespace collapsed, so all four Airwallex listings reached the board as one unbroken block.",
			input: "<p><strong>About Airwallex</strong></p><p>We are a unified payments platform.</p><ul><li>Build backend services</li><li>Work across teams</li></ul>",
			want:  "<p><strong>About Airwallex</strong></p><p>We are a unified payments platform.</p><ul><li>Build backend services</li><li>Work across teams</li></ul>",
		},
		{
			name:  "gradconnection placeholder is one paragraph",
			about: "GradConnection emits a one-sentence 'Notify Me' description for campaigns that are not open yet. One paragraph is the whole of it, and nothing should be promoted to a heading.",
			input: "Turn on notifications for nib's Graduate Jobs and be one of the first to apply when applications open.",
			want:  "<p>Turn on notifications for nib&#39;s Graduate Jobs and be one of the first to apply when applications open.</p>",
		},
		{
			name:  "linkedin text recovers headings, paragraphs and lists",
			about: "What the LinkedIn extraction now hands over: block boundaries as blank lines, list items as '- '. This is the shape the whole markup step exists to read.",
			input: "\n\nWhat you will do:\n\nBuild and maintain payment systems.\n\n\n- Monitor health and capacity\n- Tune queries and indexes",
			want:  "<h3>What you will do:</h3><p>Build and maintain payment systems.</p><ul><li>Monitor health and capacity</li><li>Tune queries and indexes</li></ul>",
		},
		{
			name:  "a one-line fact stays a paragraph",
			about: "Across blocks only a colon marks a heading. Without that rule every one-line fact in an HTML-derived description became a section heading.",
			input: "Job Requisition ID: 41249\n\nWe are hiring a database administrator.",
			want:  "<p>Job Requisition ID: 41249</p><p>We are hiring a database administrator.</p>",
		},
		{
			name:  "a trailing colon with nothing after it is not a heading",
			about: "A heading has to introduce something. 'Apply here:' closing a description is a sentence.",
			input: "Build the platform with us.\n\nApply here:",
			want:  "<p>Build the platform with us.</p><p>Apply here:</p>",
		},
		{
			name:  "invisible space lookalikes become ordinary spaces",
			about: "Non-breaking and narrow spaces leak in from scraped pages and would otherwise render as part of the word.",
			input: "Apply by 5pm on Friday.",
			want:  "<p>Apply by 5pm on Friday.</p>",
		},
		{
			name:  "an emoji sequence survives intact",
			about: "The zero-width joiner binds an emoji sequence into one glyph. Treating it as invisible junk split it into two on the board.",
			input: "Meet the team \U0001f469‍\U0001f4bb and say hello.",
			want:  "<p>Meet the team \U0001f469‍\U0001f4bb and say hello.</p>",
		},
		{
			name:  "markup cannot be injected through a plain-text description",
			about: "Plain text is escaped on the way into the board's HTML.",
			input: "Salary <100k, benefits & equity.",
			want:  "<p>Salary &lt;100k, benefits &amp; equity.</p>",
		},
	}

	step := &AIParserStep{}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input
			got, err := step.FixDescriptionFormat(context.Background(), models.Listing{Description: &input})
			if err != nil {
				t.Fatalf("FixDescriptionFormat returned error: %v", err)
			}
			if *got.Description != tc.want {
				t.Errorf("%s\n input: %q\n   got: %q\n  want: %q", tc.about, tc.input, *got.Description, tc.want)
			}
		})
	}
}
