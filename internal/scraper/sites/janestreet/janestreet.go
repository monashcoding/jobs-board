package janestreet

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

type JaneStreetScraper struct {
	unblocker unblocker.Service
	jobLimit  int
}

type Job struct {
	ID           int    `json:"id"`
	Position     string `json:"position"`
	Category     string `json:"category"`
	Availability string `json:"availability"`
	City         string `json:"city"`
	Overview     string `json:"overview"`
	Team         string `json:"team"`
	Duration     string `json:"duration"`
	MinSalary    string `json:"min_salary"`
	MaxSalary    string `json:"max_salary"`
}

func NewJaneStreetScraper() *JaneStreetScraper {
	config := unblocker.Config{
		Features: unblocker.Features{
			UseRateLimiting: false,
			UseIPRotation:   true,
		},
		BrightData: &unblocker.BrightDataConfig{
			CustomerID: os.Getenv("BRIGHTDATA_CUSTOMER_ID"),
			ZoneName:   os.Getenv("BRIGHTDATA_ZONE_NAME"),
			Password:   os.Getenv("BRIGHTDATA_PASSWORD"),
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

	return &JaneStreetScraper{
		unblocker: unblockerService,
	}
}

func (s *JaneStreetScraper) Name() string {
	return "JaneStreet"
}

func (s *JaneStreetScraper) URL() string {
	return "https://www.janestreet.com"
}

func (s *JaneStreetScraper) SetJobLimit(limit int) {
	s.jobLimit = limit
}

func (s *JaneStreetScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	jobs, err := s.fetchJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jobs: %w", err)
	}

	var listings []models.Listing
	for _, job := range jobs {
		// Filter for HK jobs that are either internships or new grad positions
		if job.City == "HKG" && (strings.HasPrefix(job.Availability, "Summer Internship") || strings.HasPrefix(job.Availability, "Winter Internship") || job.Availability == "Full-Time: New Grad") {
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

	utils.Success("Found %d relevant Jane Street listings", len(listings))
	return listings, nil
}

func (s *JaneStreetScraper) fetchJobs(ctx context.Context) ([]Job, error) {
	jobsURL := s.URL() + "/jobs/main.json"

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

	var jobs []Job
	if err := json.Unmarshal(body, &jobs); err != nil {
		return nil, fmt.Errorf("failed to parse jobs JSON: %w", err)
	}

	return jobs, nil
}

func (s *JaneStreetScraper) convertToListing(job Job) (models.Listing, error) {
	// Determine job type based on availability
	var jobType models.JobType
	if strings.Contains(job.Availability, "Internship") {
		jobType = models.INTERN
	} else {
		jobType = models.GRADUATE
	}

	// Generate application URL
	applicationURL := fmt.Sprintf("%s/join-jane-street/position/%d/", s.URL(), job.ID)

	// Set company details
	companyWebsite := s.URL()
	logo := "https://www.janestreet.com/assets/logo_horizontal-177b793d3559af186aa24ab7a93bea2c04d723f02e2ea905263735abe031f15b.png"

	listing := models.Listing{
		Title:       job.Position,
		Description: &job.Overview,
		Company: models.Company{
			Name:    "Jane Street",
			Website: &companyWebsite,
			Logo:    &logo,
		},
		ApplicationURL: &applicationURL,
		WorkingRights: []models.WorkingRight{
			models.AUS_CITIZEN_PR,
			models.NZ_CITIZEN_PR,
			models.INTERNATIONAL,
			models.OTHER_RIGHTS,
		},
		SourceURLs:    []string{applicationURL},
		Type:          &jobType,
		Locations:     []models.LocationType{models.OTHERS},
		StudyFields:   []string{"IT & Computer Science", "Engineering & Mathematics"},
		Source:        models.JANESTREET,
		IndustryField: models.QUANT_TRADING,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	return listing, nil
}
