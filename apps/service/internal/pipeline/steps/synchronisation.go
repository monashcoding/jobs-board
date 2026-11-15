package steps

import (
	"context"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type SynchronisationStep struct {
	seenHashes map[string]bool
}

func NewSynchronisationStep() *SynchronisationStep {
	return &SynchronisationStep{
		seenHashes: make(map[string]bool),
	}
}

func (s *SynchronisationStep) Name() string {
	return "Synchronisation"
}

func (s *SynchronisationStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	/*
		Synchronisation Goals:
			1. Listings from LinkedIn don't have company logo URLS. Synchronise with company logos from listings from other sources.

	*/

	logos := buildCompanyLogoIndex(listings)

	for i := range listings {
		// Pass pointer to the listing
		SyncCompanyLogoUrl(&listings[i], logos)
	}
	return listings, nil
}

// buildCompanyLogoIndex maps each company to the first non-empty logo seen for
// it. Resolving the logo by rescanning every listing per listing was quadratic:
// a full scrape is a few thousand listings, so it cost millions of string
// comparisons to answer a question with one distinct answer per company.
func buildCompanyLogoIndex(listings []models.Listing) map[string]*string {
	logos := make(map[string]*string)

	for i := range listings {
		company := listings[i].Company
		if company.Logo == nil || *company.Logo == "" {
			continue
		}
		if _, seen := logos[company.Name]; !seen {
			logos[company.Name] = company.Logo
		}
	}

	return logos
}

// SyncCompanyLogoUrl fills in a missing company logo from another listing for
// the same company. LinkedIn listings arrive without logos, so they borrow one
// from whichever source did supply it.
func SyncCompanyLogoUrl(listing *models.Listing, logos map[string]*string) {
	// More defensive nil checking
	if listing == nil || listing.Company.Logo != nil && *listing.Company.Logo != "" {
		return
	}

	if logo, ok := logos[listing.Company.Name]; ok {
		utils.Log("Synchronising company logo for %s", listing.Company.Name)
		listing.Company.Logo = logo
	}
}
