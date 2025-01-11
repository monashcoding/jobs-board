package test_utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

// LoadTestListings loads listings from a JSON file
func LoadTestListings() []models.Listing {
	// Get the directory of the current file
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		utils.Fatal("Failed to get current file path")
	}

	// Get the directory containing the current file
	currentDir := filepath.Dir(currentFile)

	// Construct path relative to the current file's directory
	path := filepath.Join(currentDir, "test_listings.json")

	data, err := os.ReadFile(path)
	if err != nil {
		utils.Fatal("Failed to read listings file %s: %v", path, err)
	}

	var listings []models.Listing
	if err := json.Unmarshal(data, &listings); err != nil {
		utils.Fatal("Failed to unmarshal listings from %s: %v", path, err)
	}

	return listings
}
