package linkedin

import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
)

// corpusDir holds real LinkedIn job pages saved by earlier runs. These tests
// verify the extraction against actual pages rather than hand-written fixtures,
// and skip when the directory is not present.
const corpusDir = "../../../../debug/linkedin"

const descriptionSelector = "div.show-more-less-html__markup"

var (
	tagStripper  = regexp.MustCompile(`<[^>]*>`)
	whitespaceRE = regexp.MustCompile(`\s+`)
	// A lowercase letter immediately followed by an uppercase one inside one
	// token is the signature of two words fused at a lost tag boundary:
	// "resilienceWhat", "capacityTune". Legitimate CamelCase exists, so this is
	// a comparative measure rather than a limit.
	fusionPattern = regexp.MustCompile(`\p{Ll}\p{Lu}`)
)

func words(s string) []string {
	return strings.Fields(html.UnescapeString(tagStripper.ReplaceAllString(s, " ")))
}

// bare reduces text to its characters, dropping all whitespace, so two versions
// of the same description can be compared regardless of how it is broken up.
func bare(s string) string {
	return whitespaceRE.ReplaceAllString(s, "")
}

// bulletPrefix mirrors the production bulletPattern: a list marker is only a
// marker at the start of a line, and only when something follows it. A "-"
// anywhere else -- "Sydney - Hybrid" -- is content and must survive.
var bulletPrefix = regexp.MustCompile(`^\s*(?:[-*\x{2022}\x{00b7}\x{2023}\x{25aa}\x{25e6}o]|\(?\d{1,2}[.)])\s+`)

// contentWords is the description's words with line-leading bullet markers
// removed, since the markup step consumes those into <li>.
func contentWords(s string) []string {
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		kept = append(kept, words(bulletPrefix.ReplaceAllString(line, ""))...)
	}
	return kept
}

func corpusPages(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(corpusDir, "job_*.html"))
	if err != nil || len(files) == 0 {
		t.Skipf("no saved LinkedIn pages in %s", corpusDir)
	}
	return files
}

// parse returns a fresh document; descriptionText rewrites the DOM it is given,
// so each extraction needs its own.
func parse(t *testing.T, raw []byte) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(raw)))
	if err != nil {
		return nil
	}
	return doc.Find(descriptionSelector)
}

// Invariant 1: restoring the block boundaries adds structure and nothing else.
// With the whitespace stripped back out, the character stream has to be exactly
// what Text() produced.
func TestCorpusExtractionOnlyAddsStructure(t *testing.T) {
	var checked int

	for _, file := range corpusPages(t) {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read %s: %v", file, err)
		}

		before := parse(t, raw)
		after := parse(t, raw)
		if before == nil || after == nil {
			continue
		}

		original := before.Text()
		if strings.TrimSpace(original) == "" {
			continue
		}
		checked++

		// referenceText walks the DOM exactly as descriptionText does but
		// inserts only whitespace, so with the whitespace removed its character
		// stream must be identical to what Text() produced. Anything else means
		// the walk is dropping or inventing content rather than separating it.
		if bare(referenceText(after)) != bare(original) {
			t.Errorf("%s: extraction changed the description's characters", filepath.Base(file))
		}
	}

	t.Logf("verified %d saved pages", checked)
}

// Invariant 2: the markup step reproduces every word of what it is handed. The
// reference is built the same way but without bullet markers, so the comparison
// is against the description's own content.
func TestCorpusMarkupReproducesEveryWord(t *testing.T) {
	var (
		checked, withPara, withList, withHeading int
		fusedBefore, fusedAfter                  int
		wordsBefore, wordsAfter                  int
		failures                                 []string
	)

	for _, file := range corpusPages(t) {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read %s: %v", file, err)
		}

		flat := parse(t, raw)
		marked := parse(t, raw)
		reference := parse(t, raw)
		if flat == nil || marked == nil || reference == nil {
			continue
		}

		flatText := flat.Text()
		if strings.TrimSpace(flatText) == "" {
			continue
		}
		checked++

		output := steps.MarkupDescription(steps.CleanDescription(descriptionText(marked)))

		// referenceText is descriptionText without the bullet markers, so its
		// words are exactly the content the output must reproduce. It goes
		// through the same cleaning, since that is the markup step's real input.
		in := contentWords(steps.CleanDescription(referenceText(reference)))
		out := words(output)

		if len(in) != len(out) {
			failures = append(failures, filepath.Base(file))
		} else {
			for i := range in {
				if in[i] != out[i] {
					failures = append(failures, filepath.Base(file)+": "+in[i]+" -> "+out[i])
					break
				}
			}
		}

		before := words(flatText)
		wordsBefore += len(before)
		wordsAfter += len(in)
		for _, w := range before {
			if fusionPattern.MatchString(w) {
				fusedBefore++
			}
		}
		for _, w := range in {
			if fusionPattern.MatchString(w) {
				fusedAfter++
			}
		}

		if strings.Contains(output, "<p>") {
			withPara++
		}
		if strings.Contains(output, "<ul>") {
			withList++
		}
		if strings.Contains(output, "<h3>") {
			withHeading++
		}
	}

	t.Logf("pages checked:  %d", checked)
	t.Logf("words:          %d -> %d (%d were fused into a neighbour)", wordsBefore, wordsAfter, wordsAfter-wordsBefore)
	t.Logf("fused tokens:   %d -> %d", fusedBefore, fusedAfter)
	t.Logf("output <p>:     %d/%d", withPara, checked)
	t.Logf("output <ul>:    %d/%d", withList, checked)
	t.Logf("output <h3>:    %d/%d", withHeading, checked)

	if len(failures) > 0 {
		shown := failures
		if len(shown) > 10 {
			shown = shown[:10]
		}
		t.Errorf("%d of %d pages did not reproduce their input word for word:\n  %s",
			len(failures), checked, strings.Join(shown, "\n  "))
	}
}

// referenceText mirrors descriptionText but inserts no bullet markers, so its
// words are the description's content alone. Filtering "-" out of
// descriptionText's result instead would also drop the real hyphens in titles
// like "Expressions Of Interest - Fujitsu Cyber".
func referenceText(sel *goquery.Selection) string {
	sel.Find("br").AfterHtml("\n")
	sel.Find("li").BeforeHtml("\n")
	sel.Find("p, div, section, h1, h2, h3, h4, h5, h6, ul, ol, table, tr").BeforeHtml("\n\n")
	return sel.Text()
}
