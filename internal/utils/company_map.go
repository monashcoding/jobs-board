package utils

func GetCompanyMap() []struct {
	Name    string
	Matches []string
} {
	return companyMap
}

var companyMap = []struct {
	Name    string
	Matches []string
}{
	{
		Name: "Amazon",
		Matches: []string{
			"Amazon AU",
			"Amazon Australia",
		},
	},
	{
		Name: "Australian Security Intelligence Organisation",
		Matches: []string{
			"Australian Security Intelligence Organisation (ASIO)",
			"ASIO",
		},
	},
	{
		Name: "Ernst & Young",
		Matches: []string{
			"Ernst & Young",
			"EY",
		},
	},
	{
		Name: "Australian Government",
		Matches: []string{
			"Australian Government Graduate Program",
		},
	},
	{
		Name: "NAB",
		Matches: []string{
			"National Australia Bank",
		},
	},
	{
		Name: "CommBank",
		Matches: []string{
			"Commonwealth Bank",
			"CommBank",
			"CBA",
			"BankWest",
		},
	},
	{
		Name: "TikTok",
		Matches: []string{
			"TikTok Australia & New Zealand",
		},
	},
	{
		Name: "Sportsbet",
		Matches: []string{
			"Sportsbet Australia",
		},
	},
	{
		Name: "Macquarie Group",
		Matches: []string{
			"Macquarie Group",
			"Macquarie Technology Group",
		},
	},
	{
		Name: "IMC Trading",
		Matches: []string{
			"IMC Trading",
			"IMC Trading Australia",
		},
	},
}
