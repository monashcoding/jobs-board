package steps

import (
	"context"
	"net/url"
	"strings"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

// boardDisciplines are the technical disciplines the Discord job board covers.
// Non-software engineering is scored but not board-eligible: the audience is
// developers, and civil/mining/mechanical grad programs drown them out.
var boardDisciplines = map[models.Discipline]bool{
	models.SOFTWARE: true,
	models.DATA:     true,
	models.CYBER:    true,
}

// aggregatorHosts are the job boards we scrape. An application URL pointing at
// one of these means the employer never published a first-party posting, which
// correlates with thinner, lower-quality ads.
var aggregatorHosts = []string{
	"gradconnection.com",
	"prosple.com",
	"linkedin.com",
}

// notableIndustries carry the strongest employer-recognition signal available
// without a hand-maintained company list. IndustryField is assigned per listing
// by the AI parser, so it keeps up with employers the sponsored list misses
// (Telstra, Macquarie, TikTok, Quantium, Airwallex).
var notableIndustries = map[models.IndustryField]bool{
	models.BIG_TECH:      true,
	models.QUANT_TRADING: true,
}

var recognisedTechIndustries = map[models.IndustryField]bool{
	models.TECH:               true,
	models.TELECOMMUNICATIONS: true,
	models.BANKS:              true,
}

// BoardVerdict is the result of scoring one listing for the Discord job board.
type BoardVerdict struct {
	Score    int
	Eligible bool
	// Reasons explains the verdict. For an ineligible listing it names every
	// gate that failed, so a listing missing from the board can be explained
	// without re-running the pipeline.
	Reasons []string
}

// EvaluateForBoard decides whether a listing belongs on the Discord job board and
// how strongly. It is pure: no clock, no network, no database.
//
// Eligibility is derived only from STABLE attributes. Volatile state (outdated,
// an elapsed close date) is deliberately excluded — once the bot filters its
// change stream on board_eligible, a listing that flipped would stop delivering
// updates and its already-posted thread would go stale.
func EvaluateForBoard(listing models.Listing) BoardVerdict {
	verdict := BoardVerdict{}

	// --- Hard gates -------------------------------------------------------
	if !boardDisciplines[listing.Discipline] {
		verdict.Reasons = append(verdict.Reasons, "discipline is not SOFTWARE, DATA or CYBER")
	}

	if !utils.IsAustralian(listing.Locations) {
		verdict.Reasons = append(verdict.Reasons, "no Australian location")
	}

	if listing.Type == nil || (*listing.Type != models.INTERN && *listing.Type != models.GRADUATE) {
		verdict.Reasons = append(verdict.Reasons, "not an intern or graduate role")
	}

	if listing.ApplicationURL == nil || strings.TrimSpace(*listing.ApplicationURL) == "" {
		verdict.Reasons = append(verdict.Reasons, "no application URL")
	}

	verdict.Eligible = len(verdict.Reasons) == 0

	// --- Score ------------------------------------------------------------
	if listing.IsSponsored || notableIndustries[listing.IndustryField] {
		verdict.Score += 30
		verdict.Reasons = append(verdict.Reasons, "+30 notable employer")
	} else if recognisedTechIndustries[listing.IndustryField] {
		verdict.Score += 15
		verdict.Reasons = append(verdict.Reasons, "+15 recognised tech industry")
	}

	if listing.CloseDate != nil {
		verdict.Score += 20
		verdict.Reasons = append(verdict.Reasons, "+20 has a closing date")
	}

	if isFirstPartyApplicationURL(listing.ApplicationURL) {
		verdict.Score += 15
		verdict.Reasons = append(verdict.Reasons, "+15 first-party application link")
	}

	if listing.Description != nil {
		switch length := len(strings.TrimSpace(*listing.Description)); {
		case length > 1500:
			verdict.Score += 15
			verdict.Reasons = append(verdict.Reasons, "+15 detailed description")
		case length > 400:
			verdict.Score += 8
			verdict.Reasons = append(verdict.Reasons, "+8 description")
		}
	}

	if utils.HasAustralianState(listing.Locations) {
		verdict.Score += 10
		verdict.Reasons = append(verdict.Reasons, "+10 named state")
	}

	return verdict
}

// isFirstPartyApplicationURL reports whether the apply link goes to the
// employer's own site rather than back to an aggregator we scraped it from.
func isFirstPartyApplicationURL(applicationURL *string) bool {
	if applicationURL == nil {
		return false
	}

	parsed, err := url.Parse(strings.TrimSpace(*applicationURL))
	if err != nil || parsed.Host == "" {
		return false
	}

	host := strings.ToLower(parsed.Host)
	for _, aggregator := range aggregatorHosts {
		if host == aggregator || strings.HasSuffix(host, "."+aggregator) {
			return false
		}
	}

	return true
}

// BoardScoringStep annotates listings with their Discord job board verdict. It
// only tags; nothing is dropped, and no listing is withheld from the website.
type BoardScoringStep struct{}

func NewBoardScoringStep() *BoardScoringStep {
	return &BoardScoringStep{}
}

func (s *BoardScoringStep) Name() string {
	return "BoardScoring"
}

func (s *BoardScoringStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	eligible := 0

	for i, listing := range listings {
		verdict := EvaluateForBoard(listing)
		listings[i].BoardScore = verdict.Score
		listings[i].BoardEligible = verdict.Eligible

		if verdict.Eligible {
			eligible++
		}
	}

	utils.Success("Board scoring: %d/%d listings eligible for the job board", eligible, len(listings))

	return listings, nil
}
