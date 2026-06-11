package model

import (
	"database/sql"
	"encoding/json"
)

// NullString wraps sql.NullString to serialize as a plain string or null in JSON,
// instead of the default {"String":"...","Valid":true} format.
type NullString struct {
	sql.NullString
}

func (ns NullString) MarshalJSON() ([]byte, error) {
	if !ns.Valid {
		return json.Marshal(nil)
	}
	return json.Marshal(ns.String)
}

func (ns *NullString) UnmarshalJSON(data []byte) error {
	var s *string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s != nil {
		ns.Valid = true
		ns.String = *s
	} else {
		ns.Valid = false
		ns.String = ""
	}
	return nil
}

type SanctionsRecord struct {
	ID           uint32     `json:"id"`
	RecordType   NullString `json:"record_type"`
	Action       NullString `json:"action"`
	ActionDate   NullString `json:"action_date"`
	Gender       NullString `json:"gender"`
	ActiveStatus NullString `json:"active_status"`
	Deceased     NullString `json:"deceased"`
	ProfileNotes NullString `json:"profile_notes"`

	Names        []SanctionsName               `json:"names,omitempty"`
	Dates        []SanctionsDate               `json:"dates,omitempty"`
	Countries    []SanctionsCountry            `json:"countries,omitempty"`
	Images       []SanctionsImage              `json:"images,omitempty"`
	Descriptions []SanctionsDescriptionDetail  `json:"descriptions,omitempty"`
	Associations []SanctionsAssociationDetail  `json:"associations,omitempty"`
}

type SanctionsName struct {
	ID                 uint64     `json:"id"`
	RecordID           uint32     `json:"record_id"`
	NameType           NullString `json:"name_type"`
	TitleHonorific     NullString `json:"title_honorific"`
	FirstName          NullString `json:"first_name"`
	MiddleName         NullString `json:"middle_name"`
	Surname            NullString `json:"surname"`
	MaidenName         NullString `json:"maiden_name"`
	Suffix             NullString `json:"suffix"`
	SingleStringName   NullString `json:"single_string_name"`
	OriginalScriptName NullString `json:"original_script_name"`
	EntityName         NullString `json:"entity_name"`
}

func (n *SanctionsName) DisplayName() string {
	if n.EntityName.Valid && n.EntityName.String != "" {
		return n.EntityName.String
	}
	if n.SingleStringName.Valid && n.SingleStringName.String != "" {
		return n.SingleStringName.String
	}
	name := ""
	if n.FirstName.Valid {
		name = n.FirstName.String
	}
	if n.MiddleName.Valid && n.MiddleName.String != "" {
		name += " " + n.MiddleName.String
	}
	if n.Surname.Valid && n.Surname.String != "" {
		name += " " + n.Surname.String
	}
	return name
}

type SanctionsDate struct {
	ID       uint64     `json:"id"`
	RecordID uint32     `json:"record_id"`
	DateType NullString `json:"date_type"`
	Day      NullString `json:"day"`
	Month    NullString `json:"month"`
	Year     NullString `json:"year"`
	Note     NullString `json:"note"`
}

type SanctionsCountry struct {
	ID          uint64     `json:"id"`
	RecordID    uint32     `json:"record_id"`
	CountryType NullString `json:"country_type"`
	CountryCode NullString `json:"country_code"`
	CountryName NullString `json:"country_name"`
}

type SanctionsImage struct {
	ID       uint64     `json:"id"`
	RecordID uint32     `json:"record_id"`
	URL      NullString `json:"url"`
}

type SanctionsDescription struct {
	ID             uint64  `json:"id"`
	RecordID       uint32  `json:"record_id"`
	Description1ID *uint16 `json:"description1_id"`
	Description2ID *uint16 `json:"description2_id"`
	Description3ID *uint16 `json:"description3_id"`
}

type SanctionsDescriptionDetail struct {
	Description1 NullString `json:"description1"`
	Description2 NullString `json:"description2"`
	Description3 NullString `json:"description3"`
}

type SanctionsAssociationDetail struct {
	AssociateID      uint32     `json:"associate_id"`
	AssociateName    string     `json:"associate_name"`
	Relationship     NullString `json:"relationship"`
	IsEx             bool       `json:"is_ex"`
}

type SanctionsRole struct {
	ID         uint64     `json:"id"`
	RecordID   uint32     `json:"record_id"`
	RoleType   NullString `json:"role_type"`
	OccCatCode *uint16    `json:"occ_cat_code"`
	Title      NullString `json:"title"`
	SinceDay   NullString `json:"since_day"`
	SinceMonth NullString `json:"since_month"`
	SinceYear  NullString `json:"since_year"`
	ToDay      NullString `json:"to_day"`
	ToMonth    NullString `json:"to_month"`
	ToYear     NullString `json:"to_year"`
}

type SanctionsBirthPlace struct {
	ID       uint64     `json:"id"`
	RecordID uint32     `json:"record_id"`
	Place    NullString `json:"place"`
}

type SanctionsRef struct {
	ID             uint64     `json:"id"`
	RecordID       uint32     `json:"record_id"`
	SanctionsRefID *uint32    `json:"sanctions_ref_id"`
	SinceDay       NullString `json:"since_day"`
	SinceMonth     NullString `json:"since_month"`
	SinceYear      NullString `json:"since_year"`
	ToDay          NullString `json:"to_day"`
	ToMonth        NullString `json:"to_month"`
	ToYear         NullString `json:"to_year"`
}

type SanctionsIDNumber struct {
	ID       uint64     `json:"id"`
	RecordID uint32     `json:"record_id"`
	IDType   NullString `json:"id_type"`
	IDValue  NullString `json:"id_value"`
	IDNotes  NullString `json:"id_notes"`
}

type SanctionsSource struct {
	ID       uint64     `json:"id"`
	RecordID uint32     `json:"record_id"`
	URL      NullString `json:"url"`
}

type SanctionsAddress struct {
	ID             uint64     `json:"id"`
	RecordID       uint32     `json:"record_id"`
	AddressLine    NullString `json:"address_line"`
	AddressCity    NullString `json:"address_city"`
	AddressCountry NullString `json:"address_country"`
	URL            NullString `json:"url"`
}

type SanctionsAssociation struct {
	ID               uint64  `json:"id"`
	RecordID         uint32  `json:"record_id"`
	AssociateID      uint32  `json:"associate_id"`
	RelationshipCode *uint16 `json:"relationship_code"`
	IsEx             bool    `json:"is_ex"`
}

// API request/response types

type ScreenRequest struct {
	Name       string `json:"name"`
	SearchType string `json:"search_type"`
	MinScore   int    `json:"min_score"`
}

type ScreenResult struct {
	Record      SanctionsRecord `json:"record"`
	Score       int             `json:"score"`
	MatchedName string          `json:"matched_name"`
}

type ScreenResponse struct {
	Query      string         `json:"query"`
	MinScore   int            `json:"min_score"`
	Total      int            `json:"total"`
	Results    []ScreenResult `json:"results"`
}

type PaginatedResponse struct {
	Page    int         `json:"page"`
	PerPage int         `json:"per_page"`
	Total   int         `json:"total"`
	Data    interface{} `json:"data"`
}
