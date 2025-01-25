package utils

import (
	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// GetIndustryMap returns the mapping of industries to companies.
func GetIndustryMap() map[models.IndustryField][]string {
	return map[models.IndustryField][]string{
		models.CONSULTING: {
			"Ernst & Young",
			"Bain & Company",
			"Boston Consulting Group",
		},
		models.BANKS: {
			"Commonwealth Bank",
			"NAB",
			"Westpac Group",
		},
		models.BIG_TECH: {
			"Google",
			"Microsoft",
			"Amazon",
			"Facebook",
			"Atlassian",
		},
		models.TECH: {
			"Tesla Australia",
			"TikTok",
		},
		models.QUANT_TRADING: {
			"Jane Street",
			"Two Sigma",
			"Renaissance Technologies",
		},
		models.OTHER_INDUSTRY: {
			"Other Company",
		},
	}
}
