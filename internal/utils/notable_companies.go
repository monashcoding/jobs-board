package utils

import (
	"regexp"
	"strings"
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
// one narrows it; nothing else has to change.
var notableCompanies = map[string]bool{}

// notableCompanyNames is the source list, weighted towards where this audience
// actually applies: big tech, quantitative trading, banks, consulting and
// Australian technology. Engineering consultancies and mining houses are
// deliberately thin -- they advertise heavily (GHD, Stantec and AECOM are three
// of the largest sources of listings by volume) but their graduate programs are
// civil and mechanical, so they crowd out software roles without serving anyone.
//
// Entries are matched loosely: case, punctuation, parenthetical abbreviations
// and company suffixes are all ignored, so "Amazon Web Services (AWS)" matches
// "Amazon AU" matches "amazon".
var notableCompanyNames = []string{
	// --- Big tech and global product companies -------------------------------
	"Google", "Alphabet", "Microsoft", "Amazon", "Amazon Web Services", "AWS",
	"Apple", "Meta", "TikTok", "ByteDance", "Netflix", "NVIDIA", "IBM", "Oracle",
	"SAP", "Salesforce", "Adobe", "Cisco", "Intel", "AMD", "Qualcomm", "Arm",
	"LinkedIn", "Uber", "Airbnb", "Spotify", "Stripe", "Block", "Square",
	"PayPal", "Shopify", "Snowflake", "Databricks", "Palantir", "Datadog",
	"Twilio", "Zoom", "Dropbox", "GitHub", "GitLab", "Figma", "OpenAI",
	"Anthropic", "Canonical", "Red Hat", "VMware", "ServiceNow", "Workday",
	"MongoDB", "Elastic", "Confluent", "HashiCorp", "Akamai", "Bloomberg",
	"eBay", "Expedia", "Booking.com", "Mastercard", "Visa", "Tesla", "Tencent",
	"Samsung", "Sony", "Siemens", "Ericsson", "Dell", "Dell Technologies",
	"Hewlett Packard Enterprise", "Motorola Solutions", "Lenovo",

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

	// --- Banks, insurers, exchanges and financial services -------------------
	"CommBank", "Commonwealth Bank", "Westpac", "NAB", "National Australia Bank",
	"ANZ", "Macquarie Group", "Macquarie", "Suncorp Group", "Suncorp",
	"IAG", "Insurance Australia Group", "AMP", "QBE", "Allianz",
	"Bendigo Bank", "Bendigo and Adelaide Bank", "Reserve Bank of Australia",
	"ASX", "Australian Securities Exchange", "Goldman Sachs", "J.P. Morgan",
	"JPMorgan Chase", "Morgan Stanley", "UBS", "Bank of America", "HSBC",
	"Citi", "Citigroup", "Deutsche Bank", "BNP Paribas", "Nomura", "Jarden",
	"Barrenjoey", "Challenger", "Rabobank", "Colonial First State",
	"Afterpay", "Zip", "Tyro", "Judo Bank",

	// --- Consulting and professional services --------------------------------
	"Accenture", "Deloitte", "PwC", "KPMG", "Ernst & Young", "EY",
	"McKinsey & Company", "Boston Consulting Group", "Bain & Company",
	"Capgemini", "Infosys", "Tata Consultancy Services", "Wipro", "Cognizant",
	"Thoughtworks", "Mantel Group", "Kearney", "Oliver Wyman",
	"L.E.K. Consulting", "Nous Group", "Slalom", "Grant Thornton", "BDO",
	"RSM", "FTI Consulting", "Alvarez & Marsal", "KordaMentha",
	"Scyne Advisory", "Avanade", "DXC Technology", "NTT Data", "Fujitsu",
	"Protiviti",

	// --- Australian technology -----------------------------------------------
	"Atlassian", "Canva", "Airwallex", "WiseTech Global", "Xero", "SEEK",
	"REA Group", "CAR Group", "Carsales", "Domain Group", "TechnologyOne",
	"Altium", "Megaport", "NEXTDC", "Nearmap", "Culture Amp", "SafetyCulture",
	"Employment Hero", "Go1", "Linktree", "Immutable", "Quantium", "Deputy",
	"Envato", "Octopus Deploy", "Redbubble", "Rokt", "Airtasker", "Kogan",
	"Prospa", "Zeller", "Eucalyptus", "UpGuard", "DroneShield",
	"Gilmour Space Technologies", "Computershare", "IRESS", "Aristocrat",
	"Cochlear", "CSL", "ResMed", "Telix Pharmaceuticals",

	// --- Telco, retail and other major Australian employers ------------------
	"Telstra", "Optus", "TPG Telecom", "Vocus", "NBN Co", "Australia Post",
	"Woolworths Group", "Woolworths", "Coles Group", "Coles", "Wesfarmers",
	"Bunnings", "Qantas", "Virgin Australia", "Transurban",

	// --- Defence, government and research ------------------------------------
	"Australian Signals Directorate", "Australian Security Intelligence Organisation",
	"Department of Defence", "Defence Science and Technology Group", "CSIRO",
	"Data61", "BAE Systems", "Leidos", "Lockheed Martin", "Boeing", "Thales",
	"Northrop Grumman", "Raytheon", "Australian Taxation Office",
	"Department of Home Affairs", "Services Australia",
	"Australian Bureau of Statistics", "Australian Federal Police",
	"Australian Energy Market Operator", "AEMO",
	"Digital Transformation Agency", "Australian Securities and Investments Commission",

	// --- Cyber security ------------------------------------------------------
	"CrowdStrike", "Palo Alto Networks", "CyberCX", "Splunk", "Okta",
	"Cloudflare", "Fortinet", "SentinelOne", "Tenable", "Rapid7", "Wiz",
	"Zscaler", "Check Point Software Technologies", "Sophos",

	// --- Energy and resources ------------------------------------------------
	// Deliberately short. These are household names with real graduate
	// programs, but the audience is software, so the block is limited to the
	// largest rather than every miner that advertises.
	"BHP", "Rio Tinto", "Fortescue", "Woodside Energy", "Woodside", "Santos",
	"Origin Energy", "AGL Energy", "AGL",
}

// companyNoisePattern strips everything that is not alphanumeric, so casing,
// punctuation and spacing differences between sources collapse to one key.
var companyNoisePattern = regexp.MustCompile(`[^a-z0-9]+`)

// normaliseCompanyForMatching reduces a company name to a comparable key.
// Sources spell the same employer many ways ("Amazon Web Services (AWS)",
// "Amazon AU", "Thales Australia"), so matching the raw string misses most of
// them. StripCompanySuffix already handles the country and legal-entity
// suffixes that feed the listing fingerprint; this adds the rest.
func normaliseCompanyForMatching(name string) string {
	trimmed := StripCompanySuffix(strings.TrimSpace(name))
	lowered := strings.ToLower(trimmed)

	// Drop any parenthetical, which is usually an abbreviation of what precedes it.
	if open := strings.Index(lowered, "("); open != -1 {
		lowered = lowered[:open]
	}

	return companyNoisePattern.ReplaceAllString(lowered, "")
}

func init() {
	for _, name := range notableCompanyNames {
		if key := normaliseCompanyForMatching(name); key != "" {
			notableCompanies[key] = true
		}
	}
}

// IsNotableCompany reports whether a listing's employer is one the board covers.
func IsNotableCompany(name string) bool {
	key := normaliseCompanyForMatching(name)
	if key == "" {
		return false
	}
	return notableCompanies[key]
}
