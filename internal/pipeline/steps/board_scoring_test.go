package steps

import (
	"context"
	"testing"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

func boardListing(mutate func(*models.Listing)) models.Listing {
	jobType := models.GRADUATE
	applicationURL := "https://careers.atlassian.com/jobs/123"

	listing := models.Listing{
		Title:          "Graduate Software Engineer",
		Company:        models.Company{Name: "Atlassian"},
		ApplicationURL: &applicationURL,
		Type:           &jobType,
		Locations:      []models.LocationType{models.VIC},
		Discipline:     models.SOFTWARE,
	}

	if mutate != nil {
		mutate(&listing)
	}
	return listing
}

func TestEvaluateForBoardGates(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*models.Listing)
		eligible bool
	}{
		{"a technical australian grad role qualifies", nil, true},
		{"intern also qualifies", func(l *models.Listing) {
			t := models.INTERN
			l.Type = &t
		}, true},
		{"data qualifies", func(l *models.Listing) { l.Discipline = models.DATA }, true},
		{"cyber qualifies", func(l *models.Listing) { l.Discipline = models.CYBER }, true},

		{"non-software engineering is excluded", func(l *models.Listing) { l.Discipline = models.ENGINEERING }, false},
		{"non-technical is excluded", func(l *models.Listing) { l.Discipline = models.OTHER_DISCIPLINE }, false},
		{"unclassified is excluded", func(l *models.Listing) { l.Discipline = "" }, false},
		{"overseas is excluded", func(l *models.Listing) {
			l.Locations = []models.LocationType{models.OTHERS}
		}, false},
		{"no location is excluded", func(l *models.Listing) { l.Locations = nil }, false},
		{"untyped role is excluded", func(l *models.Listing) { l.Type = nil }, false},
		{"OTHER job type is excluded", func(l *models.Listing) {
			t := models.OTHER
			l.Type = &t
		}, false},
		{"missing application URL is excluded", func(l *models.Listing) { l.ApplicationURL = nil }, false},
		{"blank application URL is excluded", func(l *models.Listing) {
			blank := "   "
			l.ApplicationURL = &blank
		}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict := EvaluateForBoard(boardListing(tc.mutate))

			if verdict.Eligible != tc.eligible {
				t.Errorf("eligible = %v, want %v (reasons: %v)", verdict.Eligible, tc.eligible, verdict.Reasons)
			}
			if !verdict.Eligible && len(verdict.Reasons) == 0 {
				t.Error("an ineligible listing must explain why")
			}
		})
	}
}

func TestEvaluateForBoardIgnoresVolatileState(t *testing.T) {
	// Eligibility must not depend on state that changes over a listing's life.
	// If it did, an already-posted Discord thread would stop receiving updates
	// the moment the job expired, and would sit there stale.
	past := time.Now().Add(-30 * 24 * time.Hour)

	expired := boardListing(func(l *models.Listing) { l.CloseDate = &past })

	if !EvaluateForBoard(expired).Eligible {
		t.Error("an elapsed close date must not affect eligibility")
	}
}

func TestEvaluateForBoardScoring(t *testing.T) {
	soon := time.Now().Add(14 * 24 * time.Hour)
	long := string(make([]byte, 1600))
	medium := string(make([]byte, 600))

	cases := []struct {
		name   string
		mutate func(*models.Listing)
		want   int
	}{
		{"bare listing scores only its named state", func(l *models.Listing) {
			l.ApplicationURL = nil
		}, 10},
		{"sponsored employer", func(l *models.Listing) { l.IsSponsored = true }, 30 + 15 + 10},
		{"big tech counts as notable", func(l *models.Listing) { l.IndustryField = models.BIG_TECH }, 30 + 15 + 10},
		{"quant trading counts as notable", func(l *models.Listing) { l.IndustryField = models.QUANT_TRADING }, 30 + 15 + 10},
		{"recognised tech scores lower than notable", func(l *models.Listing) { l.IndustryField = models.TECH }, 15 + 15 + 10},
		{"banks are recognised", func(l *models.Listing) { l.IndustryField = models.BANKS }, 15 + 15 + 10},
		{"close date", func(l *models.Listing) { l.CloseDate = &soon }, 20 + 15 + 10},
		{"long description", func(l *models.Listing) { l.Description = &long }, 15 + 15 + 10},
		{"medium description", func(l *models.Listing) { l.Description = &medium }, 8 + 15 + 10},
		{"aggregator link earns no first-party points", func(l *models.Listing) {
			u := "https://au.gradconnection.com/employers/x/jobs/y"
			l.ApplicationURL = &u
		}, 10},
		{"aggregator subdomain is still an aggregator", func(l *models.Listing) {
			u := "https://www.linkedin.com/jobs/view/123"
			l.ApplicationURL = &u
		}, 10},
		{"nationwide only, no state bonus", func(l *models.Listing) {
			l.Locations = []models.LocationType{models.AUSTRALIA}
		}, 15},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluateForBoard(boardListing(tc.mutate)).Score; got != tc.want {
				t.Errorf("score = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBoardScoringStepTagsWithoutDropping(t *testing.T) {
	listings := []models.Listing{
		boardListing(nil),
		boardListing(func(l *models.Listing) { l.Discipline = models.OTHER_DISCIPLINE }),
	}

	result, err := NewBoardScoringStep().Process(context.Background(), listings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The website reads the same collection, so the step must never drop a
	// listing -- it only annotates.
	if len(result) != 2 {
		t.Fatalf("expected 2 listings back, got %d", len(result))
	}
	if !result[0].BoardEligible {
		t.Error("the technical listing should be board eligible")
	}
	if result[1].BoardEligible {
		t.Error("the non-technical listing should not be board eligible")
	}
	if result[0].BoardScore == 0 {
		t.Error("expected a non-zero score on the eligible listing")
	}
}
