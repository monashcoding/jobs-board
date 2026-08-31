package utils

import "testing"

func TestStripCompanySuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Real pairs from production where both forms appear and previously
		// fingerprinted as separate employers.
		{"Thales Australia", "Thales"},
		{"TikTok Australia", "TikTok"},
		{"Deloitte Australia", "Deloitte"},
		{"Leidos Australia", "Leidos"},
		{"Amazon Australia and New Zealand", "Amazon"},
		{"Datacom Australia & New Zealand", "Datacom"},
		{"Linfox ANZ", "Linfox"},
		{"Amazon AU", "Amazon"},
		{"Dematic Australia Pty Ltd", "Dematic"},
		{"Some Company Pty Limited", "Some Company"},
		{"Another Company Limited", "Another Company"},

		// "Group" is part of the canonical name for these employers; stripping
		// it would rename them on the public board, not just deduplicate them.
		{"Macquarie Group", "Macquarie Group"},
		{"Coles Group", "Coles Group"},
		{"FDM Group", "FDM Group"},

		{"Atlassian", "Atlassian"},
		{"  Optiver  ", "Optiver"},
		{"", ""},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := StripCompanySuffix(tc.in); got != tc.want {
				t.Errorf("StripCompanySuffix(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripCompanySuffixDoesNotMatchMidName(t *testing.T) {
	// Only a trailing suffix should be removed, never an interior word.
	cases := []string{
		"Australia Post",
		"Australian Taxation Office",
		"AU Digital Services",
	}

	for _, name := range cases {
		if got := StripCompanySuffix(name); got != name {
			t.Errorf("StripCompanySuffix(%q) = %q, want it unchanged", name, got)
		}
	}
}
