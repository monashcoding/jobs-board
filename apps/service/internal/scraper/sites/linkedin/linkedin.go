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

// JobCheckpoint remembers listings across attempts so an interrupted scrape can
// pick up where it stopped. It is an interface rather than the concrete store so
// the scraper keeps working without one.
type JobCheckpoint interface {
	Seen(key string) (models.Listing, bool)
	Record(ctx context.Context, key string, listing models.Listing)
}

type LinkedInScraper struct {
	unblocker  unblocker.Service
	jobLimit   int
	checkpoint JobCheckpoint
}

// SetCheckpoint gives the scraper somewhere to bank each job it fetches. Without
// one the scraper simply starts from the beginning every time.
func (s *LinkedInScraper) SetCheckpoint(c JobCheckpoint) {
	s.checkpoint = c
}

func NewLinkedInScraper() *LinkedInScraper {
	config := unblocker.Config{
		Features: unblocker.Features{
			UseRateLimiting: false,
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
	}
}

func (s *LinkedInScraper) Name() string {
	return "LinkedIn"
}

func (s *LinkedInScraper) URL() string {
	return "https://www.linkedin.com"
}

func (s *LinkedInScraper) SetJobLimit(limit int) {
	s.jobLimit = limit
}

func (s *LinkedInScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	jobs, err := s.getAllJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all jobs: %w", err)
	}

	utils.Log("Found %d jobs in search results. Starting to parse individual jobs.", len(jobs))

	return s.processJobs(ctx, jobs), nil
}

// reachedJobLimit reports whether enough listings have been gathered.
func (s *LinkedInScraper) reachedJobLimit(gathered int) bool {
	if s.jobLimit > 0 && gathered >= s.jobLimit {
		utils.Log("Reached job limit (%d), stopping job processing", s.jobLimit)
		return true
	}
	return false
}

// processJobs turns each search result into a listing, reusing anything an
// earlier attempt already fetched.
func (s *LinkedInScraper) processJobs(ctx context.Context, jobs []Job) []models.Listing {
	var listings []models.Listing

	// Process jobs with job limit in mind
	resumed := 0
	for i, job := range jobs {
		// A job already fetched by an earlier attempt costs a map lookup rather
		// than a proxied page load. This is the bulk of a LinkedIn scrape: 839
		// individual job pages, which is why a run that died at 430 of them used
		// to throw away a quarter of an hour of work.
		if s.checkpoint != nil {
			if listing, ok := s.checkpoint.Seen(job.ID); ok {
				listings = append(listings, listing)
				resumed++
				// A recovered listing counts against the limit exactly as a
				// freshly fetched one does, or a resumed run would fetch past
				// the limit it was given.
				if s.reachedJobLimit(len(listings)) {
					break
				}
				continue
			}
		}

		utils.Log("Processing job %d/%d with ID %s", i+1, len(jobs), job.ID)
		listing, err := s.processJob(ctx, job)
		if err != nil {
			utils.Error("Failed to process job %s: %v", job.ID, err)
			continue
		}
		if s.checkpoint != nil {
			s.checkpoint.Record(ctx, job.ID, listing)
		}
		listings = append(listings, listing)

		if s.reachedJobLimit(len(listings)) {
			break
		}
	}

	if resumed > 0 {
		utils.Success("Recovered %d job pages from an earlier attempt, refetched %d.", resumed, len(listings)-resumed)
	}

	// Trim the list to match our target exactly if needed
	if s.jobLimit > 0 && len(listings) > s.jobLimit {
		listings = listings[:s.jobLimit]
		utils.Log("Trimmed listings to match job limit exactly (%d)", s.jobLimit)
	}

	return listings
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
				"carsales" OR
                "data engineer" OR 
                "data scientist" OR 
                "machine learning" OR 
                "security engineer" OR 
                "cyber security" OR 
                "full stack" OR 
                "frontend engineer" OR 
                "backend engineer" OR 
                "cloud engineer" OR 
                "quantitative trader"
            )`,
			"location": "Australia",
			"distance": "50",
			"f_E":      "1,2",      // 1=Internship, 2=Entry level
			"f_JT":     "F,I",      // F=Full-time, I=Internship
			"f_TPR":    "r2592000", // Past month
			"start":    fmt.Sprintf("%d", start),
		}

		searchURL := buildSearchURL(s.URL(), searchParams)

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
			utils.Log("No more job results found")
			break
		}

		allJobs = append(allJobs, jobs...)

		// Check if we've hit our maximum
		if s.jobLimit > 0 && len(allJobs) >= s.jobLimit {
			utils.Log("Reached job limit (%d), stopping search", s.jobLimit)
			if len(allJobs) > s.jobLimit {
				allJobs = allJobs[:s.jobLimit]
			}
			break
		}

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
