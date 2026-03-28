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
	OperationName string                 `json:"operationName"`
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
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
		Logo          struct {
			Thumbnail struct {
				Url string `json:"url"`
			} `json:"thumbnail"`
		} `json:"logo"`
	} `json:"parentEmployer"`
	ApplyByUrl            string `json:"applyByUrl"`
	ApplicationsOpenDate  string `json:"applicationsOpenDate"`
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

func SendGraphQLQuery(url string, variables map[string]interface{}, unblocker unblocker.Service) (*GraphQLResponse, error) {
	requestBody := GraphQLRequest{
		OperationName: "OpportunitiesSearch",
		Query:         GetGraphQlQuery(),
		Variables:     variables,
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
      applicationsOpenDate
      applicationsCloseDate
      locationDescription
      studyFields {
        label
      }
      physicalLocations {
        label
        children {
          label
        }
      }
      parentEmployer {
        title
        detailPageURL
        logo {
          thumbnail {
            url
          }
        }
      }
      workingRights {
        children {
          label
        }
      }
    }
  }
}`
}

func GetGraphQlVariables(offset int, limit int, field string) map[string]interface{} {
	return map[string]interface{}{
		"parameters": map[string]interface{}{
			"gid": "6",
			"range": map[string]interface{}{
				"offset": offset,
				"limit":  limit,
			},
			"sortBy": map[string]interface{}{
				"criteria":  "NEWEST_OPPORTUNITIES",
				"direction": "DESC",
			},
			"defaultBid":                  0,
			"selectedStartDateRangeFacet": nil,
			"experiments": map[string]interface{}{
				"sortByBidPopularityKeyword": "FIFTH_VARIATION",
				"prod2432":                   false,
			},
			"locationFilter": map[string]interface{}{
				"location": "Australia",
			},
			"unitOfDistance":    "KM",
			"workRightLocation": "29058", // Australia (all work rights)
			"selectedOpportunityTypeFacets": []map[string]interface{}{
				{"id": "1", "childFacets": []interface{}{}}, // Graduate Job or Program
				{"id": "2", "childFacets": []interface{}{}}, // Internship, Clerkship or Placement
				{"id": "9", "childFacets": []interface{}{}}, // Part-Time Student Job
			},
			"studyFieldsFilter": field, // 502 = IT & CS, 532 = Engineering, 677 = Maths
		},
	}
}
