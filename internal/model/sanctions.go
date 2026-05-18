package model

import "database/sql"

type SanctionsRecord struct {
	ID           uint32         `json:"id"`
	RecordType   sql.NullString `json:"record_type"`
	Action       sql.NullString `json:"action"`
	ActionDate   sql.NullString `json:"action_date"`
	Gender       sql.NullString `json:"gender"`
	ActiveStatus sql.NullString `json:"active_status"`
	Deceased     sql.NullString `json:"deceased"`
	ProfileNotes sql.NullString `json:"profile_notes"`

	Names     []SanctionsName    `json:"names,omitempty"`
	Dates     []SanctionsDate    `json:"dates,omitempty"`
	Countries []SanctionsCountry `json:"countries,omitempty"`
	Images    []SanctionsImage   `json:"images,omitempty"`
}

type SanctionsName struct {
	ID                 uint64         `json:"id"`
	RecordID           uint32         `json:"record_id"`
	NameType           sql.NullString `json:"name_type"`
	TitleHonorific     sql.NullString `json:"title_honorific"`
	FirstName          sql.NullString `json:"first_name"`
	MiddleName         sql.NullString `json:"middle_name"`
	Surname            sql.NullString `json:"surname"`
	MaidenName         sql.NullString `json:"maiden_name"`
	Suffix             sql.NullString `json:"suffix"`
	SingleStringName   sql.NullString `json:"single_string_name"`
	OriginalScriptName sql.NullString `json:"original_script_name"`
	EntityName         sql.NullString `json:"entity_name"`
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
	ID       uint64         `json:"id"`
	RecordID uint32         `json:"record_id"`
	DateType sql.NullString `json:"date_type"`
	Day      sql.NullString `json:"day"`
	Month    sql.NullString `json:"month"`
	Year     sql.NullString `json:"year"`
	Note     sql.NullString `json:"note"`
}

type SanctionsCountry struct {
	ID          uint64         `json:"id"`
	RecordID    uint32         `json:"record_id"`
	CountryType sql.NullString `json:"country_type"`
	CountryCode sql.NullString `json:"country_code"`
}

type SanctionsImage struct {
	ID       uint64         `json:"id"`
	RecordID uint32         `json:"record_id"`
	URL      sql.NullString `json:"url"`
}

type SanctionsDescription struct {
	ID             uint64 `json:"id"`
	RecordID       uint32 `json:"record_id"`
	Description1ID *uint16 `json:"description1_id"`
	Description2ID *uint16 `json:"description2_id"`
	Description3ID *uint16 `json:"description3_id"`
}

type SanctionsRole struct {
	ID         uint64         `json:"id"`
	RecordID   uint32         `json:"record_id"`
	RoleType   sql.NullString `json:"role_type"`
	OccCatCode *uint16        `json:"occ_cat_code"`
	Title      sql.NullString `json:"title"`
	SinceDay   sql.NullString `json:"since_day"`
	SinceMonth sql.NullString `json:"since_month"`
	SinceYear  sql.NullString `json:"since_year"`
	ToDay      sql.NullString `json:"to_day"`
	ToMonth    sql.NullString `json:"to_month"`
	ToYear     sql.NullString `json:"to_year"`
}

type SanctionsBirthPlace struct {
	ID       uint64         `json:"id"`
	RecordID uint32         `json:"record_id"`
	Place    sql.NullString `json:"place"`
}

type SanctionsRef struct {
	ID              uint64  `json:"id"`
	RecordID        uint32  `json:"record_id"`
	SanctionsRefID  *uint32 `json:"sanctions_ref_id"`
	SinceDay        sql.NullString `json:"since_day"`
	SinceMonth      sql.NullString `json:"since_month"`
	SinceYear       sql.NullString `json:"since_year"`
	ToDay           sql.NullString `json:"to_day"`
	ToMonth         sql.NullString `json:"to_month"`
	ToYear          sql.NullString `json:"to_year"`
}

type SanctionsIDNumber struct {
	ID       uint64         `json:"id"`
	RecordID uint32         `json:"record_id"`
	IDType   sql.NullString `json:"id_type"`
	IDValue  sql.NullString `json:"id_value"`
	IDNotes  sql.NullString `json:"id_notes"`
}

type SanctionsSource struct {
	ID       uint64         `json:"id"`
	RecordID uint32         `json:"record_id"`
	URL      sql.NullString `json:"url"`
}

type SanctionsAddress struct {
	ID             uint64         `json:"id"`
	RecordID       uint32         `json:"record_id"`
	AddressLine    sql.NullString `json:"address_line"`
	AddressCity    sql.NullString `json:"address_city"`
	AddressCountry sql.NullString `json:"address_country"`
	URL            sql.NullString `json:"url"`
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
