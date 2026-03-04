// Package config provides utilities for working with environment-based configuration
package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

// EnvVar represents an environment variable and its validation
type EnvVar struct {
	Name     string
	Required bool
	Default  string
	Validate func(string) bool
}

// Environment variables used in the application
var EnvironmentVars = []EnvVar{
	// MongoDB configuration
	{Name: "MONGODB_URI", Required: true, Default: "mongodb://localhost:27017"},
	{Name: "MONGODB_DATABASE", Required: true, Default: "mploy"},
	{Name: "MONGODB_COLLECTION", Required: true, Default: "jobs"},

	// BrightData configuration
	{Name: "BRIGHTDATA_CUSTOMER_ID", Required: true},
	{Name: "BRIGHTDATA_ZONE_NAME", Required: true},
	{Name: "BRIGHTDATA_PASSWORD", Required: true},
	{Name: "BRIGHTDATA_API_TOKEN", Required: true},

	// Ollama configuration
	{Name: "OLLAMA_ENDPOINT", Required: true},

	// Discord webhook
	{Name: "DISCORD_WEBHOOK_URL", Required: true},

	// Version
	{Name: "VERSION", Required: true, Default: "1.0.0"},

	// Scraper configuration
	{Name: "RUN_SCRAPERS", Required: false},
	{Name: "RUN_PIPELINE_STEPS", Required: false},
	{Name: "SCRAPER_JOB_LIMITS", Required: false},
}

// ValidateEnvironment checks required environment variables and sets defaults
func ValidateEnvironment() []string {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		utils.Fatal(".env file not found: %v\n.", err)
	}

	var missing []string

	for _, envVar := range EnvironmentVars {
		value := os.Getenv(envVar.Name)

		// If required and empty, add to missing list
		if envVar.Required && value == "" {
			// Check if default exists
			if envVar.Default != "" {
				os.Setenv(envVar.Name, envVar.Default)
			} else {
				missing = append(missing, envVar.Name)
			}
		}

		// Run validation if provided
		if envVar.Validate != nil && value != "" {
			if !envVar.Validate(value) {
				missing = append(missing, envVar.Name+" (invalid value)")
			}
		}
	}

	return missing
}

// GetScrapers returns the list of scrapers to run based on environment config
func GetScrapers() []string {
	scrapers := os.Getenv("RUN_SCRAPERS")
	if scrapers == "" {
		// Return empty slice to indicate all scrapers should run
		return []string{}
	}

	return strings.Split(scrapers, ",")
}

// GetPipelineSteps returns the list of pipeline steps to run
func GetPipelineSteps() []string {
	steps := os.Getenv("RUN_PIPELINE_STEPS")
	if steps == "" {
		// Return empty slice to indicate all steps should run
		return []string{}
	}

	return strings.Split(steps, ",")
}

// ShouldRunScraper determines if a scraper should be run based on configuration
func ShouldRunScraper(scraperName string) bool {
	scrapers := GetScrapers()
	if len(scrapers) == 0 {
		// No specific scrapers specified, run all
		return true
	}

	for _, s := range scrapers {
		if strings.TrimSpace(s) == scraperName {
			return true
		}
	}

	return false
}

// ShouldRunPipelineStep determines if a pipeline step should be run
func ShouldRunPipelineStep(stepName string) bool {
	steps := GetPipelineSteps()
	if len(steps) == 0 {
		// No specific steps specified, run all
		return true
	}

	// Convert the Name() method output to the environment variable format
	// For example: "Normalisation" from step.Name() should match "Normalisation" in config
	for _, s := range steps {
		if strings.TrimSpace(s) == stepName {
			return true
		}
	}

	return false
}

// GetScraperJobLimit returns the job limit for a specific scraper
// Returns -1 if no limit is defined
func GetScraperJobLimit(scraperName string) int {
	limitConfig := os.Getenv("SCRAPER_JOB_LIMITS")
	if limitConfig == "" {
		return -1
	}

	limits := strings.Split(limitConfig, ",")
	for _, limit := range limits {
		parts := strings.Split(limit, ":")
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), scraperName) {
			limitValue, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err == nil && limitValue > 0 {
				return limitValue
			}
		}
	}

	return -1
}
