package pipeline

import (
	"context"
	"fmt"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

// Step represents a single processing step in the pipeline
type Step interface {
	Name() string
	Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error)
}

// Pipeline manages the processing steps
type Pipeline struct {
	steps []Step
}

func NewPipeline(steps []Step) *Pipeline {
	return &Pipeline{
		steps: steps,
	}
}

// Run executes all pipeline steps in order
func (p *Pipeline) Run(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	var err error
	currentListings := listings

	for _, step := range p.steps {
		utils.Log("Running step: %s", step.Name())
		currentListings, err = step.Process(ctx, currentListings)
		if err != nil {
			return nil, fmt.Errorf("error in %s step: %w", step.Name(), err)
		}
		utils.Log("Finished step: %s", step.Name())
	}

	logs := utils.GetLogs()

	// Only include important logs (logs marked as ERROR, CUSTOM, or FATAL)
	var importantLogs []utils.LogEntry
	for _, entry := range logs {
		if entry.Level == "ERROR" || entry.Level == "CUSTOM" || entry.Level == "FATAL" {
			importantLogs = append(importantLogs, entry)
		}
	}

	// Send logs as embeds
	if len(importantLogs) > 0 {
		if err := utils.SendScraperLogsEmbed(importantLogs); err != nil {
			utils.Warn("Failed to send logs to Discord: %v", err)
		}
	}

	// Clear logs if needed
	utils.ClearLogs()
	return currentListings, nil
}
