package steps

import (
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Plain-text descriptions used to be marked up by asking the model to echo the
// whole text back wrapped in HTML. That single operation dominated the token
// budget: the description was paid for on the way in and again, at four times
// the price, on the way out. It was also the least valuable of the three AI
// operations -- purely cosmetic, and carrying a standing risk that the model
// reworded a legal document it was told only to wrap.
//
// The structure being recovered is punctuation, not meaning, so it is derived
// here instead. This is deterministic, free, instant, and cannot reword.

// bulletPattern matches the list markers job boards actually emit at the start
// of a line, including the numbered forms.
var bulletPattern = regexp.MustCompile(`^\s*(?:[-*•·‣▪◦o]|\(?\d{1,2}[.)])\s+`)

// maxHeadingLength bounds what may be promoted to a heading. Anything longer is
// prose that happens to lack a full stop.
const maxHeadingLength = 80

// maxHeadingWords bounds a heading that does not announce itself with a colon.
// Longer than this and a capitalised line is far more likely to be the first
// half of a soft-wrapped sentence.
const maxHeadingWords = 6

// MarkupDescription turns a plain-text job description into simple HTML:
// paragraphs in <p>, bullet runs in <ul>/<li>, and section headings in <h3>.
//
// It reproduces every word it is given. Text is HTML-escaped, so a description
// containing "<" or "&" cannot inject markup into the board.
func MarkupDescription(text string) string {
	blocks := splitBlocks(text)

	var out strings.Builder
	for _, block := range blocks {
		lines := block
		for len(lines) > 0 {
			if bulletPattern.MatchString(lines[0]) {
				var items []string
				for len(lines) > 0 && bulletPattern.MatchString(lines[0]) {
					items = append(items, bulletPattern.ReplaceAllString(lines[0], ""))
					lines = lines[1:]
				}
				out.WriteString("<ul>")
				for _, item := range items {
					out.WriteString("<li>" + html.EscapeString(strings.TrimSpace(item)) + "</li>")
				}
				out.WriteString("</ul>")
				continue
			}

			// A short unpunctuated line directly above more content is a
			// heading. The same line standing alone at the end is just a short
			// paragraph, so it needs something to head.
			if len(lines) > 1 && isHeading(lines[0], lines[1]) {
				out.WriteString("<h3>" + html.EscapeString(strings.TrimSpace(lines[0])) + "</h3>")
				lines = lines[1:]
				continue
			}

			// Everything up to the next bullet run is one paragraph. Soft line
			// breaks inside a paragraph are wrapping, not structure.
			var para []string
			for len(lines) > 0 && !bulletPattern.MatchString(lines[0]) {
				if len(para) > 0 && len(lines) > 1 && isHeading(lines[0], lines[1]) {
					break
				}
				para = append(para, strings.TrimSpace(lines[0]))
				lines = lines[1:]
			}
			if joined := strings.TrimSpace(strings.Join(para, " ")); joined != "" {
				out.WriteString("<p>" + html.EscapeString(joined) + "</p>")
			}
		}
	}

	return out.String()
}

// isHeading reports whether a line reads as a section heading rather than prose.
//
// The hard case is a soft-wrapped sentence: "We are hiring an engineer to join"
// is short and carries no full stop, but it is the first half of a paragraph,
// not a heading. Two signals separate them -- a heading either announces itself
// with a trailing colon, or it is a handful of capitalised words followed by
// something that starts afresh.
func isHeading(line string, next string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || len(trimmed) > maxHeadingLength {
		return false
	}
	if bulletPattern.MatchString(trimmed) {
		return false
	}

	// A colon is an explicit heading marker and needs no other evidence.
	if strings.HasSuffix(trimmed, ":") {
		return true
	}

	// Any other terminal punctuation means the line is prose, either finished
	// (".") or continuing onto the next line (",").
	if strings.ContainsAny(trimmed[len(trimmed)-1:], ".!?,;") {
		return false
	}

	if len(strings.Fields(trimmed)) > maxHeadingWords {
		return false
	}

	first, _ := utf8.DecodeRuneInString(trimmed)
	if !unicode.IsUpper(first) {
		return false
	}

	// A heading introduces something. If what follows continues in lower case
	// it is a wrapped sentence, not a new section.
	nextTrimmed := strings.TrimSpace(next)
	if nextTrimmed == "" {
		return false
	}
	if bulletPattern.MatchString(nextTrimmed) {
		return true
	}
	nextFirst, _ := utf8.DecodeRuneInString(nextTrimmed)
	return unicode.IsUpper(nextFirst)
}

// splitBlocks breaks text into blank-line-separated blocks of non-empty lines.
func splitBlocks(text string) [][]string {
	normalised := strings.ReplaceAll(text, "\r\n", "\n")
	normalised = strings.ReplaceAll(normalised, "\r", "\n")

	var blocks [][]string
	var current []string

	for _, line := range strings.Split(normalised, "\n") {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}

	return blocks
}
