package steps

import (
	"github.com/microcosm-cc/bluemonday"
)

// Four of the six sources return their descriptions as HTML, and it is stored
// and rendered as HTML because it is better than anything that could be
// re-derived from flattened text. That means a job ad's markup reaches a public
// board, so it is filtered down to the tags a job description has any business
// containing.
//
// This is not the security boundary. Sanitising at render is, because that is
// the only place that knows the output context and the only place that covers
// content this pipeline never touched. This is here because the board is not
// the only consumer -- the Discord bot reads the same collection, and anything
// added later will too -- and because a scrape of real listings today carries
// 50 descriptions with <iframe>, 49 with a style attribute and 4 with a data:
// URL, none of which belong in a job posting.
var descriptionPolicy = newDescriptionPolicy()

func newDescriptionPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	// Structure and emphasis, which is all a job description needs.
	p.AllowElements(
		"p", "br", "hr",
		"ul", "ol", "li",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"strong", "b", "em", "i", "u",
		"blockquote",
	)

	// Links keep their destination but nothing else. A relative href is a link
	// into the source site's own routing -- GradConnection emits
	// "/track-link/..." -- which resolves against the board's domain and dead
	// ends, so only absolute destinations survive and the rest becomes text.
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireParseableURLs(true)
	p.AllowRelativeURLs(false)

	// Deliberately no rel="nofollow" and no target="_blank". Both are choices
	// about how the board presents a link, which belongs to whatever renders
	// it, not to a filter deciding what is safe to keep. Leaving them out also
	// keeps already-clean markup byte-identical, so sanitising does not rewrite
	// every description in the collection the first time it runs.

	return p
}

// SanitiseDescriptionHTML filters source markup down to the allowed tags.
//
// Everything not on the list is unwrapped rather than deleted: the tag goes and
// the words inside it stay. A description is content first, so dropping an
// unrecognised wrapper must never drop the posting along with it.
func SanitiseDescriptionHTML(description string) string {
	return descriptionPolicy.Sanitize(description)
}
