package sites

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/monashcoding/mploy-golang-core/internal/models"

	"github.com/monashcoding/mploy-golang-core/internal/scraper"
)

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}
type GraphQLResponse struct {
	Data struct {
		OpportunitiesSearch struct {
			ResultCount   int           `json:"resultCount"`
			Opportunities []Opportunity `json:"opportunities"`
		} `json:"opportunitiesSearch"`
	} `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

type Opportunity struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	ApplyByUrl            string `json:"applyByUrl"`
	DetailPageURl         string `json:"detailPageUrl"`
	ApplicationsCloseDate string `json:"applicationsCloseDate"`
	LocationDescription   string `json:"locationDescription"`

	Overview struct {
		Summary  string `json:"summary"`
		FullText string `json:"fullText"`
	} `json:"overview"`

	ParentEmployer struct {
		Title string `json:"title"`
	} `json:"parentEmployer"`

	OpportunityTypes []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	} `json:"opportunityTypes"`

	PhysicalLocations []struct {
		Id    string `json:"id"`
		Label string `json:"label"`

		Children []struct {
			Id    string `json:"id"`
			Label string `json:"label"`
		} `json:"children"`
	} `json:"physicalLocations"`

	StudyFields []struct {
		Id    string `json:"id"`
		Label string `json:"label"`

		Children []struct {
			Id    string `json:"id"`
			Label string `json:"label"`
		} `json:"children"`
	} `json:"studyFields"`

	StartDate struct {
		Category struct {
			Id    string `json:"id"`
			Label string `json:"label"`
		} `json:"category"`
	} `json:"startDate"`

	WorkingRights []struct {
		Id    string `json:"id"`
		Label string `json:"label"`

		Children []struct {
			Id    string `json:"id"`
			Label string `json:"label"`
		} `json:"children"`
	} `json:"workingRights"`
}

type GraphQLError struct {
	Message string `json:"message"`
}

type ProspleScraper struct {
	*scraper.BaseScraper
}

func NewProspleScraper() *ProspleScraper {
	return &ProspleScraper{
		BaseScraper: scraper.NewBaseScraper(),
	}
}

func (s *ProspleScraper) Name() string {
	return "Prosple"
}

func (s *ProspleScraper) URL() string {
	return "https://au.prosple.com/"
}

func (s *ProspleScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	fmt.Println("Scraping Prosple")

	prospleApiUrl := "https://prosple-gw.global.ssl.fastly.net/internal"
	query := s.GetGraphQlQuery()
	variables := s.GetGraphQlVariables()

	response, err := s.SendGraphQLQuery(prospleApiUrl, query, variables, "OpportunitiesSearch")

	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %v", err)
	}

	fmt.Printf("Found %d opportunities\n", response.Data.OpportunitiesSearch.ResultCount)

	for _, opp := range response.Data.OpportunitiesSearch.Opportunities {
		fmt.Printf("- Opportunity Summary: %.20s\n", opp.Overview.FullText)
	}

	return nil, nil
}

func (s *ProspleScraper) SendGraphQLQuery(url string, query string, variables map[string]interface{}, operationName string) (*GraphQLResponse, error) {

	// Prepare the request body
	requestBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	// Convert the request body to JSON
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Create a new HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set the headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %s, %v", resp.Status, string(body))
	}

	// Parse the response
	var graphqlResp GraphQLResponse
	if err := json.Unmarshal(body, &graphqlResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Check for GraphQL errors
	if len(graphqlResp.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %v", graphqlResp.Errors)
	}

	return &graphqlResp, nil
}

func (s *ProspleScraper) GetGraphQlVariables() map[string]interface{} {
	return map[string]interface{}{
		"parameters": map[string]interface{}{
			"gid": "6",
			"range": map[string]interface{}{
				"offset": 0,
				"limit":  20,
			},
			"sortBy": map[string]interface{}{
				"criteria":  "POPULARITY",
				"direction": "DESC",
			},
			"subscriptionTierEpicVariation": "THIRD_VARIATION",
			"keywords":                      "",
			"workRightLocation":             "29058",
			"experiments": map[string]interface{}{
				"bidKeyword": map[string]interface{}{
					"enabled": false,
				},
				"sortByBidPopularityKeyword": "SECOND_VARIATION",
			},
			"selectedStartDateRangeFacet": nil,
			"selectedLocationFacets": []map[string]interface{}{
				{
					"id":          "9692",
					"childFacets": []interface{}{},
				},
			},
		},
	}
}

func (s *ProspleScraper) GetGraphQlQuery() string {
	return `query OpportunitiesSearch($parameters: OpportunitiesSearchInput!) {
  opportunitiesSearch(parameters: $parameters) {
    resultCount
    opportunities {
      id
      groupContentId
      url
      title
      applyByUrl
      expired
      overview {
        summary
        fullText
      }
      opportunityTypes {
        id
        label
      }
      locationDescription
      applicationsOpenDate
      applicationsCloseDate
      }
    }
  }`
}
