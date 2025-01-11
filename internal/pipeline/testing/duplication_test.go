package testing

import (
	"context"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
	test_utils "github.com/monashcoding/mploy-golang-core/internal/testing"
)

func TestDeduplication(t *testing.T) {
	testListings := test_utils.LoadTestListings()
	norm := steps.NewDeduplicationStep()

	_, err := norm.Process(context.Background(), testListings)

	if err != nil {
		t.Fatalf("Normalisation Process failed on test listings : %v", err)
	}
}
