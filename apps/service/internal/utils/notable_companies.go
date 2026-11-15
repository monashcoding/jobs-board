package utils

import (
	"regexp"
	"strings"
)

// Company tiers rank an employer by how much its name alone carries a message.
// They order the Discord bot's weekly recap, which shows only a handful of the
// week's postings and has to choose which. Whether a listing appears at all is
// still IsNotableCompany's decision; this is a separate axis on top of it.
//
// The values are strings rather than ints so they read the same in Mongo, in a
// log line and here. Consumers that need an ordering map them to one; inserting
// a tier later then costs nothing, where renumbering would touch every stored
// document.
const (
	TierHeadline = "headline"
	TierMajor    = "major"
	// TierUnranked is what CompanyTier reports for an employer that carries no
	// tier. It should not reach a board-eligible listing -- the two tier lists
	// partition the notable list, and IsNotableCompany is a hard gate -- but the
	// field is always present so no consumer has to guess.
	TierUnranked = "unranked"
)

// notableCompanies are the employers the Discord job board covers: the ones
// students recognise and actually apply to.
//
// This is the hand-maintained company list that board scoring previously did
// without, approximating recognition from the employer's industry instead. That
// approximation was too coarse to control volume: roughly 385 listings cleared
// the other gates, well past what a forum should carry, and the surplus was
// mostly unrecognisable employers, recruiters and placement agencies.
//
// Editing this list is how you tune the board. Adding a name widens it, removing
// one narrows it; nothing else has to change. The list is split across
// headlineCompanyNames and majorCompanyNames, which together are the whole of
// it -- add a name to whichever tier it belongs in.
var notableCompanies = map[string]bool{}

// companyTiers maps a normalised company key to its tier.
var companyTiers = map[string]string{}

// headlineCompanyNames are the employers whose name alone makes someone open the
// message: big tech, the quant trading firms (small by headcount, but the single
// strongest draw for this audience), the bulge-bracket banks and Macquarie, MBB,
// and the Australian names with the same pull.
//
// Entries are matched loosely: case, punctuation, parenthetical abbreviations
// and company suffixes are all ignored, so "Amazon Web Services (AWS)" matches
// "Amazon AU" matches "amazon".
var headlineCompanyNames = []string{
	// --- Big tech and global product companies -------------------------------
	"Google", "Alphabet", "Microsoft", "Amazon", "Amazon Web Services", "AWS",
	"Apple", "Meta", "TikTok", "ByteDance", "Netflix", "NVIDIA", "IBM", "Oracle",
	"SAP", "Salesforce", "Adobe", "Cisco", "Intel", "AMD", "Qualcomm",
	"LinkedIn", "Uber", "Airbnb", "Spotify", "Stripe", "Block", "Square",
	"PayPal", "Shopify", "Snowflake", "Databricks", "Palantir", "Datadog",
	"GitHub", "Figma", "OpenAI", "Anthropic", "MongoDB", "Cloudflare",
	"Bloomberg", "Tesla", "Tencent", "Samsung", "Sony",

	// --- Quantitative trading and market making ------------------------------
	"Optiver", "Jane Street", "IMC Trading", "IMC",
	"Susquehanna International Group", "Susquehanna", "SIG", "Citadel",
	"Citadel Securities", "Jump Trading", "Akuna Capital",
	"Tower Research Capital", "Virtu Financial", "Two Sigma", "DRW",
	"Hudson River Trading", "Vivienne Court Trading", "VivCourt",
	"Eclipse Trading", "Millennium", "Point72", "Squarepoint Capital",
	"Maven Securities", "XTX Markets", "Qube Research and Technologies", "QRT",
	"Flow Traders", "Old Mission Capital", "Five Rings", "Radix Trading",
	"WorldQuant", "AQR Capital Management", "Man Group", "Balyasny",
	"Schonfeld", "Vatic Labs", "Headlands Technologies", "Wolverine Trading",
	"Belvedere Trading", "PEAK6",

	// --- Investment banks and strategy consulting ----------------------------
	"Goldman Sachs", "J.P. Morgan", "JPMorgan Chase", "Morgan Stanley",
	"Macquarie Group", "Macquarie", "UBS", "Citi", "Citigroup",
	"Bank of America", "McKinsey & Company", "Boston Consulting Group",
	"Bain & Company",

	// --- Australian technology with national name recognition ----------------
	"Atlassian", "Canva", "Airwallex", "WiseTech Global", "Xero", "SEEK",
	"REA Group", "Culture Amp", "SafetyCulture", "Linktree", "Immutable",
	"Quantium",

	// --- Australian household names outside tech -----------------------------
	"CommBank", "Commonwealth Bank", "Telstra", "Qantas", "Woolworths Group",
	"Woolworths", "Coles Group", "Coles", "BHP", "Rio Tinto", "CSIRO",
	"Australian Signals Directorate", "CSL", "Cochlear", "ResMed",
}

// majorCompanyNames are the rest of the board: still notable, still worth
// posting, just not what should win one of the recap's few slots when a
// headline name is competing for it.
var majorCompanyNames = []string{
	// --- Remaining global technology -----------------------------------------
	"Arm", "Twilio", "Zoom", "Dropbox", "GitLab", "Canonical", "Red Hat",
	"VMware", "ServiceNow", "Workday", "Elastic", "Confluent", "HashiCorp",
	"Akamai", "eBay", "Expedia", "Booking.com", "Mastercard", "Visa",
	"Siemens", "Ericsson", "Dell", "Dell Technologies",
	"Hewlett Packard Enterprise", "Motorola Solutions", "Lenovo",

	// --- Banks, insurers, exchanges and financial services -------------------
	"Westpac", "NAB", "National Australia Bank", "ANZ", "Suncorp Group",
	"Suncorp", "IAG", "Insurance Australia Group", "AMP", "QBE", "Allianz",
	"Bendigo Bank", "Bendigo and Adelaide Bank", "Reserve Bank of Australia",
	"ASX", "Australian Securities Exchange", "HSBC", "Deutsche Bank",
	"BNP Paribas", "Nomura", "Jarden", "Barrenjoey", "Challenger", "Rabobank",
	"Colonial First State", "Afterpay", "Zip", "Tyro", "Judo Bank",

	// --- Consulting and professional services --------------------------------
	"Accenture", "Deloitte", "PwC", "KPMG", "Ernst & Young", "EY",
	"Capgemini", "Infosys", "Tata Consultancy Services", "Wipro", "Cognizant",
	"Thoughtworks", "Mantel Group", "Kearney", "Oliver Wyman",
	"L.E.K. Consulting", "Nous Group", "Slalom", "Grant Thornton", "BDO",
	"RSM", "FTI Consulting", "Alvarez & Marsal", "KordaMentha",
	"Scyne Advisory", "Avanade", "DXC Technology", "NTT Data", "Fujitsu",
	"Protiviti",

	// --- Remaining Australian technology -------------------------------------
	"CAR Group", "Carsales", "Domain Group", "TechnologyOne", "Altium",
	"Megaport", "NEXTDC", "Nearmap", "Employment Hero", "Go1", "Deputy",
	"Envato", "Octopus Deploy", "Redbubble", "Rokt", "Airtasker", "Kogan",
	"Prospa", "Zeller", "Eucalyptus", "UpGuard", "DroneShield",
	"Gilmour Space Technologies", "Computershare", "IRESS", "Aristocrat",
	"Telix Pharmaceuticals",

	// --- Telco, retail and other major Australian employers ------------------
	"Optus", "TPG Telecom", "Vocus", "NBN Co", "Australia Post", "Wesfarmers",
	"Bunnings", "Virgin Australia", "Transurban",

	// --- Defence, government and research ------------------------------------
	"Australian Security Intelligence Organisation", "Department of Defence",
	"Defence Science and Technology Group", "Data61", "BAE Systems", "Leidos",
	"Lockheed Martin", "Boeing", "Thales", "Northrop Grumman", "Raytheon",
	"Australian Taxation Office", "Department of Home Affairs",
	"Services Australia", "Australian Bureau of Statistics",
	"Australian Federal Police", "Australian Energy Market Operator", "AEMO",
	"Digital Transformation Agency",
	"Australian Securities and Investments Commission",

	// --- Cyber security ------------------------------------------------------
	"CrowdStrike", "Palo Alto Networks", "CyberCX", "Splunk", "Okta",
	"Fortinet", "SentinelOne", "Tenable", "Rapid7", "Wiz", "Zscaler",
	"Check Point Software Technologies", "Sophos",

	// --- Energy and resources ------------------------------------------------
	// Deliberately short. These are household names with real graduate
	// programs, but the audience is software, so the block is limited to the
	// largest rather than every miner that advertises.
	"Fortescue", "Woodside Energy", "Woodside", "Santos", "Origin Energy",
	"AGL Energy", "AGL",
}

// companyNoisePattern strips everything that is not alphanumeric, so casing,
// punctuation and spacing differences between sources collapse to one key.
var companyNoisePattern = regexp.MustCompile(`[^a-z0-9]+`)

// companyConnectorPattern matches a connector left dangling once the suffix
// behind it is stripped: "Commonwealth Bank of Australia" loses "Australia" and
// is left with a trailing "of", so it would never match the bare
// "Commonwealth Bank" that sources also use.
//
// This is deliberately confined to matching. StripCompanySuffix feeds both the
// stored company name and the fingerprint, so stripping connectors there would
// rename employers on the public job board and re-key their documents; here it
// only decides which names are the same employer.
var companyConnectorPattern = regexp.MustCompile(`(?i)\s+(of|the|and|&)$`)

// normaliseCompanyForMatching reduces a company name to a comparable key.
// Sources spell the same employer many ways ("Amazon Web Services (AWS)",
// "Amazon AU", "Thales Australia"), so matching the raw string misses most of
// them. StripCompanySuffix already handles the country and legal-entity
// suffixes that feed the listing fingerprint; this adds the rest.
func normaliseCompanyForMatching(name string) string {
	trimmed := StripCompanySuffix(strings.TrimSpace(name))

	// Alternating, because one exposes the other: stripping "Australia" leaves
	// the "of" behind it, and stripping a connector can expose another suffix.
	for {
		stripped := strings.TrimSpace(companyConnectorPattern.ReplaceAllString(trimmed, ""))
		if stripped == trimmed || stripped == "" {
			break
		}
		trimmed = StripCompanySuffix(stripped)
	}

	lowered := strings.ToLower(trimmed)

	// Drop any parenthetical, which is usually an abbreviation of what precedes it.
	if open := strings.Index(lowered, "("); open != -1 {
		lowered = lowered[:open]
	}

	return companyNoisePattern.ReplaceAllString(lowered, "")
}

func init() {
	register := func(names []string, tier string) {
		for _, name := range names {
			key := normaliseCompanyForMatching(name)
			if key == "" {
				continue
			}
			notableCompanies[key] = true
			// The first tier to claim a name keeps it, so a company listed in
			// both blocks ranks by the better of the two.
			if _, seen := companyTiers[key]; !seen {
				companyTiers[key] = tier
			}
		}
	}

	register(headlineCompanyNames, TierHeadline)
	register(majorCompanyNames, TierMajor)
}

// IsNotableCompany reports whether a listing's employer is one the board covers.
func IsNotableCompany(name string) bool {
	key := normaliseCompanyForMatching(name)
	if key == "" {
		return false
	}
	return notableCompanies[key]
}

// CompanyTier reports how prominent an employer is, for ordering the board's
// weekly recap. IsNotableCompany still decides whether it appears at all.
func CompanyTier(name string) string {
	if tier, ok := companyTiers[normaliseCompanyForMatching(name)]; ok {
		return tier
	}
	return TierUnranked
}
