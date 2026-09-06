package utils

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// unmappedTally counts source strings that no mapping table recognised, so the
// tables can be grown from real scrape output rather than guesswork.
//
// Every scraper translates the vocabulary of one site into our enums through a
// literal map, and every one of those maps quietly falls back to an "other"
// value on a miss. That fallback is invisible: the listing is stored, nothing
// errors, and the only way to find the strings we are dropping on the floor is
// to notice them in the data weeks later. Counting them per run and printing
// the tally at the end turns each miss into a line naming the exact string to
// add and how often it appeared.
type unmappedTally struct {
	mu       sync.Mutex
	bySource map[string]map[string]int
}

func (t *unmappedTally) record(source string, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.bySource == nil {
		t.bySource = map[string]map[string]int{}
	}
	if t.bySource[source] == nil {
		t.bySource[source] = map[string]int{}
	}
	t.bySource[source][raw]++
}

// report logs one line per source, most frequent string first, and clears the
// tally so a later run starts clean. fallback names the value the misses were
// stored as, which is what makes the line actionable.
func (t *unmappedTally) report(kind string, fallback string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for source, counts := range t.bySource {
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

		Custom("%s: %d unmapped %s strings -> %s: %s",
			source, len(entries), kind, fallback, strings.Join(lines, ", "))
	}

	t.bySource = nil
}

var unmappedRights unmappedTally

// RecordUnmappedRight notes a working-rights string that no source's mapping
// table matched. Counts are reported by ReportUnmappedRights.
//
// This matters more than the equivalent for locations. An unmapped location
// becomes OTHERS, which reads on the board as "somewhere else" and is roughly
// true. An unmapped working right becomes OTHER_RIGHTS, which reads as a
// restriction the posting may never have stated -- and it is the field an
// international student decides on.
func RecordUnmappedRight(source string, raw string) {
	unmappedRights.record(source, raw)
}

// ReportUnmappedRights logs every working-rights string that fell through to
// OTHER_RIGHTS, most frequent first, and clears the tally.
func ReportUnmappedRights() {
	unmappedRights.report("working right", "OTHER_RIGHTS")
}
