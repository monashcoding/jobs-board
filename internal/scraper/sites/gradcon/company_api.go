package gradcon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/monashcoding/mploy-golang-core/internal/unblocker"
	"github.com/monashcoding/mploy-golang-core/internal/utils"
)

type DescriptionAPIResponse = []GradConnectDecriptionData

type GradConnectDecriptionData struct {
	Id      string `json:"id"`
	Slug    string `json:"slug"`
	Content string `json:"content"`
}

func COMPANY_API_URL() string {
	return "https://au.gradconnection.com/api/campaigns/"
}

func SendCompanyAPIRequest(companyId string, unblocker unblocker.Service) (*DescriptionAPIResponse, error) {
	params := "?customer_organization=" + companyId + "&active=True" + "&state=published" + "&limit=100"
	apiUrl := COMPANY_API_URL() + params

	// Create a new request
	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := unblocker.MakeRequest(context.Background(), req)
	if err != nil {
		log.Fatal("Error making request:", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Fatal("Error reading response: %v", err)
	}

	var result DescriptionAPIResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		utils.Warn("Error parsing response body: %v", err)
		return &result, err
	}

	return &result, nil
}
