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

// jobTypesCanDiffer reports whether two listings are distinguishable by job
// type. A nil type is unknown rather than different, so it matches anything:
// treating it as its own value is what let duplicates through.
func jobTypesCanDiffer(a, b *models.JobType) bool {
	if a == nil || b == nil {
		return false
	}
	return *a != *b
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

	// Step 3: Filter by title, company, and location, treating an unknown job
	// type as matching any type.
	//
	// Job type used to be part of the key outright, which quietly stopped
	// working: LinkedIn is the only scraper that does not set it
	// (parser.go: "Type: nil, // TODO"), and this step runs before the AI parser
	// fills it in. The same job scraped from both LinkedIn and GradConnection
	// therefore hashed as ""|... and "INTERN"|..., survived as two listings, and
	// only looked identical later once the parser had filled the gap. That was
	// 18 of 98 board-eligible listings, and TikTok alone carried 18 duplicate
	// pairs.
	//
	// Two genuinely different roles that share a title and location but differ
	// in type are still kept apart, so an intern and a graduate opening of the
	// same name do not collapse into one.
	seenListings := make(map[string][]int)

	for _, listing := range urlFiltered {
		// Convert locations to strings and sort them for consistent hashing
		locStrs := make([]string, len(listing.Locations))
		for i, loc := range listing.Locations {
			locStrs[i] = string(loc)
		}
		sort.Strings(locStrs)

		hash := fmt.Sprintf("%s|%s|%s",
			listing.Title,
			listing.Company.Name,
			strings.Join(locStrs, ","))

		duplicate := false
		for _, kept := range seenListings[hash] {
			if !jobTypesCanDiffer(result[kept].Type, listing.Type) {
				utils.Custom("[Hash Exact Match] Skipping %s | %s from %s", listing.Company.Name, listing.Title, listing.Source)

				// Keep whichever copy knows its job type. Sources disagree about
				// which fields they populate, and the surviving listing carries
				// the type into the fingerprint and the board gates.
				if result[kept].Type == nil && listing.Type != nil {
					result[kept] = listing
				}
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}

		seenListings[hash] = append(seenListings[hash], len(result))
		result = append(result, listing)
	}

	utils.Custom("Deduplication: %d → %d listings", len(listings), len(result))

	return result, nil
}
