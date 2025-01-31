package config

type Configuration struct {
	Version string

	RunGradConnection bool
	RunProsple        bool
	RunLinkedIn       bool

	RunWriteJson       bool
	RunNormalisation   bool
	RunDeduplication   bool
	RunSynchronisation bool
	RunAiParser        bool
	RunUpload          bool

	GradConnectionScrapeAll bool
	GradConnectionPageSize  int
	GradConnectionMaxPages  int

	ProspleScrapeAll bool
	ProsplePageSize  int
	ProspleMaxPages  int

	LinkedInMaxListings int
}

func GetConfig() *Configuration {
	return &Configuration{
		// Versioning
		Version: "v1.0.0 beta",

		// Scrapers
		RunGradConnection: true, // Scrape GradConnection?
		RunProsple:        true, // Scrape Prosple?
		RunLinkedIn:       true, // Scrape LinkedIn?

		// Pipeline
		RunWriteJson:       true, // Run Write JSON Step?
		RunNormalisation:   true, // Run Normalisation Step?
		RunDeduplication:   true, // Run Deduplication Step?
		RunSynchronisation: true, // Run Synchronisation Step?
		RunAiParser:        true, // Run AI Parser Step?
		RunUpload:          true, // Run Upload Step?

		// Grad Connection Settings
		GradConnectionScrapeAll: true, // Scrape all pages on GradConnect?
		GradConnectionPageSize:  25,   // Number of listings per page
		GradConnectionMaxPages:  2,    // Maximum number of pages to scrape. Ignore if GradConnectionScrapeAll is true

		// Prosple Settings
		ProspleScrapeAll: true, // Scrape all pages on Prosple?
		ProsplePageSize:  50,   // Number of listings per page
		ProspleMaxPages:  2,    // Maximum number of pages to scrape. Ignore if ProspleScrapeAll is true

		// LinkedIn Settings
		LinkedInMaxListings: 100, // Maximum number of listings to scrape on LinkedIn
	}
}
