package airwallex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/unblocker"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

const (
	graphqlEndpoint = "https://jobs.ashbyhq.com/api/non-user-graphql"
	orgSlug         = "airwallex"
)

// cityToState maps lowercase city names found in Ashby locationName to AU states.
var cityToState = map[string]models.LocationType{
	"melbourne": models.VIC,
	"sydney":    models.NSW,
	"brisbane":  models.QLD,
	"perth":     models.WA,
	"adelaide":  models.SA,
	"canberra":  models.ACT,
	"darwin":    models.NT,
	"hobart":    models.TAS,
}

type AirwallexScraper struct {
	unblocker unblocker.Service
	jobLimit  int
}

// jobBoardResponse is the GraphQL response for ApiJobBoardWithTeams.
type jobBoardResponse struct {
	Data struct {
		JobBoard struct {
			JobPostings []jobPosting `json:"jobPostings"`
		} `json:"jobBoard"`
	} `json:"data"`
}

type jobPosting struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	LocationID         string `json:"locationId"`
	LocationName       string `json:"locationName"`
	WorkplaceType      string `json:"workplaceType"`
	EmploymentType     string `json:"employmentType"`
	SecondaryLocations []struct {
		LocationID   string `json:"locationId"`
		LocationName string `json:"locationName"`
	} `json:"secondaryLocations"`
}

// jobDetailResponse is the GraphQL response for ApiJobPosting.
type jobDetailResponse struct {
	Data struct {
		JobPosting struct {
			DescriptionHtml string `json:"descriptionHtml"`
		} `json:"jobPosting"`
	} `json:"data"`
}

func NewAirwallexScraper() *AirwallexScraper {
	config := unblocker.Config{
		Features: unblocker.Features{
			UseRateLimiting: false,
			UseIPRotation:   true,
		},
		BrightData: &unblocker.BrightDataConfig{
			CustomerID: os.Getenv("BRIGHTDATA_CUSTOMER_ID"),
			ZoneName:   os.Getenv("BRIGHTDATA_ZONE_NAME"),
			Password:   os.Getenv("BRIGHTDATA_PASSWORD"),
			Country:    "au",
		},
		Rate: &unblocker.RateLimitConfig{
			BaseDelay: time.Second * 1,
			MaxJitter: time.Second * 1,
		},
	}

	unblockerService, err := unblocker.NewService(config)
	if err != nil {
		utils.Error("Failed to initialize unblocker: %v", err)
		return nil
	}

	return &AirwallexScraper{unblocker: unblockerService}
}

func (s *AirwallexScraper) Name() string          { return "Airwallex" }
func (s *AirwallexScraper) URL() string           { return "https://www.airwallex.com" }
func (s *AirwallexScraper) SetJobLimit(limit int) { s.jobLimit = limit }

func (s *AirwallexScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	jobs, err := s.fetchAllJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs: %w", err)
	}

	var listings []models.Listing
	for _, job := range jobs {
		if !s.isAustralianJob(job) || !s.isRelevantJob(job) {
			continue
		}

		details, err := s.fetchJobDetails(ctx, job.ID)
		if err != nil {
			utils.Error("Failed to fetch details for job %s: %v", job.ID, err)
			// Continue with nil details — description will be omitted.
		}

		listings = append(listings, s.convertToListing(job, details))
	}

	if s.jobLimit > 0 && len(listings) > s.jobLimit {
		listings = listings[:s.jobLimit]
	}

	utils.Success("Found %d relevant Airwallex listings", len(listings))
	return listings, nil
}

func (s *AirwallexScraper) fetchAllJobs(ctx context.Context) ([]jobPosting, error) {
	const query = `query ApiJobBoardWithTeams($organizationHostedJobsPageName: String!) {
  jobBoard: jobBoardWithTeams(organizationHostedJobsPageName: $organizationHostedJobsPageName) {
    jobPostings {
      id
      title
      locationId
      locationName
      workplaceType
      employmentType
      secondaryLocations {
        locationId
        locationName
      }
    }
  }
}`

	payload := map[string]interface{}{
		"operationName": "ApiJobBoardWithTeams",
		"variables":     map[string]string{"organizationHostedJobsPageName": orgSlug},
		"query":         query,
	}

	body, err := s.makeGraphQLRequest(ctx, "ApiJobBoardWithTeams", payload)
	if err != nil {
		return nil, err
	}

	var resp jobBoardResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse job board response: %w", err)
	}

	return resp.Data.JobBoard.JobPostings, nil
}

func (s *AirwallexScraper) fetchJobDetails(ctx context.Context, jobID string) (*jobDetailResponse, error) {
	const query = `query ApiJobPosting($organizationHostedJobsPageName: String!, $jobPostingId: String!) {
  jobPosting(organizationHostedJobsPageName: $organizationHostedJobsPageName, jobPostingId: $jobPostingId) {
    descriptionHtml
  }
}`

	payload := map[string]interface{}{
		"operationName": "ApiJobPosting",
		"variables": map[string]string{
			"organizationHostedJobsPageName": orgSlug,
			"jobPostingId":                   jobID,
		},
		"query": query,
	}

	body, err := s.makeGraphQLRequest(ctx, "ApiJobPosting", payload)
	if err != nil {
		return nil, err
	}

	var resp jobDetailResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse job detail response: %w", err)
	}

	return &resp, nil
}

func (s *AirwallexScraper) makeGraphQLRequest(ctx context.Context, op string, payload interface{}) ([]byte, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL payload: %w", err)
	}

	req, err := http.NewRequest("POST", graphqlEndpoint+"?op="+op, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.unblocker.MakeRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// isAustralianJob returns true if the job's primary or any secondary location is in Australia.
func (s *AirwallexScraper) isAustralianJob(job jobPosting) bool {
	if isAULocationName(job.LocationName) {
		return true
	}
	for _, sec := range job.SecondaryLocations {
		if isAULocationName(sec.LocationName) {
			return true
		}
	}
	return false
}

// isAULocationName checks if a location name corresponds to an Australian city/office.
func isAULocationName(name string) bool {
	return strings.HasPrefix(name, "AU - ") || strings.Contains(strings.ToLower(name), "australia")
}

// isRelevantJob returns true for intern and graduate roles.
func (s *AirwallexScraper) isRelevantJob(job jobPosting) bool {
	lower := strings.ToLower(job.Title)
	return strings.Contains(lower, "intern") ||
		strings.Contains(lower, "graduate") ||
		strings.Contains(lower, "grad")
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// stripHTML removes HTML tags and decodes HTML entities.
func stripHTML(raw string) string {
	stripped := htmlTagRe.ReplaceAllString(raw, " ")
	decoded := html.UnescapeString(stripped)
	// Collapse multiple spaces/newlines.
	return strings.Join(strings.Fields(decoded), " ")
}

func (s *AirwallexScraper) convertToListing(job jobPosting, details *jobDetailResponse) models.Listing {
	var jobType models.JobType
	lower := strings.ToLower(job.Title)
	if strings.Contains(lower, "intern") {
		jobType = models.INTERN
	} else {
		jobType = models.GRADUATE
	}

	applicationURL := fmt.Sprintf("https://jobs.ashbyhq.com/airwallex/%s", job.ID)
	companyWebsite := s.URL()
	logo := "https://app.ashbyhq.com/api/images/org-theme-wordmark/6e23f7f7-cd7d-4ce0-9c98-a110a190ef11/189f2f03-7a70-4685-8fa1-542ff3f2df0e/ad10e6e7-1ce7-4bb6-93ea-5559a187f21b.png"

	var description *string
	if details != nil && details.Data.JobPosting.DescriptionHtml != "" {
		desc := stripHTML(details.Data.JobPosting.DescriptionHtml)
		description = &desc
	}

	return models.Listing{
		Title:       job.Title,
		Description: description,
		Company: models.Company{
			Name:    "Airwallex",
			Website: &companyWebsite,
			Logo:    &logo,
		},
		ApplicationURL: &applicationURL,
		WorkingRights: []models.WorkingRight{
			models.AUS_CITIZEN_PR,
			models.NZ_CITIZEN_PR,
			models.INTERNATIONAL,
		},
		SourceURLs:    []string{applicationURL},
		Type:          &jobType,
		Locations:     s.mapLocations(job),
		StudyFields:   []string{"IT & Computer Science", "Engineering & Mathematics"},
		Source:        models.AIRWALLEX,
		IndustryField: models.TECH,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// mapLocations maps Ashby location names to models.LocationType values.
func (s *AirwallexScraper) mapLocations(job jobPosting) []models.LocationType {
	seen := make(map[models.LocationType]bool)

	addLocation := func(name string) {
		if !isAULocationName(name) {
			return
		}
		seen[models.AUSTRALIA] = true
		lower := strings.ToLower(name)
		for city, state := range cityToState {
			if strings.Contains(lower, city) {
				seen[state] = true
				return
			}
		}
	}

	addLocation(job.LocationName)
	for _, sec := range job.SecondaryLocations {
		addLocation(sec.LocationName)
	}

	locs := make([]models.LocationType, 0, len(seen))
	for loc := range seen {
		locs = append(locs, loc)
	}
	return locs
}
