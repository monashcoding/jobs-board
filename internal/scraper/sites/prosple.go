package sites

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"

	"github.com/monashcoding/mploy-golang-core/internal/scraper"
)

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

	// Set up the GraphQL query
	prospleApiUrl := "https://prosple-gw.global.ssl.fastly.net/internal"
	query := s.GetGraphQlQuery()
	variables := s.GetGraphQlVariables()

	// Send the GraphQL query
	response, err := s.SendGraphQLQuery(prospleApiUrl, query, variables)
	if err != nil {
		return nil, fmt.Errorf("GraphQL query failed: %v", err)
	}

	// Convert GraphQL response to models.Listing
	modelsListings := []models.Listing{}

	for _, opp := range response.Data.OpportunitiesSearch.Opportunities {
		model, err := s.OpportunityToModel(opp)
		if err != nil {
			fmt.Printf("Error converting opportunity to model: %v\n", err)
		} else {
			modelsListings = append(modelsListings, *model)
			fmt.Println("Model: ", model.CloseDate)
		}
	}

	return modelsListings, nil
}

func (s *ProspleScraper) OpportunityToModel(opportunity models.Opportunity) (*models.Listing, error) {
	sourceUrl := s.URL() + opportunity.DetailPageURl // Need to add base url to relative path

	company := models.Company{
		Name:    opportunity.ParentEmployer.Title,
		Website: s.URL() + opportunity.ParentEmployer.DetailPageUrl,
	}

	jobType := models.JobType(opportunity.OpportunityTypes[0].Label)

	parsedCloseDate, err := time.Parse("2006-01-02T15:04:05.000Z", opportunity.ApplicationsCloseDate)
	if err != nil {
		return nil, fmt.Errorf("error parsing close date: %v", err)
	}

	locations := []models.LocationType{} // TODO: Convert physicalLocations to LocationType

	studyfield := []models.StudyField{} // TODO: Convert studyFields to StudyField

	parsedStartDate := parsedCloseDate // TODO:: Convert use StartDate Struct

	workingRights := []models.WorkingRight{} // TODO: Convert workingRights to WorkingRight

	return &models.Listing{
		Title:          opportunity.Title,
		Description:    opportunity.Overview.FullText,
		Company:        company,
		ApplicationURL: opportunity.ApplyByUrl,
		SourceURLs:     []string{sourceUrl},
		Type:           jobType,
		CloseDate:      &parsedCloseDate,
		Locations:      locations,
		StudyFields:    studyfield,
		StartDate:      parsedStartDate,
		WorkingRights:  workingRights,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}, nil
}

func (s *ProspleScraper) SendGraphQLQuery(url string, query string, variables map[string]interface{}) (*models.GraphQLResponse, error) {

	// Prepare the request body
	requestBody := models.GraphQLRequest{
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
	var graphqlResp models.GraphQLResponse
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
