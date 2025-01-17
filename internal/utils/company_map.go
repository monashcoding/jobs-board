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
		Name: "Commonwealth Bank",
		Matches: []string{
			"Commonwealth Bank",
			"CommBank",
			"CBA",
		},
	},
}