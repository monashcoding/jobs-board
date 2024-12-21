package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type JobType string

const (
	EOI        JobType = "EOI"
	FIRST_YEAR JobType = "FIRST_YEAR"
	INTERN     JobType = "INTERN"
	GRADUATE   JobType = "GRADUATE"
	UNKNOWN    JobType = "UNKNOWN"
)

type StudyField string

const (
	SOFTWARE      StudyField = "SOFTWARE"
	CYBERSECURITY StudyField = "CYBERSECURITY"
	DATA_SCIENCE  StudyField = "DATA_SCIENCE"
)

type LocationType string

const (
	VIC       LocationType = "VIC"
	NSW       LocationType = "NSW"
	QLD       LocationType = "QLD"
	WA        LocationType = "WA"
	NT        LocationType = "NT"
	SA        LocationType = "SA"
	HYBRID    LocationType = "HYBRID"
	REMOTE    LocationType = "REMOTE"
	AUSTRALIA LocationType = "AUSTRALIA"
)

type WorkingRight string

const (
	AUS_CITIZEN   WorkingRight = "AUS_CITIZEN"
	AUS_PR        WorkingRight = "AUS_PR"
	NZ_CITIZEN    WorkingRight = "NZ_CITIZEN"
	NZ_PR         WorkingRight = "NZ_PR"
	INTERNATIONAL WorkingRight = "INTERNATIONAL"
)

type Company struct {
	Name    string `bson:"name" json:"name"`
	Website string `bson:"website" json:"website"`
}

type Listing struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title          string             `bson:"title" json:"title"`
	Description    string             `bson:"description" json:"description"`
	Company        Company            `bson:"company" json:"company"`
	ApplicationURL string             `bson:"application_url" json:"application_url"`
	SourceURLs     []string           `bson:"source_urls" json:"source_urls"`
	Type           JobType            `bson:"type" json:"type"`
	CloseDate      *time.Time         `bson:"close_date,omitempty" json:"close_date"`
	Locations      []LocationType     `bson:"locations" json:"locations"`
	StudyFields    []StudyField       `bson:"study_fields" json:"study_fields"`
	StartDate      time.Time          `bson:"start_date" json:"start_date"`
	WorkingRights  []WorkingRight     `bson:"working_rights" json:"working_rights"`
	CreatedAt      time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time          `bson:"updated_at" json:"updated_at"`
}
