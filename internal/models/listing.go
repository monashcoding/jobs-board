package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type JobType string

const (
	EOI             JobType = "EOI"
	PRE_PENULTIMATE JobType = "PRE_PENULTIMATE"
	INTERN          JobType = "INTERN"
	GRADUATE        JobType = "GRADUATE"
	OTHER           JobType = "OTHER"
)

type LocationType string

const (
	VIC       LocationType = "VIC"
	NSW       LocationType = "NSW"
	QLD       LocationType = "QLD"
	WA        LocationType = "WA"
	NT        LocationType = "NT"
	SA        LocationType = "SA"
	ACT       LocationType = "ACT"
	TAS       LocationType = "TAS"
	AUSTRALIA LocationType = "AUSTRALIA"
	OTHERS    LocationType = "OTHERS"
)

type WFHStatus string

const (
	HYBRID WFHStatus = "HYBRID"
	REMOTE WFHStatus = "REMOTE"
	OFFICE WFHStatus = "OFFICE"
)

type WorkingRight string

const (
	AUS_CITIZEN_PR WorkingRight = "AUS_CITIZEN_PR"
	NZ_CITIZEN_PR  WorkingRight = "NZ_CITIZEN_PR"
	INTERNATIONAL  WorkingRight = "INTERNATIONAL"
	OTHER_RIGHTS   WorkingRight = "OTHER_RIGHTS"
)

type Company struct {
	Name    string  `bson:"name" json:"name"`
	Website *string `bson:"website" json:"website"`
	Logo    *string `bson:"logo" json:"logo"`
}

type IndustryField string

const (
	CONSULTING         IndustryField = "CONSULTING"
	BANKS              IndustryField = "BANKS"
	BIG_TECH           IndustryField = "BIG_TECH"
	TECH               IndustryField = "TECH"
	QUANT_TRADING      IndustryField = "QUANT_TRADING"
	ENERGY             IndustryField = "ENERGY"
	GOVERNMENT         IndustryField = "GOVERNMENT"
	MANUFACTURING      IndustryField = "MANUFACTURING"
	TELECOMMUNICATIONS IndustryField = "TELECOMMUNICATIONS"
	RETAIL             IndustryField = "RETAIL"
	TRANSPORTATION     IndustryField = "TRANSPORTATION"
	OTHER_INDUSTRY     IndustryField = "OTHER_INDUSTRY"
)

// Discipline is the technical discipline of the role itself, as opposed to
// IndustryField (the employer's sector) or StudyFields (the degrees a listing
// is open to). Sources tag broad graduate programs with every discipline they
// accept, so StudyFields cannot tell a software role from a supply chain one.
type Discipline string

const (
	SOFTWARE         Discipline = "SOFTWARE"
	DATA             Discipline = "DATA"
	CYBER            Discipline = "CYBER"
	ENGINEERING      Discipline = "ENGINEERING"
	OTHER_DISCIPLINE Discipline = "OTHER_DISCIPLINE"
)

type Source string

const (
	GRADCONNECTION Source = "GRADCONNECTION"
	PROSPLE        Source = "PROSPLE"
	LINKEDIN       Source = "LINKEDIN"
	ATLASSIAN      Source = "ATLASSIAN"
	JANESTREET     Source = "JANESTREET"
	AIRWALLEX      Source = "AIRWALLEX"
	MULTIPLE       Source = "MULTIPLE"
)

type Listing struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title          string             `bson:"title" json:"title"`
	Description    *string            `bson:"description" json:"description"`
	OneLinerDesc   *string            `bson:"one_liner" json:"one_liner"`
	Company        Company            `bson:"company" json:"company"`
	ApplicationURL *string            `bson:"application_url" json:"application_url"`
	WFHStatus      *WFHStatus         `bson:"wfh_status" json:"wfh_status"`
	SourceURLs     []string           `bson:"source_urls" json:"source_urls"`
	Type           *JobType           `bson:"type" json:"type"`
	OpenDate       *time.Time         `bson:"open_date,omitempty" json:"open_date"`
	CloseDate      *time.Time         `bson:"close_date,omitempty" json:"close_date"`
	Locations      []LocationType     `bson:"locations" json:"locations"`
	Source         Source             `bson:"source" json:"source"`
	StudyFields    []string           `bson:"study_fields" json:"study_fields"`
	IndustryField  IndustryField      `bson:"industry_field" json:"industry_field"`
	Discipline     Discipline         `bson:"discipline" json:"discipline"`
	BoardScore     int                `bson:"board_score" json:"board_score"`
	BoardEligible  bool               `bson:"board_eligible" json:"board_eligible"`
	WorkingRights  []WorkingRight     `bson:"working_rights" json:"working_rights"`
	IsSponsored    bool               `bson:"is_sponsored" json:"is_sponsored"`
	CreatedAt      time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time          `bson:"updated_at" json:"updated_at"`
	Days_lived     int                `bson:"days_lived" json:"days_lived"`
}
