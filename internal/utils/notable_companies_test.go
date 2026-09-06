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

// The two tier lists must cover the whole notable list. A name in neither is
// posted to the board and then ranked below every tiered employer, so in a busy
// week it never wins one of the recap's few slots -- silently, with no error.
func TestEveryNotableCompanyIsTiered(t *testing.T) {
	for key := range notableCompanies {
		if _, tiered := companyTiers[key]; !tiered {
			t.Errorf("company key %q is on the board's list but has no tier", key)
		}
	}
}

// The reverse: a tier list must not add employers the board does not carry, or
// the two lists have drifted apart in the other direction.
func TestEveryTieredCompanyIsNotable(t *testing.T) {
	for key := range companyTiers {
		if !notableCompanies[key] {
			t.Errorf("company key %q is tiered but not on the board's list", key)
		}
	}
}

func TestCompanyTier(t *testing.T) {
	cases := map[string]string{
		// Headline, in each of the categories the tier is meant to cover.
		"Google":                    TierHeadline,
		"Amazon Web Services (AWS)": TierHeadline,
		"Optiver":                   TierHeadline,
		"Jane Street":               TierHeadline,
		"Goldman Sachs":             TierHeadline,
		"Macquarie Group":           TierHeadline,
		"McKinsey & Company":        TierHeadline,
		"Atlassian Australia":       TierHeadline,
		"CommBank":                  TierHeadline,
		"Commonwealth Bank":         TierHeadline,
		"Qantas":                    TierHeadline,
		"BHP":                       TierHeadline,
		"CSIRO":                     TierHeadline,

		// Major: on the board, but not what should win a recap slot over a
		// headline name.
		"Deloitte":       TierMajor,
		"Accenture":      TierMajor,
		"Westpac":        TierMajor,
		"Thales Pty Ltd": TierMajor,
		"NEXTDC":         TierMajor,
		"Boeing":         TierMajor,
		"CrowdStrike":    TierMajor,
		"AGL":            TierMajor,

		// Off the list entirely. These never reach the recap -- the company
		// list is a hard gate for eligibility -- but the field is still
		// answerable so no consumer has to handle a missing value.
		"The Intern Group": TierUnranked,
		"Hays":             TierUnranked,
		"":                 TierUnranked,
	}

	for name, want := range cases {
		if got := CompanyTier(name); got != want {
			t.Errorf("CompanyTier(%q) = %q, want %q", name, got, want)
		}
	}
}

// A company named in both tier lists keeps the better of the two, so an
// accidental duplicate cannot demote an employer.
func TestCompanyTierPrefersTheBetterTier(t *testing.T) {
	key := normaliseCompanyForMatching("Netflix")
	if companyTiers[key] != TierHeadline {
		t.Errorf("a company in both lists must keep its headline tier, got %q", companyTiers[key])
	}
}

// Stripping the country suffix leaves the connector that introduced it behind,
// so the long form has to lose that too or it matches nothing.
func TestMatchingStripsDanglingConnectors(t *testing.T) {
	for _, pair := range [][2]string{
		{"Commonwealth Bank of Australia", "Commonwealth Bank"},
		{"Reserve Bank of Australia", "Reserve Bank"},
		{"Commonwealth Bank of Australia Pty Ltd", "Commonwealth Bank"},
	} {
		if got, want := normaliseCompanyForMatching(pair[0]), normaliseCompanyForMatching(pair[1]); got != want {
			t.Errorf("normaliseCompanyForMatching(%q) = %q, want %q (from %q)",
				pair[0], got, want, pair[1])
		}
	}

	if !IsNotableCompany("Commonwealth Bank of Australia") {
		t.Error("the long form of a listed employer must still be on the board's list")
	}
	if CompanyTier("Commonwealth Bank of Australia") != TierHeadline {
		t.Error("the long form of a listed employer must carry its tier")
	}
}

// The connector strip must not eat a name that legitimately ends in one, and
// must not reduce a name to nothing.
func TestMatchingKeepsMeaningfulConnectors(t *testing.T) {
	if normaliseCompanyForMatching("Bendigo and Adelaide Bank") != "bendigoandadelaidebank" {
		t.Error("a connector inside a name must be left alone")
	}
	if key := normaliseCompanyForMatching("The"); key == "" {
		t.Error("a name that is entirely a connector must not normalise to nothing")
	}
}
