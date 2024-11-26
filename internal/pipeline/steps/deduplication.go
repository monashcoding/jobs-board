package steps

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"
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
	unique := make([]models.Listing, 0, len(listings))

	// process and insert listings. objects with existing IDs will be overwritten

	return unique, nil
}
