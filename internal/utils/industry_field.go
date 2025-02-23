package utils

import "github.com/monashcoding/mploy-golang-core/internal/models"

// GetIndustryFieldMap returns a map of company names to their industry fields
func GetIndustryFieldMap() map[string]models.IndustryField {
	return industryMap
}

var industryMap = map[string]models.IndustryField{
	"Jane Street":   models.QUANT_TRADING,
	"Ernst & Young": models.CONSULTING,
	"Amazon":        models.BIG_TECH,
}
