package steps

import (
	"context"
	"testing"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
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
		{"OTHER job type is excluded: the board is for students", func(l *models.Listing) {
			t := models.OTHER
			l.Type = &t
		}, false},
		{"a job type outside the board's set is excluded", func(l *models.Listing) {
			t := models.JobType("VOLUNTEER")
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

	// The base listing is a headline employer with a first-party link and a
	// named state, so every case below carries 30 + 15 + 10 unless it removes
	// one of them.
	cases := []struct {
		name   string
		mutate func(*models.Listing)
		want   int
	}{
		{"headline employer, first-party link, named state", nil, 30 + 15 + 10},
		{"a major employer scores below a headline one", func(l *models.Listing) {
			l.Company.Name = "Deloitte"
		}, 15 + 15 + 10},

		// Recognition used to come from IndustryField because there was no
		// company list to ask. It must not be scored twice now there is.
		{"industry no longer scores on its own", func(l *models.Listing) {
			l.IndustryField = models.BIG_TECH
		}, 30 + 15 + 10},
		{"a recognised industry adds nothing to a major employer", func(l *models.Listing) {
			l.Company.Name = "Deloitte"
			l.IndustryField = models.CONSULTING
		}, 15 + 15 + 10},

		// Sponsorship is a separate claim on a slot, so it stacks rather than
		// standing in for recognition.
		{"sponsorship stacks on recognition", func(l *models.Listing) {
			l.IsSponsored = true
		}, 30 + 30 + 15 + 10},
		{"a sponsored major employer still scores below a sponsored headline one", func(l *models.Listing) {
			l.Company.Name = "Deloitte"
			l.IsSponsored = true
		}, 15 + 30 + 15 + 10},

		{"no application link", func(l *models.Listing) { l.ApplicationURL = nil }, 30 + 10},
		{"close date", func(l *models.Listing) { l.CloseDate = &soon }, 30 + 20 + 15 + 10},
		{"long description", func(l *models.Listing) { l.Description = &long }, 30 + 15 + 15 + 10},
		{"medium description", func(l *models.Listing) { l.Description = &medium }, 30 + 8 + 15 + 10},
		{"aggregator link earns no first-party points", func(l *models.Listing) {
			u := "https://au.gradconnection.com/employers/x/jobs/y"
			l.ApplicationURL = &u
		}, 30 + 10},
		{"aggregator subdomain is still an aggregator", func(l *models.Listing) {
			u := "https://www.linkedin.com/jobs/view/123"
			l.ApplicationURL = &u
		}, 30 + 10},
		{"nationwide only, no state bonus", func(l *models.Listing) {
			l.Locations = []models.LocationType{models.AUSTRALIA}
		}, 30 + 15},

		// Off the company list entirely: ineligible, and scored with no
		// recognition at all. It cannot reach the cap, but the score should
		// still say why.
		{"an untiered employer earns no recognition", func(l *models.Listing) {
			l.Company.Name = "The Intern Group"
		}, 15 + 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluateForBoard(boardListing(tc.mutate)).Score; got != tc.want {
				t.Errorf("score = %d, want %d", got, tc.want)
			}
		})
	}
}

// The cap is spent on recognition, not just on how well a listing is written.
// A bare headline posting and a well-formed major one are both meant to be
// able to win, which is what keeps the board from becoming big tech only.
func TestRecognitionOutweighsOneQualitySignalButNotAll(t *testing.T) {
	soon := time.Now().Add(14 * 24 * time.Hour)
	long := string(make([]byte, 1600))

	bareHeadline := EvaluateForBoard(boardListing(func(l *models.Listing) {
		l.ApplicationURL = nil
		l.Locations = []models.LocationType{models.AUSTRALIA}
	})).Score

	majorWithOneSignal := EvaluateForBoard(boardListing(func(l *models.Listing) {
		l.Company.Name = "Deloitte"
		l.ApplicationURL = nil
		l.Locations = []models.LocationType{models.AUSTRALIA}
		l.CloseDate = &soon
	})).Score

	fullMajor := EvaluateForBoard(boardListing(func(l *models.Listing) {
		l.Company.Name = "Deloitte"
		l.CloseDate = &soon
		l.Description = &long
	})).Score

	if bareHeadline >= majorWithOneSignal {
		t.Errorf("a headline employer should not outrank a major one on recognition alone: %d vs %d",
			bareHeadline, majorWithOneSignal)
	}
	if fullMajor <= bareHeadline {
		t.Errorf("a fully-formed major listing should beat a bare headline one: %d vs %d",
			fullMajor, bareHeadline)
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

func TestNotableCompanyIsAHardGate(t *testing.T) {
	applicationURL := "https://example.com/apply"
	jobType := models.GRADUATE
	build := func(company string) models.Listing {
		return models.Listing{
			Title:          "Graduate Software Engineer",
			Company:        models.Company{Name: company},
			Discipline:     models.SOFTWARE,
			Type:           &jobType,
			Locations:      []models.LocationType{models.VIC},
			ApplicationURL: &applicationURL,
		}
	}

	if !EvaluateForBoard(build("Atlassian")).Eligible {
		t.Error("a listing at a company on the board's list must be eligible")
	}
	if EvaluateForBoard(build("The Intern Group")).Eligible {
		t.Error("a placement agency must not reach the board")
	}
}

func TestApplyBoardLimitKeepsTheHighestScoring(t *testing.T) {
	listings := []models.Listing{
		{Title: "low", Company: models.Company{Name: "A"}, BoardScore: 10, BoardEligible: true},
		{Title: "high", Company: models.Company{Name: "B"}, BoardScore: 90, BoardEligible: true},
		{Title: "mid", Company: models.Company{Name: "C"}, BoardScore: 50, BoardEligible: true},
		{Title: "ineligible", Company: models.Company{Name: "D"}, BoardScore: 99, BoardEligible: false},
	}

	if got := applyBoardLimit(listings, 2); got != 2 {
		t.Fatalf("applyBoardLimit returned %d, want 2", got)
	}

	kept := map[string]bool{}
	for _, l := range listings {
		if l.BoardEligible {
			kept[l.Title] = true
		}
	}
	if !kept["high"] || !kept["mid"] || kept["low"] {
		t.Errorf("wrong listings kept: %v", kept)
	}
	if kept["ineligible"] {
		t.Error("the cap must never promote a listing that failed a gate")
	}
}

// A listing that drops out one run and returns the next is posted as a brand new
// thread, so ties must break the same way every time.
func TestApplyBoardLimitIsDeterministicAcrossTies(t *testing.T) {
	build := func() []models.Listing {
		var out []models.Listing
		for _, name := range []string{"Delta", "Alpha", "Charlie", "Bravo"} {
			out = append(out, models.Listing{
				Title: "Graduate Software Engineer", Company: models.Company{Name: name},
				BoardScore: 40, BoardEligible: true,
			})
		}
		return out
	}

	first, second := build(), build()
	// Feed the second run the same listings in a different order.
	second[0], second[3] = second[3], second[0]

	applyBoardLimit(first, 2)
	applyBoardLimit(second, 2)

	survivors := func(ls []models.Listing) map[string]bool {
		m := map[string]bool{}
		for _, l := range ls {
			if l.BoardEligible {
				m[l.Company.Name] = true
			}
		}
		return m
	}

	a, b := survivors(first), survivors(second)
	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("expected 2 survivors each, got %d and %d", len(a), len(b))
	}
	for name := range a {
		if !b[name] {
			t.Errorf("input order changed the board: %v vs %v", a, b)
		}
	}
}

func TestApplyBoardLimitDisabled(t *testing.T) {
	listings := []models.Listing{
		{Title: "a", BoardScore: 10, BoardEligible: true},
		{Title: "b", BoardScore: 20, BoardEligible: true},
	}
	if got := applyBoardLimit(listings, 0); got != 2 {
		t.Errorf("a non-positive limit must not cap, got %d", got)
	}
	for _, l := range listings {
		if !l.BoardEligible {
			t.Error("a non-positive limit must leave eligibility untouched")
		}
	}
}

func TestApplyPerEmployerLimit(t *testing.T) {
	build := func(company, title string, score int) models.Listing {
		return models.Listing{
			Title: title, Company: models.Company{Name: company},
			BoardScore: score, BoardEligible: true,
		}
	}

	listings := []models.Listing{
		build("TikTok", "role a", 90),
		build("TikTok", "role b", 80),
		build("TikTok", "role c", 70),
		build("TikTok", "role d", 60),
		build("CommBank", "role e", 50),
		build("CommBank", "role f", 40),
	}

	if dropped := applyPerEmployerLimit(listings, 2); dropped != 2 {
		t.Fatalf("expected 2 held back, got %d", dropped)
	}

	kept := map[string][]string{}
	for _, l := range listings {
		if l.BoardEligible {
			kept[l.Company.Name] = append(kept[l.Company.Name], l.Title)
		}
	}
	if len(kept["TikTok"]) != 2 {
		t.Errorf("TikTok should keep 2, kept %v", kept["TikTok"])
	}
	// The highest scoring of the employer's roles must be the survivors.
	if kept["TikTok"][0] != "role a" || kept["TikTok"][1] != "role b" {
		t.Errorf("wrong TikTok roles kept: %v", kept["TikTok"])
	}
	// An employer under the cap must be untouched.
	if len(kept["CommBank"]) != 2 {
		t.Errorf("CommBank was capped when it should not be: %v", kept["CommBank"])
	}
}

func TestApplyPerEmployerLimitDisabled(t *testing.T) {
	listings := []models.Listing{
		{Title: "a", Company: models.Company{Name: "X"}, BoardEligible: true},
		{Title: "b", Company: models.Company{Name: "X"}, BoardEligible: true},
	}
	if dropped := applyPerEmployerLimit(listings, 0); dropped != 0 {
		t.Errorf("a non-positive cap must hold nothing back, got %d", dropped)
	}
	for _, l := range listings {
		if !l.BoardEligible {
			t.Error("a non-positive cap must leave eligibility untouched")
		}
	}
}

// The tier is a separate axis from the score. The bot's weekly recap shows only
// a handful of the week's postings and orders them by this, so it has to be set
// wherever the score is.
func TestBoardScoringStepTiersEveryListing(t *testing.T) {
	listings := []models.Listing{
		boardListing(nil),
		boardListing(func(l *models.Listing) { l.Company.Name = "Deloitte" }),
		// Off the company list, so ineligible -- but still tiered, so the field
		// is never absent.
		boardListing(func(l *models.Listing) { l.Company.Name = "The Intern Group" }),
	}

	result, err := NewBoardScoringStep().Process(context.Background(), listings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{utils.TierHeadline, utils.TierMajor, utils.TierUnranked}
	for i, tier := range want {
		if result[i].CompanyTier != tier {
			t.Errorf("listing %d (%s): CompanyTier = %q, want %q",
				i, result[i].Company.Name, result[i].CompanyTier, tier)
		}
	}
}

// Every eligible listing is at an employer on the board's company list, and the
// tier lists cover that list, so an eligible listing can never come out
// unranked. The bot relies on this: an untiered posting sorts last.
func TestEligibleListingsAreNeverUnranked(t *testing.T) {
	for _, company := range []string{
		"Atlassian", "Deloitte", "Optiver", "Thales Australia", "AGL", "NEXTDC",
	} {
		verdict := EvaluateForBoard(boardListing(func(l *models.Listing) {
			l.Company.Name = company
		}))
		if !verdict.Eligible {
			t.Fatalf("%q should be board eligible", company)
		}
		if verdict.Tier == utils.TierUnranked {
			t.Errorf("%q is eligible but untiered", company)
		}
	}
}

// The board is an Australian one, and stays one. An exemption for a prominent
// employer's overseas roles was built and then measured: the listings it would
// have admitted were TikTok's e-commerce programs in south-east Asia, and the
// quant desks it was meant for carry no readable location at all.
func TestOverseasListingsNeverReachTheBoard(t *testing.T) {
	overseas := func(company string, location models.LocationType) models.Listing {
		return boardListing(func(l *models.Listing) {
			l.Company.Name = company
			l.Locations = []models.LocationType{location}
		})
	}

	for _, tc := range []struct {
		name    string
		listing models.Listing
	}{
		{"a headline employer in APAC", overseas("Jane Street", models.APAC)},
		{"a headline employer somewhere else overseas", overseas("Jane Street", models.OTHERS)},
		{"a major employer in APAC", overseas("Deloitte", models.APAC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if EvaluateForBoard(tc.listing).Eligible {
				t.Error("an overseas listing must not reach the board")
			}
		})
	}
}

// APAC is a location, so it must not be mistaken for a state and earn the
// bonus that goes with naming one.
func TestAPACEarnsNoStateBonus(t *testing.T) {
	apac := EvaluateForBoard(boardListing(func(l *models.Listing) {
		l.Locations = []models.LocationType{models.APAC}
	}))
	australian := EvaluateForBoard(boardListing(nil))

	if apac.Score != australian.Score-10 {
		t.Errorf("APAC scored %d against %d for a named state; it should be 10 lower",
			apac.Score, australian.Score)
	}
}
