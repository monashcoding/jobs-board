package utils

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// overseasTokens are country, region and city names that positively identify a
// listing as being outside Australia. They exist because sources routinely tag
// an overseas opportunity with a nationwide "Australia" location (the search
// filter leaks into the result), which would otherwise surface the listing
// under the Australia filter. Matched case-insensitively on word boundaries so
// short tokens like "NZ" and "UK" do not fire inside unrelated words.
var overseasTokens = []string{
	"Abu Dhabi", "Amsterdam", "Auckland", "Bangalore", "Bangkok", "Beijing",
	"Berlin", "Canada", "China", "Christchurch", "Dubai", "Dublin", "France",
	"Germany", "Hong Kong", "HKG", "India", "Indonesia", "Ireland", "Jakarta",
	"Japan", "Korea", "Kuala Lumpur", "London", "Malaysia", "Manila", "Mumbai",
	"Netherlands", "New York", "New Zealand", "NZ", "Osaka", "Paris",
	"Philippines", "Seoul", "Shanghai", "Shenzhen", "Singapore", "Taipei",
	"Taiwan", "Thailand", "Tokyo", "Toronto", "UAE", "UK", "United Arab Emirates",
	"United Kingdom", "United States", "USA", "Vancouver", "Vietnam",
}

var overseasPattern = func() *regexp.Regexp {
	// Longest first so "New Zealand" wins over "NZ"-style partial overlaps.
	tokens := make([]string, len(overseasTokens))
	copy(tokens, overseasTokens)
	sort.Slice(tokens, func(i, j int) bool { return len(tokens[i]) > len(tokens[j]) })

	quoted := make([]string, len(tokens))
	for i, token := range tokens {
		quoted[i] = regexp.QuoteMeta(token)
	}

	return regexp.MustCompile(`(?i)\b(` + strings.Join(quoted, "|") + `)\b`)
}()

// FindOverseasToken returns the first overseas token found in any of the given
// strings, and whether one was found.
func FindOverseasToken(candidates ...string) (string, bool) {
	for _, candidate := range candidates {
		if match := overseasPattern.FindString(candidate); match != "" {
			return match, true
		}
	}
	return "", false
}

// HasAustralianState reports whether the locations contain a specific
// Australian state or territory. AUSTRALIA (nationwide) and OTHERS do not
// count: neither pins the listing to a place inside the country.
func HasAustralianState(locations []models.LocationType) bool {
	for _, location := range locations {
		switch location {
		case models.AUSTRALIA, models.OTHERS:
			continue
		default:
			return true
		}
	}
	return false
}

// IsAustralian reports whether a listing is located in Australia, i.e. it has
// either a specific state or the nationwide AUSTRALIA marker.
func IsAustralian(locations []models.LocationType) bool {
	for _, location := range locations {
		if location != models.OTHERS {
			return true
		}
	}
	return false
}

// SanitiseAustralianLocations strips a bogus nationwide AUSTRALIA marker from a
// listing that is actually overseas.
//
// A listing keeps AUSTRALIA if it names a specific Australian state, since a
// program genuinely running in both Sydney and Singapore is still an Australian
// opportunity. Otherwise, if any of the raw location strings or the title names
// an overseas place, the listing is relabelled OTHERS so it stops appearing
// under the Australia filter.
func SanitiseAustralianLocations(locations []models.LocationType, title string, rawLocations []string) []models.LocationType {
	if HasAustralianState(locations) {
		return locations
	}

	candidates := append([]string{title}, rawLocations...)
	token, overseas := FindOverseasToken(candidates...)
	if !overseas {
		return locations
	}

	sanitised := make([]models.LocationType, 0, len(locations))
	stripped := false
	for _, location := range locations {
		if location == models.AUSTRALIA {
			stripped = true
			continue
		}
		sanitised = append(sanitised, location)
	}

	if !stripped {
		return locations
	}

	if len(sanitised) == 0 {
		sanitised = append(sanitised, models.OTHERS)
	}

	Warn("Relabelled %q as OTHERS: no Australian state and overseas token %q found", title, token)

	return sanitised
}

var (
	unmappedMu        sync.Mutex
	unmappedLocations = map[string]map[string]int{}
)

// RecordUnmappedLocation notes a location string that no state or city table
// matched, so the tables can be grown from real scrape output rather than
// guesswork. Counts are reported by ReportUnmappedLocations.
func RecordUnmappedLocation(source string, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}

	unmappedMu.Lock()
	defer unmappedMu.Unlock()

	if unmappedLocations[source] == nil {
		unmappedLocations[source] = map[string]int{}
	}
	unmappedLocations[source][raw]++
}

// ReportUnmappedLocations logs every location string that fell through to
// OTHERS, most frequent first, and clears the tally.
func ReportUnmappedLocations() {
	unmappedMu.Lock()
	defer unmappedMu.Unlock()

	for source, counts := range unmappedLocations {
		type entry struct {
			raw   string
			count int
		}

		entries := make([]entry, 0, len(counts))
		for raw, count := range counts {
			entries = append(entries, entry{raw, count})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].count != entries[j].count {
				return entries[i].count > entries[j].count
			}
			return entries[i].raw < entries[j].raw
		})

		lines := make([]string, 0, len(entries))
		for _, e := range entries {
			lines = append(lines, fmt.Sprintf("%s (x%d)", e.raw, e.count))
		}

		Custom("%s: %d unmapped location strings -> OTHERS: %s",
			source, len(entries), strings.Join(lines, ", "))
	}

	unmappedLocations = map[string]map[string]int{}
}
