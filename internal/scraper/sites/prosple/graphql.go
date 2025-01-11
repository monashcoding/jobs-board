package prosple

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/monashcoding/mploy-golang-core/internal/unblocker"

	"github.com/monashcoding/mploy-golang-core/internal/utils"
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
	DetailPageUrl string `json:"detailPageURL"`
	Overview      struct {
		FullText string `json:"fullText"`
	} `json:"overview"`
	ParentEmployer struct {
		Title         string `json:"title"`
		DetailPageUrl string `json:"detailPageURL"`
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

func SendGraphQLQuery(url string, query string, variables map[string]interface{}, unblocker unblocker.Service) (*GraphQLResponse, error) {
	requestBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		utils.Log("Failed to marshal request body: %v", err)
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		utils.Log("Failed to create request: %v", err)
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Use unblocker instead of http.Client
	resp, err := unblocker.MakeRequest(context.Background(), req)
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

	utils.Success("GraphQL response received.")

	var graphqlResp GraphQLResponse
	if err := json.Unmarshal(body, &graphqlResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	if len(graphqlResp.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %v", graphqlResp.Errors)
	}

	utils.Success("Parsed %d jobs.", len(graphqlResp.Data.OpportunitiesSearch.Opportunities))

	return &graphqlResp, nil
}

func GetGraphQlVariables(offset int, limit int) map[string]interface{} {
	return map[string]interface{}{
		"parameters": map[string]interface{}{
			"gid": "6",
			"range": map[string]interface{}{
				"offset": offset,
				"limit":  limit,
			},
			"sortBy": map[string]interface{}{
				"criteria":  "NEWEST_OPPORTUNITIES", // NEWEST_OPPORTUNITIES, POPULARITY, CLOSING_DATE
				"direction": "DESC",
			},
			"subscriptionTierEpicVariation": "THIRD_VARIATION",
			"keywords":                      "",
			"experiments": map[string]interface{}{
				"bidKeyword": map[string]interface{}{
					"enabled": false,
				},
				"sortByBidPopularityKeyword": "SECOND_VARIATION",
			},
			"selectedStartDateRangeFacet": nil,
			"selectedLocationFacets": []map[string]interface{}{
				{
					"id":          "9692", // 9692 = Australia
					"childFacets": []interface{}{},
				},
			},
			"selectedStudyFieldFacets": []map[string]interface{}{
				{
					"id": "502", // 502 = IT & Computer Science
				},
			},
			"selectedWorkRightsType": []map[string]interface{}{
				{"id": "29059"}, // 29059 = Australian Citizen
				{"id": "29060"}, // 29060 = Australian Permanent Resident
				{"id": "29061"}, // 29061 = International Student/Graduate Visa
				{"id": "29062"}, // 29062 = Australian Work Visa (All Other)
				{"id": "29063"}, // 29063 = Visa Sponsorship Included
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
			detailPageURL
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
				detailPageURL
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
