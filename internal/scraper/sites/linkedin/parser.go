package linkedin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type Job struct {
	ID          string
	Title       string
	Company     string
	CompanyURL  string
	CompanyLogo string
	Location    string
	Description string
	JobType     string
	PostedDate  time.Time
	ApplyURL    string
}

func parseSearchResults(resp *http.Response) ([]Job, error) {
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var jobs []Job

	doc.Find("div.base-search-card").Each(func(i int, s *goquery.Selection) {
		job := Job{}

		// Extract job ID from the URL
		if href, exists := s.Find("a.base-card__full-link").Attr("href"); exists {
			job.ID = extractJobID(href)
		}

		// Extract job title
		job.Title = s.Find("h3.base-search-card__title").Text()

		// Extract company name and URL
		companyLink := s.Find("h4.base-search-card__subtitle a")
		job.Company = companyLink.Text()
		if href, exists := companyLink.Attr("href"); exists {
			job.CompanyURL = href
		}

		// Extract company logo (data-delayed-url is the lazy-loaded real URL;
		// static.licdn.com URLs are placeholder ghosts, not real logos)
		if logoURL, exists := s.Find("div.search-entity-media img").Attr("data-delayed-url"); exists {
			if !strings.Contains(logoURL, "static.licdn.com") {
				job.CompanyLogo = logoURL
			}
		}

		// Extract location
		job.Location = s.Find("span.job-search-card__location").Text()

		jobs = append(jobs, job)
	})

	return jobs, nil
}

func (s *LinkedInScraper) processJob(ctx context.Context, job Job) (models.Listing, error) {
	jobURL := fmt.Sprintf("%s/jobs/view/%s", s.URL(), job.ID)

	req, err := http.NewRequest("GET", jobURL, nil)
	if err != nil {
		return models.Listing{}, err
	}

	resp, err := s.unblocker.MakeRequest(ctx, req)
	if err != nil {
		return models.Listing{}, err
	}
	defer resp.Body.Close()

	// Save job details response
	if err := s.saveResponse(resp, fmt.Sprintf("job_%s", job.ID)); err != nil {
		utils.Error("Failed to save job response: %v", err)
	}

	// Check if we got redirected to signup page
	if strings.Contains(resp.Request.URL.Path, "/signup") {
		return models.Listing{}, fmt.Errorf("redirected to signup page - possible detection")
	}

	// Parse job details
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return models.Listing{}, err
	}

	// Extract job description
	description := doc.Find("div.show-more-less-html__markup").Text()

	// Check for external apply URL first
	if externalLink := doc.Find("a.sign-up-modal__company_webiste"); externalLink.Length() > 0 {
		if href, exists := externalLink.Attr("href"); exists {
			// Parse the URL to extract the application URL from query params
			if parsedURL, err := url.Parse(href); err == nil {
				if encodedURL := parsedURL.Query().Get("url"); encodedURL != "" {
					// URL decode the application URL
					if decodedURL, err := url.QueryUnescape(encodedURL); err == nil {
						job.ApplyURL = decodedURL
					}
				}
			}
		}
	}

	// If application url is "" just use source url
	if job.ApplyURL == "" {
		job.ApplyURL = jobURL
	}

	// Convert to our job model
	listing := models.Listing{
		Title:       job.Title,
		Description: &description,
		Company: models.Company{
			Name:    job.Company,
			Website: &job.CompanyURL,
			Logo:    logoPtr(job.CompanyLogo),
		},
		ApplicationURL: &job.ApplyURL,
		SourceURLs:     []string{jobURL},
		Type:           nil, // TODO: Job type
		Locations:      parseLocations(job.Location),
		WFHStatus:      nil, // TODO: WFH Status
		StudyFields:    nil,
		WorkingRights:  nil,
		Source:         models.LINKEDIN,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Days_lived:     0,
	}

	return listing, nil
}

func logoPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
