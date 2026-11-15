package steps

import (
	"context"
	"sort"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BoardScoring caps the listings it is given. That is the wrong scope for a cap
// on the board: the board is the collection, and a run is whatever the scrapers
// happened to return. Three things follow from the mismatch.
//
// A partial run sizes the cap against a fraction of the board, and leaves the
// rest of it carrying eligibility from some earlier run -- 83 documents eligible
// after a run that marked 37. A listing whose deadline passes keeps its slot,
// because eligibility is deliberately blind to volatile state and the unseen
// sweep only clears listings the sources have dropped. And nothing ever refills
// a slot: the per-employer cap held back 64 gate-passing listings scoring up to
// 90 while the board sat at 92 of 100, with its lowest-scoring thread on 30.
//
// This step answers the same question against the collection instead. It runs
// after the upload, so it sees this run's writes, and it is idempotent: a second
// pass over an unchanged collection writes nothing.

// BoardCandidate is one listing considered for a slot on the board.
type BoardCandidate struct {
	ID       primitive.ObjectID
	Employer string
	Title    string
	// Score is the listing's stored board_score -- what BoardScoring decided
	// when it last saw the listing -- rather than a score recomputed here.
	// Ranking has to agree between the two steps, and part of the score comes
	// from the description, which is stored formatted and no longer matches the
	// text that was scored.
	Score int
	// Passes says the listing clears the hard gates, from EvaluateForBoard.
	Passes bool
	// OnBoard says the collection currently has it board-eligible.
	OnBoard bool
	// Expired says applications are over: the deadline has passed, or the
	// sources stopped carrying the listing. Unlike a failed gate this is the
	// end of the listing, so it is the only thing that takes a slot back.
	Expired bool
}

// BoardPlan is the set of writes that brings the board to the cap.
type BoardPlan struct {
	Promote []primitive.ObjectID
	Demote  []primitive.ObjectID
	// Kept is how many listings were already on the board and stay there.
	Kept int
	// Free is how many slots are still empty afterwards, when there was
	// nothing left that could fill them.
	Free int
}

// PlanBoard decides which listings hold the board's slots.
//
// Nothing already on the board is demoted while applications are still open --
// not for scoring below a listing waiting for a slot, and not for slipping a
// gate. A demotion is not a reshuffle: the bot has a thread for that listing,
// and the board treats an ineligible job as one to archive, so demoting a role
// people can still apply to takes it off the board while it is live. A listing
// reclassified by the AI parser is the case that actually happens, and it is
// not a reason to hide an open role. So the cap is a low-water mark, and a slot
// comes back only when the listing is over: its deadline has passed, or the
// sources have dropped it.
//
// Free slots are filled employer by employer rather than in pure score order.
// Pure score order hands every free slot to whichever employer has the largest
// intake -- 42 of the 64 listings waiting were TikTok's, so the eight free slots
// would have made the board 13% one employer, which is the pile-up the
// per-employer cap exists to prevent. Instead every employer is topped up to
// perEmployer first, and any slots still free go round again, one per employer
// per pass. The per-employer limit is therefore a floor that holds while the
// board is full and relaxes evenly when it is not.
func PlanBoard(candidates []BoardCandidate, limit, perEmployer int) BoardPlan {
	plan := BoardPlan{}

	held := make(map[string]int)
	var promotable []BoardCandidate

	for _, candidate := range candidates {
		switch {
		case candidate.OnBoard && candidate.Expired:
			plan.Demote = append(plan.Demote, candidate.ID)
		case candidate.OnBoard:
			// Holds its slot whether or not it still passes: the thread is up,
			// and it is the deadline that ends a listing.
			plan.Kept++
			held[candidate.Employer]++
		case candidate.Passes && !candidate.Expired:
			promotable = append(promotable, candidate)
		}
	}

	sortCandidates(promotable)

	// A non-positive cap means no cap, matching BOARD_MAX_LISTINGS.
	remaining := limit - plan.Kept
	if limit <= 0 {
		remaining = len(promotable)
	}
	if remaining <= 0 {
		return plan
	}

	// The same for the per-employer limit: unset, the slots go in score order.
	if perEmployer <= 0 {
		for _, candidate := range promotable[:min(remaining, len(promotable))] {
			plan.Promote = append(plan.Promote, candidate.ID)
		}
		plan.Free = remaining - len(plan.Promote)
		return plan
	}

	queues, order := queueByEmployer(promotable)

	// Each pass raises the ceiling by one, so the first fills every employer up
	// to perEmployer and later ones hand out the leftovers evenly.
	//
	// It runs until the slots or the candidates are gone, rather than until a
	// pass promotes nothing: an employer already over the limit -- topped up
	// when the board was emptier -- blocks on one pass and takes its turn on a
	// later one, which is the same queue everyone else is in. The ceiling rises
	// every pass, so a non-empty queue always comes up eventually.
	waiting := len(promotable)
	for ceiling := perEmployer; remaining > 0 && waiting > 0; ceiling++ {
		for _, employer := range order {
			queue := queues[employer]
			for len(queue) > 0 && held[employer] < ceiling && remaining > 0 {
				plan.Promote = append(plan.Promote, queue[0].ID)
				queue = queue[1:]
				held[employer]++
				remaining--
				waiting--
			}
			queues[employer] = queue
		}
	}

	plan.Free = remaining
	return plan
}

// sortCandidates puts the best candidate first, and is fully deterministic
// including ties: the same collection must always produce the same board, or a
// listing would move on and off it between runs and be reposted each time.
func sortCandidates(candidates []BoardCandidate) {
	sort.SliceStable(candidates, func(a, b int) bool {
		x, y := candidates[a], candidates[b]
		if x.Score != y.Score {
			return x.Score > y.Score
		}
		if x.Employer != y.Employer {
			return x.Employer < y.Employer
		}
		if x.Title != y.Title {
			return x.Title < y.Title
		}
		return x.ID.Hex() < y.ID.Hex()
	})
}

// queueByEmployer groups sorted candidates per employer and returns the order
// employers are offered slots in: the strongest listing waiting goes first, so
// a round of top-ups is spent on the best of what is being held back.
func queueByEmployer(sorted []BoardCandidate) (map[string][]BoardCandidate, []string) {
	queues := make(map[string][]BoardCandidate)
	var order []string

	for _, candidate := range sorted {
		if _, seen := queues[candidate.Employer]; !seen {
			order = append(order, candidate.Employer)
		}
		queues[candidate.Employer] = append(queues[candidate.Employer], candidate)
	}

	return queues, order
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// boardWriteBatch bounds one UpdateMany. Large enough that a full board is a
// couple of round trips, small enough that a failure names a manageable set.
const boardWriteBatch = 250

// BoardReconciliationStep brings the board to its cap against the whole
// collection, rather than against the listings of one run.
type BoardReconciliationStep struct {
	mongodb     *database.MongoDB
	limit       int
	perEmployer int
}

// BoardCaps returns the configured board and per-employer caps. The same two
// knobs BoardScoring reads, exported so a tool cannot enforce the cap against a
// different number from the one the pipeline sized the board with.
func BoardCaps() (limit, perEmployer int) {
	return intFromEnv("BOARD_MAX_LISTINGS", defaultBoardLimit),
		intFromEnv("BOARD_MAX_PER_EMPLOYER", defaultPerEmployerLimit)
}

func NewBoardReconciliationStep(mongodb *database.MongoDB) *BoardReconciliationStep {
	limit, perEmployer := BoardCaps()
	return &BoardReconciliationStep{
		mongodb:     mongodb,
		limit:       limit,
		perEmployer: perEmployer,
	}
}

func (s *BoardReconciliationStep) Name() string {
	return "BoardReconciliation"
}

func (s *BoardReconciliationStep) Process(ctx context.Context, listings []models.Listing) ([]models.Listing, error) {
	if s.mongodb == nil {
		utils.Warn("Board reconciliation: no database connection; skipping.")
		return listings, nil
	}

	coll := s.mongodb.GetDatabase().Collection("active_jobs")

	if err := ReconcileBoard(ctx, coll, s.limit, s.perEmployer, true); err != nil {
		// The board is left exactly as the run found it, which is a board, so
		// this does not fail the pipeline: the listings are already uploaded
		// and the next run reconciles again.
		utils.Error("Board reconciliation failed: %v", err)
	}

	return listings, nil
}

// ReconcileBoard reads the collection, plans the board and optionally writes it.
// Exported so cmd/board-reconcile can run the same pass by hand.
func ReconcileBoard(
	ctx context.Context,
	coll *mongo.Collection,
	limit, perEmployer int,
	apply bool,
) error {
	candidates, err := loadBoardCandidates(ctx, coll, time.Now())
	if err != nil {
		return err
	}

	plan := PlanBoard(candidates, limit, perEmployer)

	utils.Log("Board reconciliation: %d candidates, %d already on the board, "+
		"%d to promote, %d to demote, %d slot(s) left free (cap %d, %d per employer)",
		len(candidates), plan.Kept, len(plan.Promote), len(plan.Demote), plan.Free,
		limit, perEmployer)

	reportBoardPlan(candidates, plan)

	if len(plan.Promote) == 0 && len(plan.Demote) == 0 {
		utils.Success("Board reconciliation: the board is already at the cap; nothing to write.")
		return nil
	}

	if !apply {
		utils.Warn("Board reconciliation: dry run, nothing written.")
		return nil
	}

	return writeBoardPlan(ctx, coll, plan)
}

// reportBoardPlan names what the pass would do, employer by employer. A dry run
// is the point of this: the cap decides what the board carries, so changing it
// should be readable before it is written.
func reportBoardPlan(candidates []BoardCandidate, plan BoardPlan) {
	byID := make(map[primitive.ObjectID]BoardCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.ID] = candidate
	}

	for _, group := range []struct {
		label string
		ids   []primitive.ObjectID
	}{
		{"promoting", plan.Promote},
		{"demoting", plan.Demote},
	} {
		if len(group.ids) == 0 {
			continue
		}

		counts := map[string]int{}
		best := map[string]int{}
		for _, id := range group.ids {
			candidate := byID[id]
			counts[candidate.Employer]++
			if candidate.Score > best[candidate.Employer] {
				best[candidate.Employer] = candidate.Score
			}
		}

		employers := make([]string, 0, len(counts))
		for employer := range counts {
			employers = append(employers, employer)
		}
		sort.Slice(employers, func(a, b int) bool {
			if counts[employers[a]] != counts[employers[b]] {
				return counts[employers[a]] > counts[employers[b]]
			}
			return employers[a] < employers[b]
		})

		utils.Log("Board reconciliation: %s %d listing(s)", group.label, len(group.ids))
		for i, employer := range employers {
			if i >= 20 {
				utils.Log("  ... and %d more employer(s)", len(employers)-20)
				break
			}
			utils.Log("  x%d  %s (best score %d)", counts[employer], employer, best[employer])
		}
	}
}

// boardCandidateDoc is the part of a document the board needs. The description
// is deliberately absent: it feeds the score, and the score is read from the
// document rather than recomputed here, so pulling every description out of the
// collection would move megabytes to change nothing.
type boardCandidateDoc struct {
	ID      primitive.ObjectID `bson:"_id"`
	Title   string             `bson:"title"`
	Company struct {
		Name string `bson:"name"`
	} `bson:"company"`
	ApplicationURL *string               `bson:"application_url"`
	Type           *models.JobType       `bson:"type"`
	Locations      []models.LocationType `bson:"locations"`
	Discipline     models.Discipline     `bson:"discipline"`
	BoardScore     int                   `bson:"board_score"`
	BoardEligible  bool                  `bson:"board_eligible"`
	IsSponsored    bool                  `bson:"is_sponsored"`
	CloseDate      *time.Time            `bson:"close_date"`
}

var boardCandidateProjection = bson.M{
	"title":           1,
	"company.name":    1,
	"application_url": 1,
	"type":            1,
	"locations":       1,
	"discipline":      1,
	"board_score":     1,
	"board_eligible":  1,
	"is_sponsored":    1,
	"close_date":      1,
}

// loadBoardCandidates returns every listing that could hold a slot, plus the
// listings holding one that no longer can.
//
// Two queries rather than one over the whole collection: the candidates are the
// open, current listings, and the second query is only there to find the slots
// that a closed or outdated listing is still occupying. Everything else --
// thousands of documents the board will never carry -- is never read.
func loadBoardCandidates(
	ctx context.Context,
	coll *mongo.Collection,
	now time.Time,
) ([]BoardCandidate, error) {
	open := bson.M{
		"outdated": bson.M{"$ne": true},
		"$or": []bson.M{
			{"close_date": nil},
			{"close_date": bson.M{"$gt": now}},
		},
	}
	candidates, err := readBoardCandidates(ctx, coll, open)
	if err != nil {
		return nil, err
	}

	// A listing that has closed or gone outdated fails no gate -- eligibility
	// is deliberately blind to both -- so it has to be excluded here instead,
	// or it holds its slot until its source drops it. This is also the only
	// query that produces a demotion: a slot comes back when applications are
	// over, not when a listing slips a gate while it is still open.
	expired := bson.M{
		"board_eligible": true,
		"$or": []bson.M{
			{"outdated": true},
			{"close_date": bson.M{"$lte": now}},
		},
	}
	stale, err := readBoardCandidates(ctx, coll, expired)
	if err != nil {
		return nil, err
	}
	for i := range stale {
		stale[i].Expired = true
	}

	return append(candidates, stale...), nil
}

func readBoardCandidates(
	ctx context.Context,
	coll *mongo.Collection,
	filter bson.M,
) ([]BoardCandidate, error) {
	cursor, err := coll.Find(ctx, filter, options.Find().SetProjection(boardCandidateProjection))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var candidates []BoardCandidate
	for cursor.Next(ctx) {
		var doc boardCandidateDoc
		if err := cursor.Decode(&doc); err != nil {
			utils.Warn("Board reconciliation: skipping undecodable document: %v", err)
			continue
		}

		// The gates come from EvaluateForBoard rather than being restated here,
		// so a change to what belongs on the board is made in one place. It is
		// pure, which is what lets a stored document be scored like a scraped
		// one.
		verdict := EvaluateForBoard(models.Listing{
			Title:          doc.Title,
			Company:        models.Company{Name: doc.Company.Name},
			ApplicationURL: doc.ApplicationURL,
			Type:           doc.Type,
			Locations:      doc.Locations,
			Discipline:     doc.Discipline,
			IsSponsored:    doc.IsSponsored,
			CloseDate:      doc.CloseDate,
		})

		candidates = append(candidates, BoardCandidate{
			ID:       doc.ID,
			Employer: doc.Company.Name,
			Title:    doc.Title,
			Score:    doc.BoardScore,
			Passes:   verdict.Eligible,
			OnBoard:  doc.BoardEligible,
		})
	}

	return candidates, cursor.Err()
}

func writeBoardPlan(ctx context.Context, coll *mongo.Collection, plan BoardPlan) error {
	// Only the documents that change are written. Every write is a change
	// stream event the Discord bot re-renders a thread for, and a pass that
	// rewrote the whole board every run would be a board-sized burst of them.
	for _, write := range []struct {
		ids      []primitive.ObjectID
		eligible bool
	}{
		{plan.Promote, true},
		{plan.Demote, false},
	} {
		for start := 0; start < len(write.ids); start += boardWriteBatch {
			end := start + boardWriteBatch
			if end > len(write.ids) {
				end = len(write.ids)
			}

			result, err := coll.UpdateMany(ctx,
				bson.M{"_id": bson.M{"$in": write.ids[start:end]}},
				bson.M{"$set": bson.M{"board_eligible": write.eligible}})
			if err != nil {
				return err
			}
			utils.Log("Board reconciliation: set board_eligible=%v on %d listing(s)",
				write.eligible, result.ModifiedCount)
		}
	}

	utils.Success("Board reconciliation: %d promoted, %d demoted",
		len(plan.Promote), len(plan.Demote))
	return nil
}
