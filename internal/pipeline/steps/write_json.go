// File: internal/pipeline/steps/write_json.go

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type WriteJSONStep struct {
	debugDir string
}

func NewWriteJSONStep() *WriteJSONStep {
	return &WriteJSONStep{
		debugDir: "debug/write-json",
	}
}

func (s *WriteJSONStep) Name() string {
	return "WriteJSON"
}

func (s *WriteJSONStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	if len(listings) == 0 {
		utils.Log("No listings to write to JSON file")
		return listings, nil
	}

	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(s.debugDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create debug directory: %w", err)
	}

	// Create filename with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := filepath.Join(s.debugDir, fmt.Sprintf("listings_%s.json", timestamp))

	// Pretty print JSON
	jsonData, err := json.MarshalIndent(listings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal listings: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write JSON file: %w", err)
	}

	utils.Success("Wrote %d listings to %s", len(listings), filename)

	return listings, nil
}
