package steps

import (
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"
)

type NormalizationStep struct{}

func NewNormalizationStep() *NormalizationStep {
	return &NormalizationStep{}
}

func (s *NormalizationStep) Name() string {
	return "Normalization"
}

func (s *NormalizationStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	normalized := make([]models.Listing, len(listings))

	// process and insert listings

	return normalized, nil
}
