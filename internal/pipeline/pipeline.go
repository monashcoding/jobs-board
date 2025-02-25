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
		utils.Log("%s", "Running step: "+step.Name())
		currentListings, err = step.Process(ctx, currentListings)
		if err != nil {
			return nil, fmt.Errorf("error in %s step: %w", step.Name(), err)
		}
		utils.Log("%s", "Finished step: "+step.Name())
	}
	logs := utils.GetLogs()
	for _, entry := range logs {
		fmt.Printf("Time: %s, Level: %s, Message: %s\n",
			entry.Timestamp,
			entry.Level,
			entry.Message)
	}

	// Clear logs if needed
	utils.ClearLogs()
	return currentListings, nil
}
