package linkedin

import (
	"bytes"
	"context"
	"fmt"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/unblocker"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
	"io"
	"net/http"
	"os"
	"time"
)

type LinkedInScraper struct {
	unblocker unblocker.Service
	baseURL   string
}

func NewLinkedInScraper() *LinkedInScraper {
	// Configure unblocker with strict rate limiting for LinkedIn
	config := unblocker.Config{
		Features: unblocker.Features{
			UseRateLimiting: true,
			UseIPRotation:   true,
			//UseBrowserProfiles: true,
		},
		BrightData: &unblocker.BrightDataConfig{
			CustomerID:  os.Getenv("BRIGHTDATA_CUSTOMER_ID"),
			ZoneName:    os.Getenv("BRIGHTDATA_ZONE_NAME"),
			Password:    os.Getenv("BRIGHTDATA_PASSWORD"),
			Country:     "au",
			UseLocalDNS: true,
		},
		Rate: &unblocker.RateLimitConfig{
			RequestsPerMinute: 15, // More conservative rate limiting for LinkedIn
			BaseDelay:         time.Second * 15,
			MaxJitter:         time.Second * 20,
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

func (s *LinkedInScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	listings := []models.Listing{}

	MAX_JOBS_TO_SCRAPE := 5

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
		"start":    "0",
	}

	searchURL := buildSearchURL(s.baseURL, searchParams)
	//utils.Log("Searching for jobs with this URL: %s", searchURL)

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

	//utils.Log("Got search response with status: %d", resp.StatusCode)

	jobs, err := parseSearchResults(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	if MAX_JOBS_TO_SCRAPE > 0 && len(jobs) > MAX_JOBS_TO_SCRAPE {
		jobs = jobs[:MAX_JOBS_TO_SCRAPE]
		utils.Log("Trimming job results to %d", MAX_JOBS_TO_SCRAPE)

	}

	utils.Log("Found %d jobs in search results. Starting to parse individual jobs. \n", len(jobs))

	for i, job := range jobs {
		utils.Log("Processing job %d/%d", i+1, len(jobs))
		listing, err := s.processJob(ctx, job)
		if err != nil {
			utils.Error("Failed to process job %s: %v", job.ID, err)
			continue
		}
		listings = append(listings, listing)
	}

	utils.Log("Finished scraping. Found %d valid listings", len(listings))
	return listings, nil
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

	//utils.Log("Saved response to %s", filename)
	return nil
}
