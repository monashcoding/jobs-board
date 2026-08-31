package steps

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/monashcoding/mploy-golang-core/internal/models"
)

func TestApplyClassification(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		industry   models.IndustryField
		jobType    models.JobType
		discipline models.Discipline
	}{
		{
			name:       "full three part response",
			content:    "BIG_TECH#GRADUATE#SOFTWARE",
			industry:   models.BIG_TECH,
			jobType:    models.GRADUATE,
			discipline: models.SOFTWARE,
		},
		{
			name:       "whitespace is tolerated",
			content:    " BANKS # INTERN # DATA ",
			industry:   models.BANKS,
			jobType:    models.INTERN,
			discipline: models.DATA,
		},
		{
			// A non-software engineering role still classifies, so it can be
			// kept and filtered on separately from software.
			name:       "non-software engineering",
			content:    "ENERGY#GRADUATE#ENGINEERING",
			industry:   models.ENERGY,
			jobType:    models.GRADUATE,
			discipline: models.ENGINEERING,
		},
		{
			name:       "non-technical role at a tech company",
			content:    "TECH#GRADUATE#OTHER_DISCIPLINE",
			industry:   models.TECH,
			jobType:    models.GRADUATE,
			discipline: models.OTHER_DISCIPLINE,
		},
		{
			// The model dropping the third field must not cost us the first two.
			name:       "legacy two part response keeps industry and job type",
			content:    "CONSULTING#INTERN",
			industry:   models.CONSULTING,
			jobType:    models.INTERN,
			discipline: models.OTHER_DISCIPLINE,
		},
		{
			name:       "unknown discipline falls back alone",
			content:    "GOVERNMENT#GRADUATE#BIOLOGY",
			industry:   models.GOVERNMENT,
			jobType:    models.GRADUATE,
			discipline: models.OTHER_DISCIPLINE,
		},
		{
			name:       "unknown industry falls back alone",
			content:    "AGRICULTURE#GRADUATE#SOFTWARE",
			industry:   models.OTHER_INDUSTRY,
			jobType:    models.GRADUATE,
			discipline: models.SOFTWARE,
		},
		{
			name:       "garbage response falls back on every axis",
			content:    "I'm sorry, I can't help with that.",
			industry:   models.OTHER_INDUSTRY,
			jobType:    models.OTHER,
			discipline: models.OTHER_DISCIPLINE,
		},
		{
			name:       "empty response falls back on every axis",
			content:    "",
			industry:   models.OTHER_INDUSTRY,
			jobType:    models.OTHER,
			discipline: models.OTHER_DISCIPLINE,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			industry, jobType, discipline := ApplyClassification(tc.content, "Test Listing")

			if industry != tc.industry {
				t.Errorf("industry = %q, want %q", industry, tc.industry)
			}
			if jobType != tc.jobType {
				t.Errorf("jobType = %q, want %q", jobType, tc.jobType)
			}
			if discipline != tc.discipline {
				t.Errorf("discipline = %q, want %q", discipline, tc.discipline)
			}
		})
	}
}

func TestCleanDescription(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips non breaking and narrow spaces",
			input: "Apply\u00a0by\u202f5pm",
			want:  "Apply by 5pm",
		},
		{
			name:  "strips zero width characters and BOM",
			input: "\ufeffGraduate\u200bProgram",
			want:  "Graduate Program",
		},
		{
			name:  "strips literal escaped newlines",
			input: `<p>Role\ndetails</p>`,
			want:  "<p>Roledetails</p>",
		},
		{
			name:  "leaves clean html untouched",
			input: "<p>Build things.</p>",
			want:  "<p>Build things.</p>",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanDescription(tc.input); got != tc.want {
				t.Errorf("CleanDescription(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// Descriptions that already carry HTML structure must not reach the API: that
// call was the largest single item on the OpenAI bill and it added nothing.
func TestFixDescriptionFormatSkipsApiCallForHtml(t *testing.T) {
	description := "<p>Work on distributed systems.</p>"
	listing := models.Listing{Title: "Software Engineer", Description: &description}

	step := NewAiParserStep()
	got, err := step.FixDescriptionFormat(context.Background(), listing)
	if err != nil {
		t.Fatalf("FixDescriptionFormat returned error: %v", err)
	}

	if *got.Description != "<p>Work on distributed systems.</p>" {
		t.Errorf("description not cleaned locally: %q", *got.Description)
	}
	if step.skippedJobs != 1 {
		t.Errorf("expected the API call to be skipped, skippedJobs = %d", step.skippedJobs)
	}
}

// A listing whose three classification axes are all known was resolved by an
// earlier run; title and company are part of the fingerprint, so the answer
// cannot have changed.
func TestGetIndustryFieldandTypeSkipsWhenFullyClassified(t *testing.T) {
	jobType := models.GRADUATE
	listing := models.Listing{
		Title:         "Software Engineer",
		IndustryField: models.BIG_TECH,
		Type:          &jobType,
		Discipline:    models.SOFTWARE,
	}

	step := NewAiParserStep()
	got, err := step.GetIndustryFieldandType(context.Background(), listing)
	if err != nil {
		t.Fatalf("GetIndustryFieldandType returned error: %v", err)
	}

	if got.IndustryField != models.BIG_TECH || got.Discipline != models.SOFTWARE {
		t.Errorf("cached classification was overwritten: %+v", got)
	}
	if step.skippedJobs != 1 {
		t.Errorf("expected the API call to be skipped, skippedJobs = %d", step.skippedJobs)
	}
}

func TestTruncateForPrompt(t *testing.T) {
	if got := truncateForPrompt("short", 100); got != "short" {
		t.Errorf("unexpected truncation: %q", got)
	}

	// Multi-byte runes must not be cut in half.
	got := truncateForPrompt("héllo wörld", 5)
	if len([]rune(got)) != 5 {
		t.Errorf("expected 5 runes, got %q", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncation produced invalid utf-8: %q", got)
	}
}
