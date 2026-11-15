package utils

// Hard coded companies that have the same URL for all listings
func GetSameURLCompanyMap() map[string]bool {
	return sameURLCompany
}

var sameURLCompany = map[string]bool{
	"Ernst & Young": true,
	"Jane Street":   true,
	"CommBank":      true,
	"NAB":           true,
	"Australian Energy Market Operator (AEMO)": true,
}
