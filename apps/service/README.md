# MPLOY Backend Scraper

This is the backend scraping service for MPLOY, a job board aggregator focused on student opportunities. The service scrapes various job sites, processes the listings, and stores them in MongoDB.

## Project Structure

```
.
├── cmd/
│   └── scraper/
│       └── main.go           # Entry point for the scraper service
├── internal/
│   ├── database/
│   │   └── mongodb.go        # MongoDB connection and configuration
│   ├── models/
│   │   └── listing.go        # Core data models for job listings
│   ├── pipeline/
│   │   ├── pipeline.go       # Processing pipeline definition
│   │   └── steps/           # Individual processing steps
│   │       ├── deduplication.go  # Removes duplicate listings
│   │       ├── normalization.go  # Standardizes listing data
│   │       └── upload.go         # Handles MongoDB storage
│   └── scraper/
│       ├── scraper.go        # Base scraper interface and utilities
│       └── sites/            # Site-specific scrapers
│           ├── gradcon.go    # GradConnection scraper
│           ├── linkedin.go   # LinkedIn jobs scraper
│           └── prosple.go    # Prosple scraper
├── go.mod                    # Go module definition
└── go.sum                    # Go module checksums
```

## Key Components

### Scraper (`internal/scraper/`)

- `scraper.go`: Defines the base scraper interface and common utilities
- `sites/`: Contains implementations for each job site
  - Each scraper is responsible for parsing its specific site's HTML structure
  - Uses [Colly](https://github.com/gocolly/colly) for web scraping

### Processing Pipeline (`internal/pipeline/`)

The pipeline processes scraped listings through several steps:

1. **Normalization**: Standardizes data formats and cleans text
2. **Deduplication**: Removes duplicate listings across sources
3. **Upload**: Stores processed listings in MongoDB

A pipeline accepts an array of listings, and will return an array of listings.
The final pipeline `upload.go` will upload / update the listings passed to it.

### Unblocker (`internal/unblocker`)

The Unblocker service is an anti-detection and request management system.

1. **BrightData Proxy Integration**: Uses a proxy service to automatically rotate through IP addresses

```go
type BrightDataConfig struct {
    CustomerID  string   // Your BrightData customer ID (e.g. hl_29d8sj2d)
    ZoneName    string   // Your BrightData zone name (e.g. datacenter_proxy1)
    Password    string   // Your BrightData password
    Country     string   // Optional: Target country (e.g., "us", "uk")
    State       string   // Optional: US state code (only when Country="us")
    City        string   // Optional: Target city
    UseLocalDNS bool     // Whether to use local DNS resolution
}
```

2. **Browser Profile Management**: Consistent and realistic browser fingerprinting

```go
type BrowserConfig struct {
    Profiles          []BrowserProfile  // List of browser profiles to rotate
    RotationStrategy  string           // Strategy for profile rotation
    RotationInterval  time.Duration    // How often to rotate profiles
}
```

3. **Rate Limiting**: Configurable request pacing and burst control

```go
type RateLimitConfig struct {
    RequestsPerMinute int           // Maximum requests per minute
    BurstSize        int           // Maximum burst size for requests
    BaseDelay        time.Duration // Minimum delay between requests
    MaxJitter        time.Duration // Random delay addition (0 to MaxJitter)
}
```

4. **Cookie Management**: Domain-specific cookie handling

```go
type CookieConfig struct {
    PersistCookies bool                    // Whether to maintain cookies between sessions
    DomainRules    map[string]CookieRule   // Domain-specific cookie handling rules
}
```

5. **Header Management**: Request header customization and rotation, including setting fake referral sites

```go
type HeaderConfig struct {
    ReferralSites []string            // List of referrer URLs to rotate
    CustomHeaders map[string]string   // Custom headers to include
}
```

## Testing

### Pipeline Tests

Each pipeline step is tested using a shared testing framework.

We define the input and expected output `Listing[]` in JSON. You can do this by exporting existing listings in the MongoDB.

Note: Time is exported as a string in json files and doesn't import well with the native JSON.Unmarshall function. Therefore for testing purposes any time parameters should be removed from the extract.

```json
// /test_utils/test_listings.json
[
  {
    "_id": {
      "$oid": "6781bcaf108e58edc395b422"
    },
    "title": "2025 EY Graduate Computer Science Program",
    "description": "Join us full-time as a computer science graduate and let's shape your bright future together. We're committed to nurturing your career with continuous support, learning opportunities, and dynamic experiences.",
    "company": {
      "name": "EY",
      "website": "https://au.gradconnection.com/employers/ey"
    },
    "application_url": "https://www.ey.com/en_au/careers/students-apply-here",
    "wfh_status": null,
    "source_urls": [
      "https://au.gradconnection.com/employers/ey/jobs/ey-2025-ey-graduate-computer-science-program-4"
    ],
    "type": "GRADUATE",
    "locations": ["ACT", "NSW", "VIC", "WA"],
    "study_fields": ["Computer Science"],
    "working_rights": ["AUS_CITIZEN", "NZ_CITIZEN", "AUS_PR"],
    // "created_at": DON'T EXPORT
    // "updated_at": DON'T EXPORT
    // "close_date": DON'T EXPORT
  },
  {
    ...
  }
]
```

Finally, we run the pipeline step against our test case.

```go
func TestNormalisation(t *testing.T) {
	testListings := test_utils.LoadTestListings() // Load test listings
	norm := steps.NewDeduplicationStep(); // Init pipeline step

	res, err := norm.Process(context.Background(), testListings) // Run pipeline step

	if err != nil {
		t.Errorf("Error in : %v", err)
	}
}
```

```bash
go test ./... -v
```

#### Test Structure

```
internal/
└── pipeline/
    ├── steps/                # Pipeline Functions
        ├── normalization.go
    └── testing/              # Test Files
        ├── normalization_test.go
```

## Getting Started

1. Set up environment variables:

```env
# Database configuration
MONGODB_URI=mongodb://localhost:27017
MONGODB_DATABASE=mploy
MONGODB_COLLECTION=jobs

# Scraping configuration
ENVIRONMENT=DEVELOPMENT # PRODUCTION (Scraper runs at the time specified below each day) | DEVELOPMENT (Scraper runs instantly)
```

2. Set up config file:

Go to the root directory and edit the `config.go` file

```go
func GetConfig() *Configuration {
	return &Configuration{
		// Versioning
		Version: "v1.0.0 beta",

		// Scrapers
		RunGradConnection: true, // Scrape GradConnection?
		RunProsple: true, // Scrape Prosple?
		RunLinkedIn: true, // Scrape LinkedIn?

		// Pipeline
		RunWriteJson: true, // Run Write JSON Step?
		RunNormalisation: true, // Run Normalisation Step?
		RunDeduplication: true, // Run Deduplication Step?
		RunSynchronisation: true, // Run Synchronisation Step?
		RunAiParser: true, // Run AI Parser Step?
		RunUpload: true, // Run Upload Step?

		// Grad Connection Settings
		GradConnectionScrapeAll: true, // Scrape all pages on GradConnect?
		GradConnectionPageSize: 25, // Number of listings per page
		GradConnectionMaxPages: 2, // Maximum number of pages to scrape. Ignore if GradConnectionScrapeAll is true

		// Prosple Settings
		ProspleScrapeAll: true, // Scrape all pages on Prosple?
		ProsplePageSize: 50, // Number of listings per page
		ProspleMaxPages: 2, // Maximum number of pages to scrape. Ignore if ProspleScrapeAll is true

		// LinkedIn Settings
		LinkedInMaxListings: 100, // Maximum number of listings to scrape on LinkedIn
	}
}
```

3. Run the scraper:

```bash
go run cmd/scraper/main.go
```

## Adding a New Pipeline Step

1. Create a new file in `internal/pipeline/steps/`
2. Implement the `Step` interface:

```go
type Step interface {
    Name() string
    Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error)
}
```

3. Add the step to the pipeline in `cmd/scraper/main.go`

## Development Notes

- The scraper uses concurrent processing where possible
- Each step in the pipeline can modify the listings array
- Scrapers should implement rate limiting and respect robots.txt
- All site-specific logic should be contained within the respective scraper file
