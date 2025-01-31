package linkedin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/unblocker"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type LinkedInScraper struct {
	unblocker   unblocker.Service
	baseURL     string
	maxListings int
}

func NewLinkedInScraper() *LinkedInScraper {
	config := unblocker.Config{
		Features: unblocker.Features{
			UseRateLimiting: true,
			UseIPRotation:   true,
		},
		BrightData: &unblocker.BrightDataConfig{
			CustomerID:  os.Getenv("BRIGHTDATA_CUSTOMER_ID"),
			ZoneName:    os.Getenv("BRIGHTDATA_ZONE_NAME"),
			Password:    os.Getenv("BRIGHTDATA_PASSWORD"),
			Country:     "au",
			UseLocalDNS: true,
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

	return &LinkedInScraper{
		unblocker: unblockerService,
		baseURL:   "https://www.linkedin.com",
	}
}

func (s *LinkedInScraper) Name() string {
	return "LinkedIn"
}

func (s *LinkedInScraper) URL() string {
	return s.baseURL
}

func (s *LinkedInScraper) SetMaxListings(maxListings int) {
	s.maxListings = maxListings
}

func (s *LinkedInScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	listings := []models.Listing{}

	jobs, err := s.getAllJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all jobs: %w", err)
	}

	utils.Log("Found %d jobs in search results. Starting to parse individual jobs. \n", len(jobs))

	for i, job := range jobs {
		utils.Log("Processing job %d/%d with ID %s", i+1, len(jobs), job.ID)
		listing, err := s.processJob(ctx, job)
		if err != nil {
			utils.Error("Failed to process job %s: %v", job.ID, err)
			continue
		}
		listings = append(listings, listing)
	}

	return listings, nil
}

func (s *LinkedInScraper) getAllJobs(ctx context.Context) ([]Job, error) {
	var allJobs []Job
	start := 0

	for {
		utils.Log("Fetching jobs page starting at offset %d", start)

		searchParams := map[string]string{
			"keywords": `(graduate OR junior OR intern) (
                "software engineer" OR 
                "software developer" OR 
                "data engineer" OR 
                "data scientist" OR 
                "machine learning" OR 
                "security engineer" OR 
                "cyber security" OR 
                "devops engineer" OR 
                "site reliability engineer" OR 
                "sre" OR 
                "full stack" OR 
                "frontend engineer" OR 
                "backend engineer" OR 
                "cloud engineer" OR 
                "systems engineer" OR
                "quantitative trader" OR 
                "quant developer" OR 
                "algorithmic trading" OR 
                "quant analyst" OR 
                "quantitative analyst" OR 
                "trading systems"
            )`,
			"location": "Australia",
			"distance": "50",
			"f_E":      "1,2",      // 1=Internship, 2=Entry level
			"f_JT":     "F,I",      // F=Full-time, I=Internship
			"f_TPR":    "r2592000", // Past month
			"start":    fmt.Sprintf("%d", start),
		}

		searchURL := buildSearchURL(s.baseURL, searchParams)

		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := s.unblocker.MakeRequest(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch search results: %w", err)
		}
		defer resp.Body.Close()

		// Save search results response
		if err := s.saveResponse(resp, "search"); err != nil {
			utils.Error("Failed to save search response: %v", err)
		}

		jobs, err := parseSearchResults(resp)
		if err != nil {
			return nil, fmt.Errorf("failed to parse search results: %w", err)
		}

		// No more results
		if len(jobs) == 0 {
			break
		}

		allJobs = append(allJobs, jobs...)

		// Check if we've hit our maximum
		if s.maxListings > 0 && len(allJobs) >= s.maxListings {
			allJobs = allJobs[:s.maxListings]
			break
		}

		// Prepare for next page
		start += len(jobs)
	}

	return allJobs, nil
}

func (s *LinkedInScraper) saveResponse(resp *http.Response, prefix string) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Create debug directory if it doesn't exist
	debugDir := "debug/linkedin"
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	// Save response with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s/%s_%s.html", debugDir, prefix, timestamp)

	if err := os.WriteFile(filename, body, 0644); err != nil {
		return fmt.Errorf("failed to save response: %w", err)
	}

	// Important: Create new reader for resp.Body since we consumed it
	resp.Body = io.NopCloser(bytes.NewReader(body))

	return nil
}
