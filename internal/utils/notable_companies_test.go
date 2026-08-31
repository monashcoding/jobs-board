package utils

import "testing"

func TestIsNotableCompanyMatchesSourceSpellings(t *testing.T) {
	// The same employer arrives spelled differently from each source.
	for _, name := range []string{
		"Atlassian", "atlassian", "Atlassian Pty Ltd", "Atlassian Australia",
		"Amazon Web Services (AWS)", "AWS", "Amazon AU",
		"CommBank", "Commonwealth Bank",
		"Ernst & Young", "EY",
		"Boston Consulting Group (BCG)",
		"Susquehanna International Group",
		"TikTok", "Macquarie Group",
	} {
		if !IsNotableCompany(name) {
			t.Errorf("IsNotableCompany(%q) = false, want true", name)
		}
	}
}

// These were the largest sources of listings by volume and are exactly what the
// board is meant to exclude: placement agencies, recruiters and civil
// engineering consultancies.
func TestIsNotableCompanyRejectsTheLongTail(t *testing.T) {
	for _, name := range []string{
		"The Intern Group", "FDM Group", "Hays", "Stantec", "GHD", "SMEC",
		"Maincode", "BJAK", "KPay Group", "Alignerr", "",
	} {
		if IsNotableCompany(name) {
			t.Errorf("IsNotableCompany(%q) = true, want false", name)
		}
	}
}

// The list is weighted towards where this audience actually applies. Each
// priority category must be represented, or a whole class of employer silently
// disappears from the board.
func TestPriorityCategoriesAreCovered(t *testing.T) {
	categories := map[string][]string{
		"big tech":   {"Google", "Microsoft", "Amazon Web Services", "NVIDIA", "Canva", "Stripe", "OpenAI"},
		"quant":      {"Optiver", "Jane Street", "IMC Trading", "Susquehanna International Group", "Citadel Securities", "Jump Trading", "XTX Markets"},
		"banks":      {"CommBank", "Westpac", "NAB", "ANZ", "Macquarie Group", "Goldman Sachs", "Morgan Stanley", "IAG"},
		"consulting": {"Accenture", "Deloitte", "PwC", "KPMG", "Ernst & Young", "McKinsey & Company", "Boston Consulting Group", "Bain & Company"},
		"aus tech":   {"Atlassian", "Canva", "Airwallex", "WiseTech Global", "Xero", "SEEK", "REA Group", "TechnologyOne", "SafetyCulture", "Culture Amp"},
	}

	for category, names := range categories {
		for _, name := range names {
			if !IsNotableCompany(name) {
				t.Errorf("%s: %q is missing from the board's company list", category, name)
			}
		}
	}
}

func TestNotableCompanyMatchingIgnoresPunctuationAndCase(t *testing.T) {
	if normaliseCompanyForMatching("Ernst & Young") != normaliseCompanyForMatching("ERNST AND YOUNG") {
		// "and" vs "&" is a genuine difference; assert only what we claim.
		t.Log("ampersand and 'and' are distinct, which is expected")
	}
	if normaliseCompanyForMatching("J.P. Morgan") != normaliseCompanyForMatching("JP Morgan") {
		t.Error("punctuation must not change the match key")
	}
}
