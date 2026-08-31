// Command migrate-fingerprints re-keys the active_jobs collection after a change
// to the fingerprint algorithm, and cleans up the duplicates that re-keying
// reveals.
//
// WHY THIS EXISTS: fingerprint is the upsert key. If the scraper runs with a new
// algorithm before the stored fingerprints are rewritten, every document fails
// its upsert filter and is re-inserted. The Discord bot mirrors each insert as a
// forum thread -- thousands of them, past Discord's 1,000 active-thread cap.
//
// The work is split into two phases because they want opposite things from the
// bot. Re-keying touches every document, and the bot turns an update to a job it
// has no thread for into a new thread, so phase one must run with the bot
// STOPPED. Deleting duplicates should be seen by the bot so it can tidy the
// corresponding threads, so phase two must run with the bot RUNNING.
//
//	# Phase 1 -- bot STOPPED
//	go run ./cmd/migrate-fingerprints                        # dry run
//	go run ./cmd/migrate-fingerprints -apply                 # re-key
//
//	# ...deploy the new scraper, confirm a run reports ~0 inserts, restart the bot
//
//	# Phase 2 -- bot RUNNING
//	go run ./cmd/migrate-fingerprints -delete-duplicates         # dry run
//	go run ./cmd/migrate-fingerprints -delete-duplicates -apply  # delete
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/monashcoding/mploy-golang-core/internal/database"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/pipeline/steps"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

// supersededByField marks a duplicate document and names the document that
// replaced it. Phase two deletes exactly the documents carrying it, so the two
// phases cannot disagree about which rows are redundant.
const supersededByField = "superseded_by"

type docState struct {
	id             primitive.ObjectID
	oldFingerprint string
	newFingerprint string
	oldCompany     string
	newCompany     string
	title          string
	updatedAt      time.Time
}

func main() {
	apply := flag.Bool("apply", false, "write changes; without this the tool only reports what it would do")
	deleteDuplicates := flag.Bool("delete-duplicates", false, "phase two: delete the documents phase one superseded (run with the bot RUNNING)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Load .env the same way the scraper does. Only the MongoDB settings matter
	// here -- this tool never scrapes, so it deliberately does not require the
	// BrightData/OpenAI/Discord variables that config.ValidateEnvironment does.
	if err := godotenv.Load(); err != nil {
		utils.Warn("No .env file loaded (%v), falling back to the process environment.", err)
	}

	if os.Getenv("MONGODB_URI") == "" {
		utils.Fatal("MONGODB_URI is not set. It must point at the same database the scraper writes to.")
	}

	mongodb, err := database.NewMongoDB(ctx)
	if err != nil {
		utils.Fatal("Failed to connect to MongoDB: %v", err)
	}
	defer mongodb.Disconnect(ctx)

	coll := mongodb.GetDatabase().Collection("active_jobs")

	if *deleteDuplicates {
		runDeletePhase(ctx, coll, *apply)
		return
	}

	runRekeyPhase(ctx, coll, *apply)
}

// ---------------------------------------------------------------------------
// Phase 1: re-key
// ---------------------------------------------------------------------------

func runRekeyPhase(ctx context.Context, coll *mongo.Collection, apply bool) {
	states := loadStates(ctx, coll)
	if len(states) == 0 {
		utils.Warn("active_jobs is empty; nothing to migrate")
		return
	}

	// Group by the new fingerprint. A group larger than one means documents that
	// were separate under the old key collapse into a single listing -- exactly
	// the duplicates this change is meant to eliminate. Every member of a group
	// shares title, company, locations and source URL by construction.
	groups := map[string][]docState{}
	for _, state := range states {
		groups[state.newFingerprint] = append(groups[state.newFingerprint], state)
	}

	changed, renamed, duplicates := 0, 0, 0
	for _, group := range groups {
		if len(group) > 1 {
			duplicates += len(group) - 1
		}
		for _, state := range group {
			if state.oldFingerprint != state.newFingerprint {
				changed++
			}
			if state.oldCompany != state.newCompany {
				renamed++
			}
		}
	}

	utils.Log("Phase 1 (re-key) -- run this with the Discord bot STOPPED")
	utils.Log("Documents:                     %d", len(states))
	utils.Log("Distinct listings afterwards:  %d", len(groups))
	utils.Log("Fingerprints to rewrite:       %d", changed)
	utils.Log("Company names to rewrite:      %d", renamed)
	utils.Log("Duplicates to be superseded:   %d", duplicates)

	reportDuplicates(groups)

	if !apply {
		utils.Warn("Dry run. Re-run with -apply to write these changes.")
		return
	}

	rekey(ctx, coll, groups)
}

func loadStates(ctx context.Context, coll *mongo.Collection) []docState {
	cursor, err := coll.Find(ctx, bson.M{})
	if err != nil {
		utils.Fatal("Failed to read active_jobs: %v", err)
	}
	defer cursor.Close(ctx)

	var states []docState
	for cursor.Next(ctx) {
		var raw struct {
			models.Listing `bson:",inline"`
			Fingerprint    string `bson:"fingerprint"`
		}
		if err := cursor.Decode(&raw); err != nil {
			utils.Warn("Skipping undecodable document: %v", err)
			continue
		}

		listing := raw.Listing
		oldCompany := listing.Company.Name

		// The scraper normalises company names before upload and company name
		// feeds the fingerprint, so the migration must apply the CURRENT
		// normalisation rules or the next run will disagree and re-insert.
		normalised, err := steps.StandardiseCompanyName(listing)
		if err != nil {
			utils.Warn("Failed to standardise company name for %q: %v", listing.Title, err)
			normalised = listing
		}

		states = append(states, docState{
			id:             listing.ID,
			oldFingerprint: raw.Fingerprint,
			newFingerprint: steps.GenerateListingFingerprint(normalised),
			oldCompany:     oldCompany,
			newCompany:     normalised.Company.Name,
			title:          listing.Title,
			updatedAt:      listing.UpdatedAt,
		})
	}
	if err := cursor.Err(); err != nil {
		utils.Fatal("Cursor error reading active_jobs: %v", err)
	}

	return states
}

// reportDuplicates prints the collision groups so they can be eyeballed before
// anything is written.
func reportDuplicates(groups map[string][]docState) {
	var collisions [][]docState
	for _, group := range groups {
		if len(group) > 1 {
			collisions = append(collisions, group)
		}
	}
	if len(collisions) == 0 {
		return
	}

	sort.Slice(collisions, func(i, j int) bool { return len(collisions[i]) > len(collisions[j]) })

	utils.Log("Collision groups (these were duplicate documents):")
	for i, group := range collisions {
		if i >= 20 {
			utils.Log("  ... and %d more", len(collisions)-20)
			break
		}
		utils.Log("  x%d  %s | %s", len(group), truncate(group[0].newCompany, 24), truncate(group[0].title, 56))
	}
}

// rekey rewrites fingerprints and company names in place. Within a collision
// group the most recently updated document survives; the others keep their old
// fingerprint (so they cannot collide with the survivor on the next upsert) and
// are marked superseded for phase two to remove.
func rekey(ctx context.Context, coll *mongo.Collection, groups map[string][]docState) {
	updated, superseded, failed := 0, 0, 0

	for fingerprint, group := range groups {
		sort.Slice(group, func(i, j int) bool { return group[i].updatedAt.After(group[j].updatedAt) })

		for i, state := range group {
			update := bson.M{"company.name": state.newCompany}

			if i == 0 {
				update["fingerprint"] = fingerprint
			} else {
				update["outdated"] = true
				update[supersededByField] = group[0].id
			}

			if _, err := coll.UpdateByID(ctx, state.id, bson.M{"$set": update}); err != nil {
				utils.Error("Failed to update %s (%q): %v", state.id.Hex(), state.title, err)
				failed++
				continue
			}

			if i == 0 {
				updated++
			} else {
				superseded++
			}
		}
	}

	utils.Success("Phase 1 complete: %d re-keyed, %d superseded, %d failed", updated, superseded, failed)

	if failed > 0 {
		utils.Error("Some documents failed. Do NOT deploy the new scraper until this reports 0 failures.")
		os.Exit(1)
	}

	utils.Log("Next: deploy the scraper, confirm a run reports ~0 inserts, restart the bot,")
	utils.Log("then run -delete-duplicates to remove the %d superseded documents.", superseded)
}

// ---------------------------------------------------------------------------
// Phase 2: delete superseded duplicates
// ---------------------------------------------------------------------------

func runDeletePhase(ctx context.Context, coll *mongo.Collection, apply bool) {
	filter := bson.M{supersededByField: bson.M{"$exists": true}}

	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		utils.Fatal("Failed to read superseded documents: %v", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.Raw
	for cursor.Next(ctx) {
		raw := make(bson.Raw, len(cursor.Current))
		copy(raw, cursor.Current)
		docs = append(docs, raw)
	}
	if err := cursor.Err(); err != nil {
		utils.Fatal("Cursor error reading superseded documents: %v", err)
	}

	utils.Log("Phase 2 (delete duplicates) -- run this with the Discord bot RUNNING,")
	utils.Log("so it removes the matching threads instead of leaving them orphaned.")
	utils.Log("Superseded documents found: %d", len(docs))

	if len(docs) == 0 {
		utils.Warn("Nothing to delete. Has phase one been run with -apply?")
		return
	}

	for i, doc := range docs {
		if i >= 15 {
			utils.Log("  ... and %d more", len(docs)-15)
			break
		}
		utils.Log("  %s | %s", truncate(lookupString(doc, "company", "name"), 24), truncate(lookupString(doc, "title"), 56))
	}

	if !apply {
		utils.Warn("Dry run. Re-run with -delete-duplicates -apply to delete these documents.")
		return
	}

	backupPath, err := writeBackup(docs)
	if err != nil {
		utils.Fatal("Refusing to delete without a backup: %v", err)
	}
	utils.Success("Wrote %d documents to %s", len(docs), backupPath)
	utils.Log("Restore with: mongoimport --uri \"$MONGODB_URI\" --collection active_jobs --file %s", backupPath)

	result, err := coll.DeleteMany(ctx, filter)
	if err != nil {
		utils.Fatal("Delete failed: %v. The backup at %s is intact.", err, backupPath)
	}

	utils.Success("Phase 2 complete: %d documents deleted", result.DeletedCount)
	utils.Log("The bot deletes threads nobody posted in, and prompts the team on the rest.")
}

// writeBackup dumps documents as newline-delimited MongoDB Extended JSON, the
// format mongoimport reads, so a mistaken delete is recoverable.
func writeBackup(docs []bson.Raw) (string, error) {
	path := fmt.Sprintf("superseded-backup-%s.json", time.Now().Format("2006-01-02_15-04-05"))

	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()

	for _, doc := range docs {
		extended, err := bson.MarshalExtJSON(doc, true, false)
		if err != nil {
			return "", fmt.Errorf("failed to encode a document: %w", err)
		}
		if _, err := file.Write(append(extended, '\n')); err != nil {
			return "", fmt.Errorf("failed to write backup: %w", err)
		}
	}

	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("failed to flush backup: %w", err)
	}

	return path, nil
}

func lookupString(doc bson.Raw, path ...string) string {
	value, err := doc.LookupErr(path...)
	if err != nil {
		return ""
	}
	str, ok := value.StringValueOK()
	if !ok {
		return ""
	}
	return str
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
