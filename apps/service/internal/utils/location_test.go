package utils

import (
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

func TestFindOverseasToken(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		overseas bool
	}{
		{"hong kong in title", "Software Engineering Graduate Programme - Hong Kong", true},
		{"singapore in company", "Notify Me - Deutsche Bank Singapore Graduate Jobs", true},
		{"uk abbreviation", "Internships in London, UK", true},
		{"new zealand", "2027 NZI Graduate Program in New Zealand", true},
		{"plain australian title", "2027 Technology Graduate Program - Melbourne", false},
		{"no false positive inside word", "Graduate Analyst, Franzia", false},
		{"nz not matched inside word", "Graduate Bronze Program", false},
		{"uk not matched inside word", "Graduate Ukulele Technician", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := FindOverseasToken(tc.input)
			if got != tc.overseas {
				t.Errorf("FindOverseasToken(%q) = %v, want %v", tc.input, got, tc.overseas)
			}
		})
	}
}

func TestSanitiseAustralianLocations(t *testing.T) {
	cases := []struct {
		name  string
		locs  []models.LocationType
		title string
		raw   []string
		want  []models.LocationType
	}{
		{
			// The bug this whole change exists for: an overseas internship
			// carrying a nationwide AUSTRALIA marker showed up under the
			// Australia filter.
			name:  "overseas title strips bogus AUSTRALIA",
			locs:  []models.LocationType{models.AUSTRALIA, models.OTHERS},
			title: "Internships in London, UK",
			raw:   []string{"Australia"},
			want:  []models.LocationType{models.OTHERS},
		},
		{
			// Stripping AUSTRALIA leaves nothing, so the listing is placed by
			// the token that disqualified it: Singapore is APAC, not the
			// undifferentiated OTHERS it used to become.
			name:  "overseas raw location strips bogus AUSTRALIA",
			locs:  []models.LocationType{models.AUSTRALIA},
			title: "Graduate Programme",
			raw:   []string{"Singapore"},
			want:  []models.LocationType{models.APAC},
		},
		{
			// A program genuinely running in Sydney and Singapore is still an
			// Australian opportunity.
			name:  "state present keeps AUSTRALIA and overseas marker",
			locs:  []models.LocationType{models.NSW, models.OTHERS},
			title: "Consulting Analyst Program - Australia and New Zealand",
			raw:   []string{"Sydney", "Auckland"},
			want:  []models.LocationType{models.NSW, models.OTHERS},
		},
		{
			name:  "genuine australia-wide listing untouched",
			locs:  []models.LocationType{models.AUSTRALIA},
			title: "2027 Graduate Program",
			raw:   []string{"Australia Wide"},
			want:  []models.LocationType{models.AUSTRALIA},
		},
		{
			name:  "already OTHERS untouched",
			locs:  []models.LocationType{models.OTHERS},
			title: "Software Engineering Graduate Programme - Hong Kong",
			raw:   []string{"Hong Kong"},
			want:  []models.LocationType{models.OTHERS},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitiseAustralianLocations(tc.locs, tc.title, tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestHasAustralianStateAndIsAustralian(t *testing.T) {
	cases := []struct {
		name        string
		locs        []models.LocationType
		hasState    bool
		isAustralia bool
	}{
		{"state", []models.LocationType{models.VIC}, true, true},
		{"nationwide only", []models.LocationType{models.AUSTRALIA}, false, true},
		{"others only", []models.LocationType{models.OTHERS}, false, false},
		{"state and others", []models.LocationType{models.OTHERS, models.QLD}, true, true},
		{"empty", []models.LocationType{}, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasAustralianState(tc.locs); got != tc.hasState {
				t.Errorf("HasAustralianState(%v) = %v, want %v", tc.locs, got, tc.hasState)
			}
			if got := IsAustralian(tc.locs); got != tc.isAustralia {
				t.Errorf("IsAustralian(%v) = %v, want %v", tc.locs, got, tc.isAustralia)
			}
		})
	}
}

// A token nothing places is filed as OTHERS. That is right for London and New
// York, but an APAC city landing there would be a silent mistake, so the APAC
// set is checked against the token list it is drawn from.
func TestAPACTokensAreOverseasTokens(t *testing.T) {
	for token := range apacTokens {
		found := false
		for _, known := range overseasTokens {
			if known == token {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("APAC names %q, which is not an overseas token, so it can never match", token)
		}
	}
}

func TestOverseasLocationFor(t *testing.T) {
	cases := map[string]models.LocationType{
		"Hong Kong":                  models.APAC,
		"singapore":                  models.APAC,
		"Graduate Trader, Tokyo":     models.APAC,
		"Kuala Lumpur, Malaysia":     models.APAC,
		"London, UK":                 models.OTHERS,
		"New York":                   models.OTHERS,
		"Auckland":                   models.OTHERS,
		"Graduate Software Engineer": models.OTHERS,
		"":                           models.OTHERS,
	}

	for text, want := range cases {
		if got := OverseasLocationFor(text); got != want {
			t.Errorf("OverseasLocationFor(%q) = %q, want %q", text, got, want)
		}
	}
}

// APAC is a location outside Australia. Getting this wrong would put every
// Hong Kong listing back on an Australian job board.
func TestAPACIsNotAustralian(t *testing.T) {
	apac := []models.LocationType{models.APAC}
	if IsAustralian(apac) {
		t.Error("IsAustralian(APAC) = true; APAC listings would reach the board")
	}
	if HasAustralianState(apac) {
		t.Error("HasAustralianState(APAC) = true; APAC listings would earn the state bonus")
	}
	if !IsAustralian([]models.LocationType{models.VIC, models.APAC}) {
		t.Error("a listing in both Melbourne and Singapore is still Australian")
	}
}

// The table only earns its place if it knows the cities the sources actually
// send. These are the labels behind the 18 listings that were filed as OTHERS.
func TestAustralianLocationFor(t *testing.T) {
	cases := map[string]models.LocationType{
		"Sydney":     models.NSW,
		"sydney":     models.NSW,
		"Melbourne":  models.VIC,
		"Hobart TAS": models.TAS,
		"Gold Coast": models.QLD,
		"Queensland": models.QLD,
		"Perth":      models.WA,
		"Canberra":   models.ACT,
		"Victoria":   models.VIC,
		"Australia":  models.AUSTRALIA,
		// The nationwide marker must not win inside a state name.
		"Western Australia": models.WA,
		"South Australia":   models.SA,
	}

	for label, want := range cases {
		got, ok := AustralianLocationFor(label)
		if !ok {
			t.Errorf("AustralianLocationFor(%q) not recognised", label)
			continue
		}
		if got != want {
			t.Errorf("AustralianLocationFor(%q) = %q, want %q", label, got, want)
		}
	}

	for _, label := range []string{"Singapore", "London", "Remote", ""} {
		if got, ok := AustralianLocationFor(label); ok {
			t.Errorf("AustralianLocationFor(%q) = %q, want not recognised", label, got)
		}
	}
}

func TestAustralianLocationsInFindsEveryStateNamed(t *testing.T) {
	got := AustralianLocationsIn("Graduate Program – Brisbane, Gold Coast, Sunshine Coast")
	if len(got) != 1 || got[0] != models.QLD {
		t.Errorf("three Queensland cities should be one state, got %v", got)
	}

	got = AustralianLocationsIn("Graduate Engineer - Sydney and Melbourne")
	if len(got) != 2 {
		t.Errorf("two states named, got %v", got)
	}

	if got := AustralianLocationsIn("Software Engineer, Hong Kong"); got != nil {
		t.Errorf("nothing Australian named, got %v", got)
	}
}
