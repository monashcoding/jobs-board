package steps

import (
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// Prosple labels a job "Sydney" where its map only knew "New South Wales", so
// 18 plainly Australian listings were stored as OTHERS and kept off the board.
// The converter knows the cities now; this is the second line of defence, for
// a source that gives no usable location at all.
func TestRecoverLocationPromotesAnAustralianTitle(t *testing.T) {
	cases := map[string]models.LocationType{
		"Secure Kernel Engineering Intern, Sydney":       models.NSW,
		"Junior Civil Engineer - Melbourne":              models.VIC,
		"Secondary Teacher - Mathematics | Hobart TAS":   models.TAS,
		"Graduate Program – Brisbane, Gold Coast":        models.QLD,
		"Robotic Product Specialist - Perth":             models.WA,
		"Queensland Government Digital Graduate Program": models.QLD,
	}

	for title, want := range cases {
		recovered := RecoverLocation(models.Listing{
			Title:     title,
			Locations: []models.LocationType{models.OTHERS},
		})
		if len(recovered.Locations) == 0 || recovered.Locations[0] != want {
			t.Errorf("%q recovered as %v, want %q", title, recovered.Locations, want)
		}
	}
}

// Overseas wins. Recovering a location must never promote a foreign listing,
// which is the failure the whole location gate exists to prevent.
func TestRecoverLocationNeverPromotesAnOverseasListing(t *testing.T) {
	cases := map[string]models.LocationType{
		"Quantitative Trader, Hong Kong": models.APAC,
		"Software Engineer - Singapore":  models.APAC,
		"Graduate Analyst, London":       models.OTHERS,
		"Software Engineer Intern":       models.OTHERS,
		"Internships in London, UK":      models.OTHERS,
	}

	for title, want := range cases {
		recovered := RecoverLocation(models.Listing{
			Title:     title,
			Locations: []models.LocationType{models.OTHERS},
		})
		if len(recovered.Locations) != 1 || recovered.Locations[0] != want {
			t.Errorf("%q became %v, want %q", title, recovered.Locations, want)
		}
	}
}

// A listing that already knows where it is knows better than its own title.
func TestRecoverLocationLeavesAKnownLocationAlone(t *testing.T) {
	original := []models.LocationType{models.VIC}
	recovered := RecoverLocation(models.Listing{
		Title:     "Graduate Engineer, Sydney office visits",
		Locations: original,
	})
	if len(recovered.Locations) != 1 || recovered.Locations[0] != models.VIC {
		t.Errorf("locations = %v, want them left at %v", recovered.Locations, original)
	}
}
