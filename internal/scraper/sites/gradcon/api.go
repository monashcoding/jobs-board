package gradcon

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type APIResponse = []GradConnectData

type GradConnectData struct {
	Campaigns            []GradConnectCampaign           `json:"campaigns"`
	CustomerOrganisation GradConnectCustomerOrganisation `json:"customer_organization"`
	EarliestClosingDate  string                          `json:"earliest_closing_date"`
}

type GradConnectCampaign struct {
	Id          string `json:"id"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	JobType     interface{} `json:"job_type"` 
	TargetURL   string `json:"target_url"`
	Interval    struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"interval"`
	Disciplines      []string                     `json:"disciplines"`
	Locations        []string                     `json:"locations"`
	WorkRights       []string                     `json:"work_rights"`
	OriginTargetURL  string                       `json:"origin_target_url"`
	AdditionalFields []GradConnectAdditionalField `json:"additional_fields"`
}

type GradConnectAdditionalField struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type GradConnectCustomerOrganisation struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func SendAPIRequest(url string, limit int, offset int) (*APIResponse, error) {
	params := "?disciplines=computer-science&limit=" + strconv.Itoa(limit) + "&offset=" + strconv.Itoa(offset) + "&recommended=true"
	apiUrl := url + params

	// Create a new HTTP client
	client := &http.Client{}

	// Create a new request
	req, err := http.NewRequest("GET", apiUrl, nil)

	if err != nil {
		utils.Fatal("Error creating request: %v", err)
	}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		utils.Fatal("Error making request:", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Fatal("Error reading response:", err)
	}

	utils.Log("API response successfully received [\u2713]")

	var result APIResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		utils.Warn("Error parsing response body: %v", err)
		return &result, err
	}

	utils.Log("Reponse body successfully parsed [\u2713]")

	return &result, nil
}
