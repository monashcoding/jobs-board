package steps

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// candidate builds one candidate with a readable id, so a failure names the
// listing rather than an ObjectID.
func candidate(employer, title string, score int, passes, onBoard bool) BoardCandidate {
	return BoardCandidate{
		ID:       primitive.NewObjectID(),
		Employer: employer,
		Title:    title,
		Score:    score,
		Passes:   passes,
		OnBoard:  onBoard,
	}
}

// expired is a listing whose applications are over: its deadline has passed, or
// the sources stopped carrying it.
func expired(employer, title string, score int) BoardCandidate {
	c := candidate(employer, title, score, false, true)
	c.Expired = true
	return c
}

func names(plan BoardPlan, candidates []BoardCandidate) []string {
	byID := make(map[primitive.ObjectID]BoardCandidate, len(candidates))
	for _, c := range candidates {
		byID[c.ID] = c
	}

	out := make([]string, 0, len(plan.Promote))
	for _, id := range plan.Promote {
		c := byID[id]
		out = append(out, c.Employer+" "+c.Title)
	}
	return out
}

// The board sat at 92 of 100 while 64 gate-passing listings were held back by
// the per-employer cap. Free slots have to be filled or the cap is costing
// capacity rather than protecting proportion.
func TestPlanBoardFillsFreeSlots(t *testing.T) {
	candidates := []BoardCandidate{
		candidate("Atlassian", "on the board", 60, true, true),
		candidate("Canva", "waiting", 55, true, false),
		candidate("Optiver", "waiting", 40, true, false),
	}

	plan := PlanBoard(candidates, 3, 5)

	if plan.Kept != 1 {
		t.Errorf("kept = %d, want 1", plan.Kept)
	}
	if got := names(plan, candidates); len(got) != 2 {
		t.Fatalf("promoted %v, want both waiting listings", got)
	}
	if plan.Free != 0 {
		t.Errorf("free = %d, want 0", plan.Free)
	}
}

// A demotion is not a reshuffle: the bot has a thread for that listing, and a
// listing that leaves the board and returns is posted again as a new thread.
func TestPlanBoardNeverDemotesAWeakerListingThatIsAlreadyUp(t *testing.T) {
	candidates := []BoardCandidate{
		candidate("Sophos", "weak but posted", 30, true, true),
		candidate("TikTok", "strong and waiting", 90, true, false),
	}

	plan := PlanBoard(candidates, 1, 5)

	if len(plan.Demote) != 0 {
		t.Errorf("demoted %d listings; a full board must hold what it has", len(plan.Demote))
	}
	if len(plan.Promote) != 0 {
		t.Errorf("promoted %v with no free slot", names(plan, candidates))
	}
}

// Closed and outdated listings fail no gate -- eligibility is deliberately
// blind to both -- so the slot has to be taken back here or it is held until
// the source drops the listing.
func TestPlanBoardDemotesListingsWhoseApplicationsAreOver(t *testing.T) {
	closed := expired("Deloitte", "closed", 40)
	gone := expired("Amazon", "outdated", 45)
	waiting := candidate("Canva", "waiting", 55, true, false)

	plan := PlanBoard([]BoardCandidate{closed, gone, waiting}, 2, 5)

	if len(plan.Demote) != 2 {
		t.Fatalf("demoted %d, want both expired listings", len(plan.Demote))
	}
	if plan.Kept != 0 {
		t.Errorf("kept = %d, want 0", plan.Kept)
	}
	// Both slots are free once the two are demoted, and only one listing wants
	// one, so the board ends up under the cap rather than padded.
	if len(plan.Promote) != 1 || plan.Free != 1 {
		t.Errorf("promote = %v, free = %d; want the one waiting listing and a free slot",
			names(plan, []BoardCandidate{closed, gone, waiting}), plan.Free)
	}
}

// The bot archives the thread of a job it no longer considers eligible, so
// demoting an open role takes it off the board while people can still apply.
// A listing reclassified by the AI parser is the case that actually happens.
func TestPlanBoardKeepsAnOpenListingThatSlippedAGate(t *testing.T) {
	slipped := candidate("Atlassian", "reclassified", 60, false, true)
	waiting := candidate("Canva", "waiting", 55, true, false)

	plan := PlanBoard([]BoardCandidate{slipped, waiting}, 1, 5)

	if len(plan.Demote) != 0 {
		t.Errorf("demoted an open listing; its thread would be archived mid-application")
	}
	if plan.Kept != 1 {
		t.Errorf("kept = %d, want 1: it still holds a slot", plan.Kept)
	}
	if len(plan.Promote) != 0 {
		t.Errorf("promoted %v over the cap", names(plan, []BoardCandidate{slipped, waiting}))
	}
}

// Once its deadline passes, the same listing does give up the slot.
func TestPlanBoardTakesTheSlotBackWhenAGateSlipperCloses(t *testing.T) {
	slipped := expired("Atlassian", "reclassified and closed", 60)
	waiting := candidate("Canva", "waiting", 55, true, false)
	candidates := []BoardCandidate{slipped, waiting}

	plan := PlanBoard(candidates, 1, 5)

	if len(plan.Demote) != 1 {
		t.Fatalf("demoted %d, want the closed listing", len(plan.Demote))
	}
	if got := names(plan, candidates); len(got) != 1 || got[0] != "Canva waiting" {
		t.Errorf("promoted %v, want the freed slot to go to the waiting listing", got)
	}
}

// A listing that never made the board and has since closed is not a candidate
// for the slot it never had.
func TestPlanBoardIgnoresAnExpiredListingThatWasNeverUp(t *testing.T) {
	gone := BoardCandidate{ID: primitive.NewObjectID(), Employer: "Canva",
		Title: "closed", Score: 90, Passes: true, Expired: true}

	plan := PlanBoard([]BoardCandidate{gone}, 5, 5)

	if len(plan.Promote) != 0 || len(plan.Demote) != 0 {
		t.Errorf("an expired listing produced writes: %+v", plan)
	}
}

// Pure score order would hand all the free slots to whichever employer has the
// largest intake, which is the pile-up the per-employer cap exists to prevent.
func TestPlanBoardSpreadsFreeSlotsAcrossEmployers(t *testing.T) {
	var candidates []BoardCandidate
	// One employer with a large intake, all of it scoring above the others.
	for i, title := range []string{"a", "b", "c", "d", "e", "f"} {
		candidates = append(candidates, candidate("TikTok", title, 90-i, true, false))
	}
	candidates = append(candidates,
		candidate("Canva", "one", 55, true, false),
		candidate("Optiver", "one", 50, true, false),
	)

	plan := PlanBoard(candidates, 6, 2)

	got := names(plan, candidates)
	want := []string{
		// Pass one: every employer up to two, best first.
		"TikTok a", "TikTok b", "Canva one", "Optiver one",
		// Pass two: one more each, and only TikTok has anything left.
		"TikTok c", "TikTok d",
	}
	if len(got) != len(want) {
		t.Fatalf("promoted %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("promoted %v, want %v", got, want)
		}
	}
}

// The per-employer limit is a floor that holds while the board is full: with no
// free slots to hand out, nobody goes past it.
func TestPlanBoardKeepsThePerEmployerLimitWhenTheBoardIsFull(t *testing.T) {
	var candidates []BoardCandidate
	for _, title := range []string{"a", "b", "c", "d"} {
		candidates = append(candidates, candidate("TikTok", title, 90, true, false))
	}
	candidates = append(candidates, candidate("Canva", "one", 40, true, false))

	plan := PlanBoard(candidates, 3, 2)

	got := names(plan, candidates)
	want := []string{"TikTok a", "TikTok b", "Canva one"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("promoted %v, want %v", got, want)
		}
	}
	if len(got) != 3 {
		t.Fatalf("promoted %v, want exactly the cap", got)
	}
}

// An employer already over the limit -- topped up by an earlier pass when the
// board was emptier -- keeps what it has and waits its turn again.
func TestPlanBoardToleratesAnEmployerAlreadyOverTheLimit(t *testing.T) {
	var candidates []BoardCandidate
	for _, title := range []string{"a", "b", "c"} {
		candidates = append(candidates, candidate("TikTok", title, 90, true, true))
	}
	candidates = append(candidates,
		candidate("TikTok", "waiting", 85, true, false),
		candidate("Canva", "waiting", 40, true, false),
	)

	plan := PlanBoard(candidates, 5, 2)

	got := names(plan, candidates)
	want := []string{"Canva waiting", "TikTok waiting"}
	if len(got) != len(want) {
		t.Fatalf("promoted %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("promoted %v, want %v: the employer under the limit goes first", got, want)
		}
	}
}

// Same collection, same board: a listing that moved on and off between runs
// would be reposted as a brand new thread each time.
func TestPlanBoardIsDeterministicAcrossTies(t *testing.T) {
	build := func() []BoardCandidate {
		out := []BoardCandidate{
			{ID: mustID("650000000000000000000003"), Employer: "Canva", Title: "same", Score: 50, Passes: true},
			{ID: mustID("650000000000000000000001"), Employer: "Canva", Title: "same", Score: 50, Passes: true},
			{ID: mustID("650000000000000000000002"), Employer: "Canva", Title: "other", Score: 50, Passes: true},
		}
		return out
	}

	first := PlanBoard(build(), 2, 5)
	second := PlanBoard(build(), 2, 5)

	if len(first.Promote) != 2 {
		t.Fatalf("promoted %d, want 2", len(first.Promote))
	}
	for i := range first.Promote {
		if first.Promote[i] != second.Promote[i] {
			t.Fatalf("two runs over the same input promoted different listings")
		}
	}
	// Title breaks the score tie before the id does.
	if first.Promote[0].Hex() != "650000000000000000000002" {
		t.Errorf("first promoted %s, want the title that sorts first",
			first.Promote[0].Hex())
	}
}

// BOARD_MAX_LISTINGS and BOARD_MAX_PER_EMPLOYER both read a non-positive value
// as "no cap", and this pass has to mean the same thing by it.
func TestPlanBoardTreatsNonPositiveCapsAsUncapped(t *testing.T) {
	var candidates []BoardCandidate
	for _, title := range []string{"a", "b", "c"} {
		candidates = append(candidates, candidate("TikTok", title, 90, true, false))
	}

	if plan := PlanBoard(candidates, 0, 2); len(plan.Promote) != 3 {
		t.Errorf("promoted %d with no board cap, want all 3", len(plan.Promote))
	}
	if plan := PlanBoard(candidates, 10, 0); len(plan.Promote) != 3 {
		t.Errorf("promoted %d with no per-employer cap, want all 3", len(plan.Promote))
	}
}

func TestPlanBoardWritesNothingForAnUnchangedBoard(t *testing.T) {
	candidates := []BoardCandidate{
		candidate("Canva", "one", 55, true, true),
		candidate("Optiver", "one", 50, true, true),
	}

	plan := PlanBoard(candidates, 2, 5)

	if len(plan.Promote) != 0 || len(plan.Demote) != 0 {
		t.Errorf("a board already at the cap produced writes: %+v", plan)
	}
}

func mustID(hex string) primitive.ObjectID {
	id, err := primitive.ObjectIDFromHex(hex)
	if err != nil {
		panic(err)
	}
	return id
}
