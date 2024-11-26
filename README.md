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
