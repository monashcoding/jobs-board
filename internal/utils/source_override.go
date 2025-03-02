package utils

import (
	"github.com/monashcoding/mploy-golang-core/internal/models"
)

// Hard coded companies that have the same URL for all listings
func GetSourceOverrideMap() map[string]models.Source {
	return sourceOverrideMap
}

var sourceOverrideMap = map[string]models.Source{
	"Macquarie Group": models.GRADCONNECTION,
}
