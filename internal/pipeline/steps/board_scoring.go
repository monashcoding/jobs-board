package steps

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
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

// defaultHeadlineBonus and defaultMajorBonus are what employer recognition is
// worth to the board cap. They replace an industry-derived approximation of the
// same thing: BIG_TECH and QUANT_TRADING scored 30, the other recognisable
// industries 15, because there was no company list to ask. There is now, it is
// tiered, and it is the signal that approximation was standing in for --
// authoritative, and stable per employer where IndustryField is assigned per
// listing by the AI parser and can move between runs.
//
// The gap between them is the tuning knob, not the sort order. Ordering the cap
// by tier first and score second is the same mistake this scorer already made
// once with CONSULTING: there are more than 100 slots' worth of headline
// employers, so tier-first would fill the board with big tech and quant firms
// and leave no room for the Big 4, the banks or mid-tier Australian tech
// however good their listings are. A weighted bonus keeps prominence decisive
// between comparable listings while a strong major posting -- closing date,
// first-party link, detailed description -- can still beat a bare headline one.
const (
	defaultHeadlineBonus = 30
	defaultMajorBonus    = 15
)

// defaultSponsoredBonus is what a paid placement is worth. It is deliberately
// its own clause rather than an alternative to recognition: under the old
// scoring a sponsored listing and a big-tech listing were interchangeable, and
// a sponsored big-tech listing scored no more than either. Paying for placement
// and being recognisable are different claims on a slot.
const defaultSponsoredBonus = 30

// Read once at startup so the board can be retuned without a rebuild, the same
// way BOARD_MAX_LISTINGS works. Scoring stays deterministic within a process,
// which is what the cap's stable ordering depends on.
var (
	headlineBonus  = intFromEnv("BOARD_HEADLINE_BONUS", defaultHeadlineBonus)
	majorBonus     = intFromEnv("BOARD_MAJOR_BONUS", defaultMajorBonus)
	sponsoredBonus = intFromEnv("BOARD_SPONSORED_BONUS", defaultSponsoredBonus)
)

// BoardVerdict is the result of scoring one listing for the Discord job board.
type BoardVerdict struct {
	Score    int
	Eligible bool
	// Tier is how prominent the employer is. It is a separate axis from Score,
	// which blends industry, recency and the rest: a thin listing at a headline
	// employer can score below a strong one at an unknown employer, which is
	// backwards for ordering by recognition. The bot's weekly recap shows only
	// a handful of the week's postings and uses this to choose which.
	Tier string
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

	// Tiered whether or not the listing is eligible, so the field is never
	// absent on a scored document and no consumer has to guess.
	verdict.Tier = utils.CompanyTier(listing.Company.Name)

	// --- Hard gates -------------------------------------------------------
	if !boardDisciplines[listing.Discipline] {
		verdict.Reasons = append(verdict.Reasons, "discipline is not SOFTWARE, DATA or CYBER")
	}

	// The board is an Australian one, with no exception for a prominent
	// employer's overseas roles. That exception was built and then measured
	// against the collection: of 262 non-Australian listings, the ones it would
	// have admitted were TikTok's e-commerce programs in Malaysia, Indonesia
	// and Thailand. The quant desks it was meant for -- Jane Street Hong Kong
	// and Singapore -- do not carry a location the classifier can read at all.
	// An overseas listing is still placed as precisely as the source allows --
	// APAC where it says so -- but no overseas location opens this gate.
	if !utils.IsAustralian(listing.Locations) {
		verdict.Reasons = append(verdict.Reasons, "no Australian location")
	}

	if listing.Type == nil || (*listing.Type != models.INTERN && *listing.Type != models.GRADUATE) {
		verdict.Reasons = append(verdict.Reasons, "not an intern or graduate role")
	}

	if listing.ApplicationURL == nil || strings.TrimSpace(*listing.ApplicationURL) == "" {
		verdict.Reasons = append(verdict.Reasons, "no application URL")
	}

	// The board covers employers students recognise and actually apply to.
	// Without this the surplus was mostly placement agencies, recruiters and
	// civil engineering consultancies -- The Intern Group, FDM Group and Hays
	// were three of the five largest sources of listings by volume.
	if !utils.IsNotableCompany(listing.Company.Name) {
		verdict.Reasons = append(verdict.Reasons, "employer is not on the board's company list")
	}

	verdict.Eligible = len(verdict.Reasons) == 0

	// Tiered regardless of eligibility, so the field is always populated and no
	// consumer has to guess. An employer off the list is unranked by definition.
	verdict.Tier = utils.CompanyTier(listing.Company.Name)

	// --- Score ------------------------------------------------------------
	// Employer recognition, from the board's own company list. Every eligible
	// listing scores here and none scores zero: the company list is a hard gate,
	// so an eligible employer is headline or major by definition. The bonus
	// separates the two, which is the same reason the recap needed tiers rather
	// than the gate.
	switch verdict.Tier {
	case utils.TierHeadline:
		verdict.Score += headlineBonus
		verdict.Reasons = append(verdict.Reasons, fmt.Sprintf("+%d headline employer", headlineBonus))
	case utils.TierMajor:
		verdict.Score += majorBonus
		verdict.Reasons = append(verdict.Reasons, fmt.Sprintf("+%d major employer", majorBonus))
	}

	if listing.IsSponsored {
		verdict.Score += sponsoredBonus
		verdict.Reasons = append(verdict.Reasons, fmt.Sprintf("+%d sponsored", sponsoredBonus))
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

// defaultBoardLimit caps how many listings the board carries at once. A Discord
// forum tops out at 1000 active threads, but the practical limit is much lower:
// a board nobody can scan is not useful. Every gate passing still left ~223
// listings, so the count is capped explicitly rather than hoped for.
const defaultBoardLimit = 100

// defaultPerEmployerLimit caps how many of one employer's roles reach the board.
// A single large intake otherwise takes the whole forum: TikTok alone accounted
// for 29 of 53 eligible listings, and a board that is half one company stops
// being a board.
const defaultPerEmployerLimit = 5

// BoardScoringStep annotates listings with their Discord job board verdict. It
// only tags; nothing is dropped, and no listing is withheld from the website.
type BoardScoringStep struct {
	// limit is the maximum number of listings marked eligible in one run. The
	// highest scoring survive; the rest are scored but not posted.
	limit int
	// perEmployer caps how many of one employer's roles reach the board.
	perEmployer int
}

func NewBoardScoringStep() *BoardScoringStep {
	return &BoardScoringStep{
		limit:       intFromEnv("BOARD_MAX_LISTINGS", defaultBoardLimit),
		perEmployer: intFromEnv("BOARD_MAX_PER_EMPLOYER", defaultPerEmployerLimit),
	}
}

// intFromEnv reads a numeric override so the board can be tuned without a
// rebuild. A non-positive value means no cap.
func intFromEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil {
		utils.Warn("%s=%q is not a number; using the default of %d.", name, raw, fallback)
		return fallback
	}

	return parsed
}

func (s *BoardScoringStep) Name() string {
	return "BoardScoring"
}

func (s *BoardScoringStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	passed := 0

	for i, listing := range listings {
		verdict := EvaluateForBoard(listing)
		listings[i].BoardScore = verdict.Score
		listings[i].BoardEligible = verdict.Eligible
		listings[i].CompanyTier = verdict.Tier

		if verdict.Eligible {
			passed++
		}
	}

	// Per employer first, so one large intake cannot consume the global cap
	// before smaller employers are considered.
	perEmployerDropped := applyPerEmployerLimit(listings, s.perEmployer)
	eligible := applyBoardLimit(listings, s.limit)

	if perEmployerDropped > 0 {
		utils.Log("Board scoring: %d listings held back by the %d-per-employer cap.",
			perEmployerDropped, s.perEmployer)
	}

	if eligible < passed {
		utils.Success("Board scoring: %d/%d listings passed the gates, top %d posted (cap %d).",
			passed, len(listings), eligible, s.limit)
	} else {
		utils.Success("Board scoring: %d/%d listings eligible for the job board", eligible, len(listings))
	}

	return listings, nil
}

// applyBoardLimit keeps only the highest scoring eligible listings and returns
// how many remain eligible.
//
// The ordering is fully deterministic, including ties. A listing that drops out
// one day and returns the next would be posted as a brand new thread, so the
// same inputs must always produce the same board.
func applyBoardLimit(listings []models.Listing, limit int) int {
	if limit <= 0 {
		return countEligible(listings)
	}

	indices := make([]int, 0, len(listings))
	for i := range listings {
		if listings[i].BoardEligible {
			indices = append(indices, i)
		}
	}

	if len(indices) <= limit {
		return len(indices)
	}

	sort.SliceStable(indices, func(a, b int) bool {
		x, y := listings[indices[a]], listings[indices[b]]
		if x.BoardScore != y.BoardScore {
			return x.BoardScore > y.BoardScore
		}
		if x.Company.Name != y.Company.Name {
			return x.Company.Name < y.Company.Name
		}
		if x.Title != y.Title {
			return x.Title < y.Title
		}
		return GenerateListingFingerprint(x) < GenerateListingFingerprint(y)
	})

	for _, i := range indices[limit:] {
		listings[i].BoardEligible = false
	}

	return limit
}

// applyPerEmployerLimit keeps at most perEmployer of each employer's listings,
// highest scoring first, and returns how many it held back. Ordering is
// deterministic for the same reason the global cap's is: a listing that drops
// out one run and returns the next is posted as an entirely new thread.
func applyPerEmployerLimit(listings []models.Listing, perEmployer int) int {
	if perEmployer <= 0 {
		return 0
	}

	byEmployer := make(map[string][]int)
	for i := range listings {
		if listings[i].BoardEligible {
			name := listings[i].Company.Name
			byEmployer[name] = append(byEmployer[name], i)
		}
	}

	dropped := 0
	for _, indices := range byEmployer {
		if len(indices) <= perEmployer {
			continue
		}

		sort.SliceStable(indices, func(a, b int) bool {
			x, y := listings[indices[a]], listings[indices[b]]
			if x.BoardScore != y.BoardScore {
				return x.BoardScore > y.BoardScore
			}
			if x.Title != y.Title {
				return x.Title < y.Title
			}
			return GenerateListingFingerprint(x) < GenerateListingFingerprint(y)
		})

		for _, i := range indices[perEmployer:] {
			listings[i].BoardEligible = false
			dropped++
		}
	}

	return dropped
}

func countEligible(listings []models.Listing) int {
	n := 0
	for i := range listings {
		if listings[i].BoardEligible {
			n++
		}
	}
	return n
}
