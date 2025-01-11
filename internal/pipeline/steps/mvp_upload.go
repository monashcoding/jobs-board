package steps

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type MVPUploadStep struct {
	mongodb *database.MongoDB
	logs    []string
}

func NewMVPUploadStep(mongodb *database.MongoDB) *MVPUploadStep {
	return &MVPUploadStep{
		mongodb: mongodb,
		logs:    make([]string, 0),
	}
}

func (s *MVPUploadStep) Name() string {
	return "MVPUpload"
}

// stripAnsiCodes removes ANSI color codes from the string
func stripAnsiCodes(str string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(str, "")
}

// AddLog allows other parts of the application to add logs
func (s *MVPUploadStep) AddLog(log string) {
	// Clean the log before storing
	cleanLog := stripAnsiCodes(log)
	s.logs = append(s.logs, cleanLog)
}

func (s *MVPUploadStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	if len(listings) == 0 {
		return listings, nil
	}

	// Create a timestamp-based collection name
	timestamp := time.Now().Format("2006_01_02_15_04_05")
	collectionName := fmt.Sprintf("jobs_%s", timestamp)

	// Create new collection for this run
	collection := s.mongodb.GetDatabase().Collection(collectionName)

	// Create documents array
	docs := make([]interface{}, len(listings))
	for i, listing := range listings {
		docs[i] = listing
	}

	// Insert all listings
	_, err := collection.InsertMany(ctx, docs)
	if err != nil {
		return nil, fmt.Errorf("failed to insert listings: %w", err)
	}

	// Create a metadata document containing logs and timestamp
	metadataCollection := s.mongodb.GetDatabase().Collection(fmt.Sprintf("metadata_%s", timestamp))
	metadata := map[string]interface{}{
		"timestamp":      time.Now(),
		"logs":           s.logs,
		"total_listings": len(listings),
		"run_date":       time.Now().Format("2006-01-02"),
		"run_time":       time.Now().Format("15:04:05"),
	}

	_, err = metadataCollection.InsertOne(ctx, metadata)
	if err != nil {
		utils.Error("Failed to insert metadata: %v", err)
	}

	utils.Success("Successfully inserted %d listings into collection %s", len(listings), collectionName)
	return listings, nil
}
