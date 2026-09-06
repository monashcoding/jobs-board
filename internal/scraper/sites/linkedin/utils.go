package linkedin

import (
	"net/url"
	"strings"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

func buildSearchURL(baseURL string, params map[string]string) string {
	u, _ := url.Parse(baseURL + "/jobs-guest/jobs/api/seeMoreJobPostings/search")
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func extractJobID(href string) string {
	// Split off query parameters
	base := strings.Split(href, "?")[0]

	// Split by hyphen and take last element
	parts := strings.Split(base, "-")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func parseLocations(location string) []models.LocationType {
	// The city-to-state table is shared with the other scrapers, so a place
	// added for one source is known to all of them.
	if australian := utils.AustralianLocationsIn(location); len(australian) > 0 {
		return australian
	}

	if strings.Contains(location, "Australia") {
		// A place we have no city mapping for, but LinkedIn still says it is in
		// Australia (e.g. "Scoresby, Victoria, Australia").
		utils.RecordUnmappedLocation("LinkedIn", location)
		return []models.LocationType{models.AUSTRALIA}
	}

	// Nothing identifies this as Australian. Defaulting to AUSTRALIA here used
	// to stamp overseas jobs as Australian, which is how they ended up under
	// the Australia filter.
	utils.RecordUnmappedLocation("LinkedIn", location)
	return []models.LocationType{utils.OverseasLocationFor(location)}
}
