package steps

import (
	"testing"

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
