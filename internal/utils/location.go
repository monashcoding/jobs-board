package utils

import (
	"regexp"
	"sort"
	"strings"

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

// apacTokens are the overseas tokens that belong to APAC. The board is
// Australian and takes none of these, but the website shows the same
// collection, and "APAC" is worth more to a reader there than the single
// OTHERS bucket that used to hold Hong Kong, London and New York alike.
//
// Everything overseas and not in here stays OTHERS, deliberately: nobody is
// filtering an Australian graduate board by EMEA.
var apacTokens = map[string]bool{
	"Bangalore": true, "Bangkok": true, "Beijing": true, "China": true,
	"Hong Kong": true, "HKG": true, "India": true, "Indonesia": true,
	"Jakarta": true, "Japan": true, "Korea": true, "Kuala Lumpur": true,
	"Malaysia": true, "Manila": true, "Mumbai": true, "Osaka": true,
	"Philippines": true, "Seoul": true, "Shanghai": true, "Shenzhen": true,
	"Singapore": true, "Taipei": true, "Taiwan": true, "Thailand": true,
	"Tokyo": true, "Vietnam": true,
}

// OverseasLocationFor places a listing already known to be outside Australia:
// APAC when the text names somewhere in the region, OTHERS otherwise.
func OverseasLocationFor(candidates ...string) models.LocationType {
	if token, found := FindOverseasToken(candidates...); found && apacTokens[canonicalOverseasToken(token)] {
		return models.APAC
	}
	return models.OTHERS
}

// canonicalOverseasToken maps a matched token back to its entry in
// overseasTokens. The pattern matches case-insensitively, so "singapore" and
// "SINGAPORE" both have to reach the "Singapore" key.
func canonicalOverseasToken(match string) string {
	for _, token := range overseasTokens {
		if strings.EqualFold(token, match) {
			return token
		}
	}
	return match
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

// australianStateCities maps each state to the places inside it. It was three
// tables in three scrapers: GradConnection had this one, Prosple knew only the
// state names, and LinkedIn had an if-chain of eight capitals. A place added
// for one source is now known to all of them.
//
// That divergence was not free. Prosple labels a job "Sydney" or "Hobart TAS",
// its map only had "New South Wales", and 18 plainly Australian listings --
// Apple's Sydney internship, Queensland Rail, a teacher in Hobart -- were
// stored as OTHERS and kept off the board.
//
// Anything missing still lands in OTHERS and is reported by
// ReportUnmappedLocations, which is how these lists should grow.
var australianStateCities = map[models.LocationType][]string{
	models.VIC: {"Victoria", "Melbourne", "Geelong", "Ballarat", "Bendigo", "Docklands", "Clayton", "Melbourne - Bayside and South Eastern Suburbs", "Melbourne - Eastern Suburbs", "Melbourne - Northern Suburbs", "Melbourne - Western Suburbs", "Melbourne - CBD and Inner Suburbs", "Swan Hill", "Shepparton", "Traralgon", "Horsham", "Wangaratta", "Mildura", "Colac", "Pakenham", "Warrnambool", "Sale", "Echuca", "Regional Victoria", "Port Melbourne", "Dandenong", "Richmond", "Warragul", "VIC"},
	models.NSW: {"New South Wales", "Sydney", "Newcastle", "Wollongong", "Albury", "Armidale", "Austinmer", "Botany", "Dubbo", "Tamworth", "Orange", "Moree", "Hunter Valley", "Williamtown", "Grafton", "Nowra", "Penrith", "Parramatta", "Rhodes", "Chatswood", "Central Coast", "North Sydney", "Garden Island (NSW)", "Wagga Wagga", "Mulwala", "Rydalmere", "Gosford", "Regional New South Wales", "Parkes", "Goulburn", "Coffs Harbour", "Paramatta", "NSW"},
	models.QLD: {"Queensland", "Brisbane", "Gold Coast", "Cairns", "Townsville", "Maroochydore", "Mount Isa", "Mackay", "Rockhampton", "Emerald", "Dalby", "Toowoomba", "South Brisbane", "Roma", "Gladstone", "Central Queensland", "Sunshine Coast", "Weipa", "Eagle Farm", "Regional Queensland", "QLD"},
	models.WA:  {"Western Australia", "Perth", "Fremantle", "Albany", "Broome", "Kalgoorlie", "Geraldton", "Esperance", "Kwinana", "Bunbury", "Rockingham", "Cannington", "Narrogin", "Moora", "Regional Western Australia", "WA"},
	models.NT:  {"Northern Territory", "Darwin", "Alice Springs", "Regional Northern Territory", "NT"},
	models.SA:  {"South Australia", "Adelaide", "Whyalla", "Port Pirie", "Port Lincoln", "Gawler", "Regional South Australia"},
	models.ACT: {"Australian Capital Territory", "Canberra", "Regional ACT", "ACT"},
	models.TAS: {"Tasmania", "Hobart", "Launceston", "Regional Tasmania", "TAS"},

	models.AUSTRALIA: {"Australia", "Australia Wide", "Melbourne - CBD and Inner Suburbs, AU"},
}

// australianPlaces is the reverse lookup, one place to one state.
var australianPlaces = func() map[string]models.LocationType {
	places := map[string]models.LocationType{}
	for state, cities := range australianStateCities {
		for _, city := range cities {
			places[city] = state
		}
	}
	return places
}()

// ambiguousInFreeText are places that must only ever be matched as a whole
// location label. Each is a real place and each is also an ordinary word, so
// scanning a job title for them finds the wrong thing: the "Sale" in a
// marketing role, the "Orange" in a company name.
var ambiguousInFreeText = map[string]bool{
	"Sale": true, "Orange": true, "Richmond": true, "Roma": true,
	"Moree": true, "Moora": true, "Emerald": true, "Botany": true,
}

// stateAbbreviations are matched case-sensitively in free text. "Graduate
// Program - NSW" is unambiguous; "nsw" inside a word is not, and neither is a
// lowercase "act" or "sa".
var stateAbbreviations = map[string]models.LocationType{
	"ACT": models.ACT, "NSW": models.NSW, "NT": models.NT,
	"QLD": models.QLD, "SA": models.SA, "TAS": models.TAS,
	"VIC": models.VIC, "WA": models.WA,
}

var abbreviationPattern = regexp.MustCompile(`\b(ACT|NSW|NT|QLD|SA|TAS|VIC|WA)\b`)

// nationwidePattern is the last resort: a listing that names the country and no
// place inside it.
var nationwidePattern = regexp.MustCompile(`(?i)\baustralia\b`)

// australianPattern finds a place named anywhere in free text. It is built from
// the same table minus freeTextExclusions, because a title is not a location
// field: it is written by a recruiter and full of ordinary words.
var australianPattern = func() *regexp.Regexp {
	places := make([]string, 0, len(australianPlaces))
	for place := range australianPlaces {
		if _, isAbbreviation := stateAbbreviations[place]; isAbbreviation {
			continue // matched case-sensitively below
		}
		if !ambiguousInFreeText[place] && !nationwidePattern.MatchString(place) {
			places = append(places, place)
		}
	}
	// Longest first, so "South Australia" wins over "Australia" and "New South
	// Wales" over "Wales" style overlaps.
	sort.Slice(places, func(i, j int) bool { return len(places[i]) > len(places[j]) })

	quoted := make([]string, len(places))
	for i, place := range places {
		quoted[i] = regexp.QuoteMeta(place)
	}

	return regexp.MustCompile(`(?i)\b(` + strings.Join(quoted, "|") + `)\b`)
}()

// AustralianLocationFor maps one location label to a state, if it names a place
// this package knows. It matches the whole label first and then looks inside
// it, so both "Victoria" and "Hobart TAS" resolve.
func AustralianLocationFor(label string) (models.LocationType, bool) {
	trimmed := strings.TrimSpace(label)

	for place, location := range australianPlaces {
		if strings.EqualFold(place, trimmed) {
			return location, true
		}
	}

	if found := AustralianLocationsIn(trimmed); len(found) > 0 {
		return found[0], true
	}

	return "", false
}

// AustralianLocationsIn returns every Australian state named anywhere in the
// text, in a stable order. A listing advertised for "Brisbane, Gold Coast,
// Sunshine Coast" is one state; one for "Sydney and Melbourne" is two.
func AustralianLocationsIn(text string) []models.LocationType {
	seen := map[models.LocationType]bool{}
	found := []models.LocationType{}

	add := func(location models.LocationType) {
		if !seen[location] {
			seen[location] = true
			found = append(found, location)
		}
	}

	for _, match := range australianPattern.FindAllString(text, -1) {
		for place, location := range australianPlaces {
			if strings.EqualFold(place, match) {
				add(location)
				break
			}
		}
	}

	for _, match := range abbreviationPattern.FindAllString(text, -1) {
		add(stateAbbreviations[match])
	}

	// Only when nothing more specific was named: "Australia" inside "Western
	// Australia" must not add a nationwide marker to a state.
	if len(found) == 0 && nationwidePattern.MatchString(text) {
		add(models.AUSTRALIA)
	}

	if len(found) == 0 {
		return nil
	}

	sort.Slice(found, func(i, j int) bool { return found[i] < found[j] })
	return found
}

// HasAustralianState reports whether the locations contain a specific
// Australian state or territory. AUSTRALIA (nationwide) and OTHERS do not
// count: neither pins the listing to a place inside the country.
func HasAustralianState(locations []models.LocationType) bool {
	for _, location := range locations {
		switch location {
		case models.AUSTRALIA, models.OTHERS, models.APAC:
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
		if location != models.OTHERS && location != models.APAC {
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
		sanitised = append(sanitised, OverseasLocationFor(candidates...))
	}

	Warn("Relabelled %q as OTHERS: no Australian state and overseas token %q found", title, token)

	return sanitised
}

var unmappedLocations unmappedTally

// RecordUnmappedLocation notes a location string that no state or city table
// matched, so the tables can be grown from real scrape output rather than
// guesswork. Counts are reported by ReportUnmappedLocations.
func RecordUnmappedLocation(source string, raw string) {
	unmappedLocations.record(source, raw)
}

// ReportUnmappedLocations logs every location string that fell through to
// OTHERS, most frequent first, and clears the tally.
func ReportUnmappedLocations() {
	unmappedLocations.report("location", "OTHERS")
}
