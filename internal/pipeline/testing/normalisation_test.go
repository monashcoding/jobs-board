package testing

import (
	"context"
	"testing"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
	test_utils "github.com/monashcoding/mploy-golang-core/internal/testing"
)

func TestNormalisationRuns(t *testing.T) {
	testListings := test_utils.LoadTestListings()
	norm := steps.NewNormalisationStep()

	_, err := norm.Process(context.Background(), testListings)

	if err != nil {
		t.Fatalf("Normalisation Process failed on test listings : %v", err)
	}
}

func TestNormalisationTrimsTitle(t *testing.T) {
	emptyNameListing := models.Listing{
		Title: "Trim    ",
	}

	norm := steps.NewNormalisationStep()
	res, _ := norm.Process(context.Background(), []models.Listing{emptyNameListing})

	if len(res) != 1 {
		t.Fatalf("Expected 1 listing, got %d", len(res))
	}

	testListing := res[0]

	if testListing.Title != "Trim" {
		t.Fatalf("Expected 'Trim', got '%s'", testListing.Title)
	}
}


func TestNormalisationRenamesCompanyName(t *testing.T) {
	wrongListing := models.Listing{
		Company: models.Company{
			Name: "Amazon AU",
		},
	}

	correctListing, err := steps.StandardiseCompanyName(wrongListing)

	if err != nil {
		t.Fatalf("StandardiseCompanyName failed on test listing")
	}

	if correctListing.Company.Name != "Amazon" {
		t.Fatalf("Expected 'Amazon', got '%s'", correctListing.Company.Name)
	}
}
