package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func rawDoc(t *testing.T, m bson.M) bson.Raw {
	t.Helper()
	encoded, err := bson.Marshal(m)
	if err != nil {
		t.Fatalf("failed to build fixture: %v", err)
	}
	return bson.Raw(encoded)
}

// The backup is the only thing standing between a mistaken delete and lost
// documents, so it has to produce something mongoimport can actually read back.
func TestWriteBackupRoundTrips(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	id := primitive.NewObjectID()
	docs := []bson.Raw{
		rawDoc(t, bson.M{"_id": id, "title": "Graduate Software Engineer", "company": bson.M{"name": "Atlassian"}, "outdated": true}),
		rawDoc(t, bson.M{"_id": primitive.NewObjectID(), "title": "Data Scientist Intern", "locations": bson.A{"VIC", "NSW"}}),
	}

	path, err := writeBackup(docs)
	if err != nil {
		t.Fatalf("writeBackup: %v", err)
	}

	file, err := os.Open(filepath.Join(dir, path))
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	// One document per line is what mongoimport expects.
	if len(lines) != len(docs) {
		t.Fatalf("expected %d lines, got %d", len(docs), len(lines))
	}

	var first bson.M
	if err := bson.UnmarshalExtJSON([]byte(lines[0]), true, &first); err != nil {
		t.Fatalf("line 1 is not valid extended JSON: %v", err)
	}

	if first["title"] != "Graduate Software Engineer" {
		t.Errorf("title did not survive the round trip: %v", first["title"])
	}
	// The _id has to survive as an ObjectID, or a restore would create new
	// documents rather than putting the originals back.
	if got, ok := first["_id"].(primitive.ObjectID); !ok || got != id {
		t.Errorf("_id did not round trip as an ObjectID: %#v", first["_id"])
	}
	if !strings.Contains(lines[0], "$oid") {
		t.Error("expected canonical extended JSON to encode the _id as $oid")
	}
}

func TestWriteBackupEmpty(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	path, err := writeBackup(nil)
	if err != nil {
		t.Fatalf("writeBackup with no documents should still succeed: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("expected an empty file, got %d bytes", info.Size())
	}
}

func TestLookupString(t *testing.T) {
	doc := rawDoc(t, bson.M{
		"title":    "Graduate Engineer",
		"company":  bson.M{"name": "Telstra"},
		"outdated": true,
	})

	if got := lookupString(doc, "title"); got != "Graduate Engineer" {
		t.Errorf("title = %q", got)
	}
	if got := lookupString(doc, "company", "name"); got != "Telstra" {
		t.Errorf("company.name = %q", got)
	}
	// Missing and wrongly-typed fields must degrade to an empty string rather
	// than panicking mid-report.
	if got := lookupString(doc, "nope"); got != "" {
		t.Errorf("missing field = %q, want empty", got)
	}
	if got := lookupString(doc, "outdated"); got != "" {
		t.Errorf("non-string field = %q, want empty", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 20); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncate("a much longer string", 6); got != "a much" {
		t.Errorf("got %q", got)
	}
}
