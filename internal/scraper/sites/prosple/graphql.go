package prosple

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type GraphQLResponse struct {
	Data   ResponseData `json:"data"`
	Errors []Error      `json:"errors,omitempty"`
}

type ResponseData struct {
	OpportunitiesSearch SearchResult `json:"opportunitiesSearch"`
}

type SearchResult struct {
	Opportunities []Opportunity `json:"opportunities"`
}

type Opportunity struct {
	Title         string `json:"title"`
	DetailPageURl string `json:"detailPageUrl"`
	Overview      struct {
		FullText string `json:"fullText"`
	} `json:"overview"`
	ParentEmployer struct {
		Title         string `json:"title"`
		DetailPageUrl string `json:"detailPageUrl"`
	} `json:"parentEmployer"`
	ApplyByUrl            string `json:"applyByUrl"`
	ApplicationsCloseDate string `json:"applicationsCloseDate"`
	OpportunityTypes      []struct {
		Label string `json:"label"`
	} `json:"opportunityTypes"`
	StartDate struct {
		Category *struct {
			Label string `json:"label"`
		} `json:"category"`
	} `json:"startDate"`
	PhysicalLocations []struct {
		Label    string `json:"label"`
		Children []struct {
			Label string `json:"label"`
		} `json:"children"`
	} `json:"physicalLocations"`
	StudyFields []struct {
		Label string `json:"label"`
	} `json:"studyFields"`
	WorkingRights []struct {
		Label    string `json:"label"`
		Children []struct {
			Label string `json:"label"`
		} `json:"children"`
	} `json:"workingRights"`
}

type Error struct {
	Message string `json:"message"`
}

func SendGraphQLQuery(url string, query string, variables map[string]interface{}) (*GraphQLResponse, error) {
	requestBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %s, %v", resp.Status, string(body))
	}

	var graphqlResp GraphQLResponse
	if err := json.Unmarshal(body, &graphqlResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	if len(graphqlResp.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %v", graphqlResp.Errors)
	}

	return &graphqlResp, nil
}

func GetGraphQlVariables() map[string]interface{} {
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

func GetGraphQlQuery() string {
	return `query OpportunitiesSearch($parameters: OpportunitiesSearchInput!) {
  opportunitiesSearch(parameters: $parameters) {
    opportunities {
      id
      title
      applyByUrl
      overview {
        fullText
      }
      opportunityTypes {
        label
      }
      applicationsCloseDate
      locationDescription
      studyFields{
        label
      }
      physicalLocations{
        label
        children{
          label
        }
      }
      parentEmployer{
        title
      }
      workingRights{
        children{
          label
        }
      }
      }
    }
  }`
}
