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

func parseLocations(location string) []models.LocationType {
	locations := []models.LocationType{}

	// Simple mapping of common locations
	if strings.Contains(location, "Melbourne") {
		locations = append(locations, models.VIC)
	} else if strings.Contains(location, "Sydney") {
		locations = append(locations, models.NSW)
	} else if strings.Contains(location, "Brisbane") {
		locations = append(locations, models.QLD)
	} else if strings.Contains(location, "Perth") {
		locations = append(locations, models.WA)
	} else if strings.Contains(location, "Adelaide") {
		locations = append(locations, models.SA)
	} else if strings.Contains(location, "Canberra") {
		locations = append(locations, models.ACT)
	} else if strings.Contains(location, "Hobart") {
		locations = append(locations, models.TAS)
	} else if strings.Contains(location, "Darwin") {
		locations = append(locations, models.NT)
	} else {
		locations = append(locations, models.AUSTRALIA)
	}

	return locations
}
