package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type JobType string

const (
	EOI        JobType = "EOI"
	FIRST_YEAR JobType = "FIRST_YEAR"
	INTERN     JobType = "INTERN"
	GRADUATE   JobType = "GRADUATE"
	OTHER			 JobType = "OTHER"
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
	HYBRID    WFHStatus = "HYBRID"
	REMOTE    WFHStatus = "REMOTE"
	OFFICE    WFHStatus = "OFFICE"
)

type WorkingRight string

const (
	AUS_CITIZEN    WorkingRight = "AUS_CITIZEN"
	AUS_PR         WorkingRight = "AUS_PR"
	NZ_CITIZEN     WorkingRight = "NZ_CITIZEN"
	NZ_PR          WorkingRight = "NZ_PR"
	INTERNATIONAL  WorkingRight = "INTERNATIONAL"
	WORK_VISA	   WorkingRight = "WORK_VISA"
	VISA_SPONSORED WorkingRight = "VISA_SPONSORED"
	OTHER_RIGHTS   WorkingRight = "OTHER_RIGHTS"
)

type Company struct {
	Name    string `bson:"name" json:"name"`
	Website *string `bson:"website" json:"website"`
}

type Listing struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title          string             `bson:"title" json:"title"`
	Description    *string             `bson:"description" json:"description"`
	Company        Company            `bson:"company" json:"company"`
	ApplicationURL *string             `bson:"application_url" json:"application_url"`
	WFHStatus			 *WFHStatus         `bson:"wfh_status" json:"wfh_status"`
	SourceURLs     []string           `bson:"source_urls" json:"source_urls"`
	Type           *JobType           `bson:"type" json:"type"`
	CloseDate      *time.Time         `bson:"close_date,omitempty" json:"close_date"`
	Locations      []LocationType     `bson:"locations" json:"locations"`
	StudyFields    []string			      `bson:"study_fields" json:"study_fields"`
	WorkingRights  []WorkingRight     `bson:"working_rights" json:"working_rights"`
	CreatedAt      time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time          `bson:"updated_at" json:"updated_at"`
}
