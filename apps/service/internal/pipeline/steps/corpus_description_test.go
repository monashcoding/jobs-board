package steps

import (
	"context"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// corpusGlob points at the raw scraped listings earlier runs wrote out. They are
// dumped by the first pipeline step, so the descriptions in them are exactly as
// the scrapers produced -- before any cleaning or markup. That makes them the
// right input for checking what the description path does to each source.
const corpusGlob = "../../../debug/write-json/*.json"

var (
	corpusTagStripper = regexp.MustCompile(`<[^>]*>`)
	corpusWhitespace  = regexp.MustCompile(`\s+`)
	// Space-width invisibles are whitespace for the purpose of comparing two
	// versions of the same description.
	corpusSpaceLookalikes = strings.NewReplacer("\u00a0", " ", "\u202f", " ", "\u2007", " ")
)

// corpusBareText reduces a description to its characters with all whitespace
// removed, so two versions can be compared however the tags divide them.
func corpusBareText(s string) string {
	text := html.UnescapeString(corpusTagStripper.ReplaceAllString(s, " "))
	return corpusWhitespace.ReplaceAllString(corpusSpaceLookalikes.Replace(text), "")
}

func corpusWords(s string) []string {
	return strings.Fields(html.UnescapeString(corpusTagStripper.ReplaceAllString(s, " ")))
}

type sourceStats struct {
	n, keptSourceHTML, singleParagraph int
	withPara, withList, withHeading    int
	lostWords                          int
}

func loadCorpus(t *testing.T) map[string][]string {
	t.Helper()

	files, err := filepath.Glob(corpusGlob)
	if err != nil || len(files) == 0 {
		t.Skipf("no scraped listings in %s", corpusGlob)
	}

	bySource := map[string][]string{}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read %s: %v", file, err)
		}
		var listings []struct {
			Source      string  `json:"source"`
			Description *string `json:"description"`
		}
		if err := json.Unmarshal(raw, &listings); err != nil {
			continue
		}
		for _, l := range listings {
			if l.Description != nil && strings.TrimSpace(*l.Description) != "" {
				bySource[l.Source] = append(bySource[l.Source], *l.Description)
			}
		}
	}
	if len(bySource) == 0 {
		t.Skip("no descriptions in the scraped listings")
	}
	return bySource
}

// Run every source's real descriptions through the description path and report
// what comes out. A source whose descriptions all collapse into a single
// paragraph is the symptom that sent us looking at LinkedIn in the first place,
// so it is worth knowing which other sources share it.
func TestCorpusDescriptionsAcrossEverySource(t *testing.T) {
	bySource := loadCorpus(t)
	step := &AIParserStep{}

	stats := map[string]*sourceStats{}
	var failures []string

	for source, descriptions := range bySource {
		s := &sourceStats{}
		stats[source] = s

		for _, description := range descriptions {
			s.n++

			input := description
			listing := models.Listing{Description: &input}
			got, err := step.FixDescriptionFormat(context.Background(), listing)
			if err != nil {
				t.Fatalf("%s: FixDescriptionFormat returned error: %v", source, err)
			}
			output := *got.Description

			if htmlTagPattern.MatchString(CleanDescription(description)) {
				s.keptSourceHTML++
			}
			if strings.Contains(output, "<p>") {
				s.withPara++
			}
			if strings.Contains(output, "<ul>") {
				s.withList++
			}
			if strings.Contains(output, "<h3>") {
				s.withHeading++
			}
			// One <p> and nothing else is the whole description as a single
			// unbroken block.
			if strings.Count(output, "<p>") == 1 && !strings.Contains(output, "<ul>") &&
				!strings.Contains(output, "<h3>") && strings.HasPrefix(output, "<p>") {
				s.singleParagraph++
			}

			// Whatever path a description takes, it must come out with the text
			// it went in with. Compared with the whitespace removed, because a
			// tag boundary is a word boundary to corpusWords but not to a
			// reader: unwrapping "T<span>his</span>" rejoins a word rather than
			// losing one, and non-breaking spaces become ordinary ones.
			if in, out := corpusBareText(CleanDescription(description)), corpusBareText(output); in != out {
				s.lostWords++
				if len(failures) < 8 {
					failures = append(failures, source+": in="+itoa(len(in))+" out="+itoa(len(out)))
				}
			}
		}
	}

	sources := make([]string, 0, len(stats))
	for source := range stats {
		sources = append(sources, source)
	}
	sort.Strings(sources)

	t.Logf("%-16s %6s %8s %8s %6s %6s %6s", "source", "n", "srcHTML", "1-para", "<p>", "<ul>", "<h3>")
	for _, source := range sources {
		s := stats[source]
		t.Logf("%-16s %6d %8d %8d %6d %6d %6d",
			source, s.n, s.keptSourceHTML, s.singleParagraph, s.withPara, s.withList, s.withHeading)
	}

	if len(failures) > 0 {
		t.Errorf("descriptions did not come out with the words they went in with:\n  %s",
			strings.Join(failures, "\n  "))
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
