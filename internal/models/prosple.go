package models

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
		Title         string `json:"title"`
		DetailPageUrl string `json:"detailPageUrl"`
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
