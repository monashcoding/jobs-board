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

### Scraper Tests
Each scraper is tested using a shared testing framework.

For each site, we first save a copy of the HTML data, and calculate the expected `Listing[]` response from it.
   ```json
   // success/expected.json
   [
     {
       "title": "Graduate Software Engineer",
       "company": {
         "name": "Example Corp",
         "website": "https://example.com"
       },
       "type": "GRADUATE",
       "locations": ["VIC"],
       "study_fields": ["SOFTWARE"]
     }
   ]
   ```
Then we run the test, simulating HTTP requests to each URL and comparing the expected outputs.
```go
func TestYourScraper_Scrape(t *testing.T) {
scraper := NewYourScraper()
suite := scrapertesting.NewScraperTestSuite(t, scraper, "testdata/your_scraper")

         testCases := []scrapertesting.TestCase{
             {
                 Name:          "successful_scrape",
                 FixtureFolder: "success",
                 ExpectedFile:  "success/expected.json",
                 FixtureMapping: map[string]string{
                     "https://example.com/jobs": "success/main_page.html",
                 },
             },
             // Add more test cases...
         }
    
         for _, tc := range testCases {
             suite.RunTestCase(tc)
         }
     }
```

HTML data should be up to date and reflect the actual structure of each site.

#### Test Structure
```
internal/
└── scraper/
    ├── testing/                        # Shared testing utilities
    └── sites/
        ├── testdata/     
        │   ├── gradcon/                # Gradcon Specific Test Data
        │   │   ├── success/              # Successful scraping scenarios
        │   │   │   ├── page.html           # Homepage HTML Response
        │   │   │   └── expected.json       # Expected parsing results
        ├── gradcon.go                  # GradConnection scraper
        ├── gradcon_test.go             # GradConnection scraper tests
        └── ...
```

### Pipeline Tests
Each pipeline step is tested using a shared testing framework.

We define the input and expected output `Listing[]` in JSON.
   ```json
   // scenario1/input.json
   [
     {
       "title": "Example Job",
       ...
     }
   ]
   
   // scenario1/output.json
   [
     {
       "title": "Processed Example Job",
       ...
     }
   ]
   ```

Finally, we run the pipeline step against our test case.
   ```go
   func TestYourStep_Process(t *testing.T) {
       step := NewYourStep()
       suite := pipelinetesting.NewPipelineStepTestSuite(t, step, "testdata/your_step")

       testCases := []pipelinetesting.TestCase{
           {
               Name:       "scenario1",
               InputFile:  "scenario1/input.json",
               OutputFile: "scenario1/output.json",
           },
           // Add more test cases...
       }

       for _, tc := range testCases {
           suite.RunTestCase(tc)
       }
   }
   ```

#### Test Structure
```
internal/
└── pipeline/
    ├── testing/                # Shared testing utilities
    └── steps/
        ├── testdata/          # Test fixtures
        │   ├── normalization/
        │   │   ├── clean/     # Testing with already clean data
        │   │   │   ├── input.json
        │   │   │   └── output.json
        ├── normalization.go
        ├── normalization_test.go
        ├── (... repeated for other steps)
```

## Getting Started

1. Set up environment variables:
```env
MONGODB_URI=mongodb://localhost:27017
MONGODB_DATABASE=mploy
MONGODB_COLLECTION=jobs
```

2. Run the scraper:
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
