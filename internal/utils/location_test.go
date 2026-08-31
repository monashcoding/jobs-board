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
			name:  "overseas raw location strips bogus AUSTRALIA",
			locs:  []models.LocationType{models.AUSTRALIA},
			title: "Graduate Programme",
			raw:   []string{"Singapore"},
			want:  []models.LocationType{models.OTHERS},
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
