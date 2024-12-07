package sites

import (
	"fmt"
	"log"
	"strconv"
	"github.com/chromedp/chromedp"
	"github.com/gocolly/colly"
	"context"
	"github.com/monashcoding/mploy-golang-core/internal/models"
	"github.com/monashcoding/mploy-golang-core/internal/scraper"
)


type Posting struct {
	name            string
	description     string
	company         string
	application_url string
	source_url      string
	classification  string
	close_date      string
	location        string
	study_field     string
	start_date      string
	work_rights     string
}


func NewGradconScraper() *GradconScraper {
	return ScrapeGradConnection()
}

func (p Posting) String() string {
	return fmt.Sprintf("%s | %s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n...", p.company, p.name, p.description, p.classification, p.location, p.study_field, p.work_rights, p.close_date, p.start_date)
}

func scrapeIndividualJob(ctx context.Context, url string, postings []Posting) {
	fmt.Println("Visiting Job Page:", url)
	//initialising the new collector
	c := colly.NewCollector(
		colly.AllowedDomains("au.gradconnection.com"),
	)

	// extract job data
	c.OnHTML(".full-content-container.employer-content-container", func(e *colly.HTMLElement) {
		p := Posting{name: e.ChildText(".employers-profile-h1")}
		p.company = e.ChildText(".employers-panel-title")
		desc := e.ChildText(".campaign-content-container") // for now -> extract data from raw html with LLM later I think
		if len(desc) > 250 {
			p.description = desc[:250] + "..."
		} else {
			p.description = desc
		}
		p.application_url = "www.temp.com"
		p.source_url = "https://au.gradconnection.com" + url
		e.ForEach(".ellipsis-text-paragraph", func(i int, el *colly.HTMLElement) {
			// Based on the sequence, you can assign the categories
			if i == 0 {
				p.study_field = el.Text
			} else if i == 1 {
				// You can use i here to differentiate by position
				p.work_rights = el.Text
			} else if i == 2 {
				// Handle any other category based on sequence
				p.location = el.Text
			}
		})
		e.ForEach("li", func(i int, el *colly.HTMLElement) {
			// Based on the sequence, you can assign the categories
			if len(el.Text) > 13 && el.Text[:13] == "Closing Date:" {
				p.close_date = el.Text[13:]
			}
			if len(el.Text) > 9 && el.Text[:9] == "Job type:" {
				p.classification = el.Text[9:]
			}
			if len(el.Text) > 19 && el.Text[:19] == "POSITION START DATE" {
				p.start_date = el.Text[19:]
			}
		})

		postings = append(postings, p)
		fmt.Println(p)
	})

	c.Visit("https://au.gradconnection.com" + url)
}

func ScrapeGradConnection() []Posting {
	//collection of the job postings
	var postings []Posting
	maxPageNum := 0 // so we can stop paginating

	c := colly.NewCollector(
		colly.AllowedDomains("au.gradconnection.com"),
	)
	//logging in to grad connection
	er := c.Post("https://au.gradconnection.com/api/authenticate/", map[string]string{"email": "zelolox1234@gmail.com", "password": "12345@Abcd"})
	if er != nil {
		log.Fatal("Logged in failed", er)
	}

	// checking if login was sucessful
	c.OnResponse(func(r *colly.Response) {
		log.Println("Login Status: ", r.StatusCode)
	})

	// callback for pagination
	c.OnHTML(".rc-pagination", func(e *colly.HTMLElement) {
		if maxPageNum == 0 {
			// identify final page number
			// --> this only runs on the first visit
			e.ForEach("li", func(i int, child *colly.HTMLElement) {
				pageNum, err := strconv.Atoi(child.ChildText("a"))
				if err != nil {
					return // exit if the element doesnt contain a page number
				}
				if pageNum > maxPageNum {
					maxPageNum = pageNum // update max
				}
			})
			fmt.Println("Final Page:", maxPageNum)
		}
		// get current page number from url
		pageUrl := e.Request.URL // URL object
		q := pageUrl.Query()
		pageNum, err := strconv.Atoi(q.Get("page"))
		if err != nil {
			// default to 1 if it fails
			pageNum = 1
		}
		fmt.Println("Page Number:", pageNum)

		// traverse if we are not at the final page
		if pageNum != maxPageNum {
			nextPageNum := strconv.Itoa(pageNum + 1)
			q.Set("page", nextPageNum)
			pageUrl.RawQuery = q.Encode()
			c.Visit(pageUrl.String())
		}
	})

	// visit each job
	c.OnHTML(".campaign-listing-box", func(e *colly.HTMLElement) {
		url := e.ChildAttr(".box-header-title", "href")

		ctx, cancel := chromedp.NewContext(context.Background())
		defer cancel()
		scrapeIndividualJob(ctx, url, postings)
	})

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting: ", r.URL.String())
	})

	c.OnError(func(r *colly.Response, err error) {
		log.Println("Error: ", err)
	})

	//visiting grad connection
	err := c.Visit("https://au.gradconnection.com/internships/")
	if err != nil {
		log.Fatal(err)
	}

	return postings
}

