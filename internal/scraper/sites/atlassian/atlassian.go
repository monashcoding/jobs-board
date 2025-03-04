package atlassian

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/unblocker"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type AtlassianScraper struct {
	unblocker unblocker.Service
	jobLimit  int
}

type JobListing struct {
	PortalJobPost struct {
		PortalID    int    `json:"portalId"`
		PortalURL   string `json:"portalUrl"`
		ID          int    `json:"id"`
		UpdatedDate string `json:"updatedDate"`
	} `json:"portalJobPost"`
	ID               int      `json:"id"`
	PortalID         int      `json:"portalId"`
	Title            string   `json:"title"`
	Locations        []string `json:"locations"`
	Category         string   `json:"category"`
	Overview         string   `json:"overview"`
	Responsibilities string   `json:"responsibilities"`
	Qualifications   string   `json:"qualifications"`
	ApplyURL         string   `json:"applyUrl"`
}

func NewAtlassianScraper() *AtlassianScraper {
	config := unblocker.Config{
		Features: unblocker.Features{
			UseRateLimiting: true,
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

	return &AtlassianScraper{
		unblocker: unblockerService,
	}
}

func (s *AtlassianScraper) SetJobLimit(limit int) {
	s.jobLimit = limit
}

func (s *AtlassianScraper) Name() string {
	return "Atlassian"
}

func (s *AtlassianScraper) URL() string {
	return "https://www.atlassian.com"
}

func (s *AtlassianScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	jobs, err := s.fetchJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs: %w", err)
	}

	var listings []models.Listing
	for _, job := range jobs {
		// Filter for Interns or Graduates and Australian locations
		if s.isRelevantJob(job) {
			listing, err := s.convertToListing(job)
			if err != nil {
				utils.Error("Failed to convert job %d: %v", job.ID, err)
				continue
			}
			listings = append(listings, listing)
		}
	}

	if s.jobLimit > 0 && (len(listings) > s.jobLimit) {
		listings = listings[:s.jobLimit]
	}
	utils.Success("Found %d relevant Atlassian listings", len(listings))
	return listings, nil
}

func (s *AtlassianScraper) fetchJobs(ctx context.Context) ([]JobListing, error) {
	jobsURL := s.URL() + "/endpoint/careers/listings"

	req, err := http.NewRequest("GET", jobsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.unblocker.MakeRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var jobs []JobListing
	if err := json.Unmarshal(body, &jobs); err != nil {
		return nil, fmt.Errorf("failed to parse jobs JSON: %w", err)
	}

	return jobs, nil
}

func (s *AtlassianScraper) isRelevantJob(job JobListing) bool {
	// Check if the job category is for Interns or Graduates
	isRelevantCategory := strings.Contains(strings.ToLower(job.Category), "intern") ||
		strings.Contains(strings.ToLower(job.Category), "graduate")

	// Check if the job location includes Australia
	isAustralianJob := false
	for _, location := range job.Locations {
		if strings.Contains(location, "Australia") {
			isAustralianJob = true
			break
		}
	}

	return isRelevantCategory && isAustralianJob
}

func (s *AtlassianScraper) convertToListing(job JobListing) (models.Listing, error) {
	// Determine job type based on category or title
	var jobType models.JobType
	if strings.Contains(strings.ToLower(job.Category), "intern") || strings.Contains(strings.ToLower(job.Title), "intern") {
		jobType = models.INTERN
	} else {
		jobType = models.GRADUATE
	}

	// Combine all text for the description
	description := job.Overview
	if job.Responsibilities != "" {
		description += "\n\n" + job.Responsibilities
	}
	if job.Qualifications != "" {
		description += "\n\n" + job.Qualifications
	}

	// Generate application URL
	applicationURL := job.ApplyURL

	// Set company details
	companyWebsite := s.URL()
	logo := "https://logo.clearbit.com/atlassian.com"

	listing := models.Listing{
		Title:       job.Title,
		Description: &description,
		Company: models.Company{
			Name:    "Atlassian",
			Website: &companyWebsite,
			Logo:    &logo,
		},
		ApplicationURL: &applicationURL,
		WorkingRights: []models.WorkingRight{
			models.AUS_CITIZEN_PR,
			models.NZ_CITIZEN_PR,
		},
		SourceURLs:    []string{job.PortalJobPost.PortalURL},
		Type:          &jobType,
		Locations:     []models.LocationType{models.AUSTRALIA, models.NSW},
		StudyFields:   []string{"IT & Computer Science", "Engineering & Mathematics"},
		Source:        models.ATLASSIAN,
		IndustryField: models.BIG_TECH,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	return listing, nil
}
