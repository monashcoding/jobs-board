package linkedin

import (
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"net/url"
	"strings"
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

func convertJobType(linkedinType string) models.JobType {
	switch strings.ToLower(linkedinType) {
	case "full-time":
		return models.FIRST_YEAR
	case "internship":
		return models.INTERN
	default:
		return models.UNKNOWN
	}
}

func parseLocations(location string) []models.LocationType {
	locations := []models.LocationType{}

	// Simple mapping of common locations
	if strings.Contains(location, "Melbourne") {
		locations = append(locations, models.VIC)
	} else if strings.Contains(location, "Sydney") {
		locations = append(locations, models.NSW)
	} else if strings.Contains(location, "Remote") {
		locations = append(locations, models.REMOTE)
	} else {
		locations = append(locations, models.AUSTRALIA)
	}

	return locations
}
