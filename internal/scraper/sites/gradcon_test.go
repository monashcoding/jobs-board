package sites

import (
	scrapertesting "github.com/monashcoding/mploy-golang-core/internal/scraper/testing"
	"testing"
)

func TestGradconScraper_Scrape(t *testing.T) {
	scraper := NewGradconScraper()
	suite := scrapertesting.NewScraperTestSuite(t, scraper, "testdata/gradcon")

	testCases := []scrapertesting.TestCase{
		{
			Name:          "successful_scrape",
			FixtureFolder: "success",
			ExpectedFile:  "success/expected.json",
			FixtureMapping: map[string]string{
				"https://au.gradconnection.com/": "success/main_page.html",
			},
		},
	}

	for _, tc := range testCases {
		suite.RunTestCase(tc)
	}
}
