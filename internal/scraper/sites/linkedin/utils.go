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
	// Split by "view/" to get the part after it
	parts := strings.Split(href, "view/")
	if len(parts) != 2 {
		return ""
	}

	// Get everything up to the first "?" if it exists
	idPart := strings.Split(parts[1], "?")[0]

	// Clean up any remaining non-numeric characters
	id := ""
	for _, char := range idPart {
		if char >= '0' && char <= '9' {
			id += string(char)
		}
	}

	return id
}

func convertJobType(linkedinType string) models.JobType {
	switch strings.ToLower(linkedinType) {
	case "full-time":
		return models.FIRST_YEAR
	case "internship":
		return models.INTERN
	default:
		return models.GRADUATE
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
