package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/scraper"
)

type GradconScraper struct {
	*scraper.CollyScraper
}

func NewGradconScraper() *GradconScraper {
	return &GradconScraper{
		CollyScraper: scraper.NewCollyScraper(),
	}
}

func (s *GradconScraper) Name() string {
	return "GradConnections"
}

func (s *GradconScraper) URL() string {
	return "https://au.gradconnection.com/"
}

func (s *GradconScraper) Scrape(ctx context.Context) ([]models.Listing, error) {
	numItemsOnPage := 20
	num_pages := 2
	listings := []models.Listing{}
	//empListings := []tempListing{}
	for i := 0; i < num_pages; i++ {
		offset := i * numItemsOnPage
		url := fmt.Sprintf("https://au.gradconnection.com/api/campaignsearch/?limit=20&offset=%d&recommended=true", offset)
		fmt.Println(url)
		// Create a new HTTP client
		client := &http.Client{}

		// Create a new request
		req, err := http.NewRequest("GET", url, nil)

		if err != nil {
			log.Fatal("Error creating request:", err)
		}

		// Send the request
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal("Error making request:", err)
		}
		defer resp.Body.Close()

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal("Error reading response:", err)
		}
		var result []map[string]interface{}
		err = json.Unmarshal(body, &result)
		if err != nil {
			log.Fatal("Error unmarshalling response:", err)
		}
		for _, item := range result {
			if campaigns, ok := item["campaigns"].([]interface{}); ok {
				for _, campaign := range campaigns {
					if campaignMap, ok := campaign.(map[string]interface{}); ok {
						listing := models.Listing{}
						//tempListing := tempListing{}
						if title, ok := campaignMap["title"].(string); ok {
							listing.Title = title
							//tempListing.Title = title
						} else {
							fmt.Println("title key is missing or not a string")
						}
						if description, ok := campaignMap["description"].(string); ok {
							listing.Description = description
							//tempListing.Description = description
						} else {
							fmt.Println("description key is missing or not a string")
						}
						if companyMap, ok := item["customer_organization"].(map[string]interface{}); ok {
							if companyName, ok := companyMap["name"].(string); ok {
								listing.Company.Name = companyName
								listing.Company.Website = ""
								//tempListing.Company = companyName
							} else {
								fmt.Println("name key is missing or not a string in customer_organization")
							}
						} else {

							fmt.Println(campaignMap["customer_organization"])
							fmt.Println("customer_organization key is missing or not a map")
						}
						if applicationURL, ok := campaignMap["target_url"].(string); ok {
							listing.ApplicationURL = applicationURL
							//tempListing.ApplicationURL = applicationURL
						} else {
							fmt.Println("target_url key is missing or not a string")
						}
						if originTargetURL, ok := campaignMap["origin_target_url"].(string); ok {

							listing.SourceURLs = append(listing.SourceURLs, originTargetURL) //originTargetURL
							//tempListing.SourceURLs = originTargetURL
						} else {
							fmt.Println("origin_target_url key is missing or not a string")
						}
						listing.Type = "Internship"
						if locations, ok := campaignMap["locations"].([]interface{}); ok {
							for _, loc := range locations {
								listing.Locations = append(listing.Locations, loc.(string))
								//tempListing.Locations = append(tempListing.Locations, loc.(string))

							}
						} else {
							fmt.Println("locations key is missing or not a slice")
						}
						if disciplines, ok := campaignMap["disciplines"].([]interface{}); ok {

							for _, d := range disciplines {
								if discipline, ok := d.(string); ok {
									listing.StudyFields = append(listing.StudyFields, discipline)
									//tempListing.StudyFields = append(tempListing.StudyFields, discipline)
								}
							}
						} else {
							fmt.Println("disciplines key is missing or not a slice")
						}
						listing.CreatedAt = time.Now()
						//tempListing.CreatedAt = time.Now().Format("2006-01-02")
						if closeDate, ok := item["earliest_closing_date"]; ok {
							closeDateStr, ok := closeDate.(string)

							parsedTime, _ := time.Parse(time.RFC3339, closeDateStr)
							if !ok {
								log.Println("earliest_closing_date is not a string")
							} else {
								listing.CloseDate = &parsedTime
								//tempListing.CloseDate = closeDateStr
							}
						} else {
							log.Println("earliest_closing_date is missing")
						}

						if intervalMap, ok := campaignMap["interval"].(map[string]interface{}); ok {
							if start, ok := intervalMap["start"].(string); ok {
								// Use the start value as needed
								parsedStart, _ := time.Parse(time.RFC3339, start)
								listing.StartDate = parsedStart
								//tempListing.StartDate = start
							} else {
								fmt.Println("start key is missing or not a string")
							}
						} else {
							fmt.Println("interval key is missing or not a map")
						}
						if workRights, ok := campaignMap["work_rights"].([]interface{}); ok {
							for _, right := range workRights {
								if rightStr, ok := right.(string); ok {
									// Process each working right as a string
									listing.WorkingRights = append(listing.WorkingRights, rightStr)
									//tempListing.WorkingRights = append(tempListing.WorkingRights, rightStr)
								} else {
									log.Println("A work right is not a string")
								}
							}
						} else {
							log.Println("work_rights key is missing or not a slice")
						}
						listing.UpdatedAt = time.Now()
						//tempListing.UpdatedAt = time.Now().Format("2006-01-02")
						//tempListings = append(tempListings, tempListing)
						listings = append(listings, listing)
					} else {
						fmt.Println("Campaign is not a map")
					}
				}
			} else {
				fmt.Println("Campaigns not found or is not a slice")
			}
		}

	}
	fmt.Println(listings)
	return listings, nil
}
