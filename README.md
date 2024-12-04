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

## Testing

### Scraper Tests
Each scraper is tested using a shared testing framework.

For each site, we run the scrapers against saved HTML data, ensuring they return the expected `Listing[]` response.

HTML data should be up to date and reflect the actual structure of each site.

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

#### Test Structure
Each scraper's tests follow these conventions:
1. **Test Data**: HTML fixtures and expected results are stored in `internal/scraper/sites/testdata/<scraper_name>/`
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
2. **URL Mapping**: Different scenarios are tested, and each test maps URLs to local HTML fixtures
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
#### Running Tests
To test a specific scraper:
```bash
go test ./internal/scraper/sites -run TestGradconScraper_Scrape -v
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
