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

	for i := range listings {
		// Pass pointer to the listing
		SyncCompanyLogoUrl(&listings[i], listings)
	}
	return listings, nil
}

func SyncCompanyLogoUrl(listing *models.Listing, allListings []models.Listing) {
	// More defensive nil checking
	if listing == nil || listing.Company.Logo != nil && *listing.Company.Logo != "" {
		return
	}

	for _, l := range allListings {
		if l.Company.Name == listing.Company.Name &&
			l.Company.Logo != nil &&
			*l.Company.Logo != "" {
			utils.Log("Synchronising company logo for %s", listing.Company.Name)
			listing.Company.Logo = l.Company.Logo
			break
		}
	}
}
