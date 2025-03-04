package steps

import (
	"context"
	"fmt"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
	"sort"
	"strings"
)

type DeduplicationStep struct {
	seenHashes map[string]bool
}

func NewDeduplicationStep() *DeduplicationStep {
	return &DeduplicationStep{
		seenHashes: make(map[string]bool),
	}
}

func (s *DeduplicationStep) Name() string {
	return "Deduplication"
}

func (s *DeduplicationStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	/*------------------------------------------------*/
	/* Deduplication Strategy (in order):             */
	/* 1. Source Override - Only keep listings from   */
	/*    the preferred source for specific companies */
	/* 2. Application URL - Remove listings with      */
	/*    duplicate application URLs (with exceptions)*/
	/* 3. Title/Company/Type/Location - Remove        */
	/*    listings that match on all these fields     */
	/*------------------------------------------------*/

	if len(listings) == 0 {
		return listings, nil
	}

	var result []models.Listing
	sourceOverrideMap := utils.GetSourceOverrideMap()
	sameURLCompany := utils.GetSameURLCompanyMap()

	// Step 1: Filter by source override
	sourceFiltered := make([]models.Listing, 0, len(listings))
	for _, listing := range listings {
		preferredSource, hasPreference := sourceOverrideMap[listing.Company.Name]
		if hasPreference && listing.Source != preferredSource {
			utils.Custom("[Source Override] Skipping %s | %s from %s", listing.Company.Name, listing.Title, listing.Source)
			continue
		}
		sourceFiltered = append(sourceFiltered, listing)
	}

	// Step 2: Filter by application URL
	seenURLs := make(map[string]bool)
	urlFiltered := make([]models.Listing, 0, len(sourceFiltered))

	for _, listing := range sourceFiltered {
		// Skip URL check for special companies
		if sameURLCompany[listing.Company.Name] {
			urlFiltered = append(urlFiltered, listing)
			continue
		}

		// Check if URL already seen
		if listing.ApplicationURL != nil {
			url := *listing.ApplicationURL
			if seenURLs[url] {
				utils.Custom("[Apply URL Duplicate] Skipping %s | %s from %s", listing.Company.Name, listing.Title, listing.Source)
				continue
			}
			seenURLs[url] = true
		}

		urlFiltered = append(urlFiltered, listing)
	}

	// Step 3: Filter by title, company, type, and location
	seenListings := make(map[string]bool)

	for _, listing := range urlFiltered {
		// Create unique key from title, company, type, and locations
		var typeStr string
		if listing.Type != nil {
			typeStr = string(*listing.Type)
		}

		// Convert locations to strings and sort them for consistent hashing
		locStrs := make([]string, len(listing.Locations))
		for i, loc := range listing.Locations {
			locStrs[i] = string(loc)
		}
		sort.Strings(locStrs)

		// Create hash
		hash := fmt.Sprintf("%s|%s|%s|%s",
			listing.Title,
			listing.Company.Name,
			typeStr,
			strings.Join(locStrs, ","))

		if seenListings[hash] {
			utils.Custom("[Hash Exact Match] Skipping %s | %s from %s", listing.Company.Name, listing.Title, listing.Source)
			continue
		}

		seenListings[hash] = true
		result = append(result, listing)
	}

	utils.Custom("Deduplication: %d → %d listings", len(listings), len(result))

	return result, nil
}
