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
	ID             uint32     `json:"id"`
	RecordType     NullString `json:"record_type"`
	Action         NullString `json:"action"`
	ActionDate     NullString `json:"action_date"`
	Gender         NullString `json:"gender"`
	ActiveStatus   NullString `json:"active_status"`
	Deceased       NullString `json:"deceased"`
	ProfileNotes   NullString `json:"profile_notes"`
	CustomListID   *uint32    `json:"custom_list_id,omitempty"`
	CustomListName string     `json:"custom_list_name,omitempty"`
	Source         string     `json:"source"`

	Names        []SanctionsName              `json:"names,omitempty"`
	Dates        []SanctionsDate              `json:"dates,omitempty"`
	Countries    []SanctionsCountry           `json:"countries,omitempty"`
	Images       []SanctionsImage             `json:"images,omitempty"`
	Descriptions []SanctionsDescriptionDetail `json:"descriptions,omitempty"`
	Associations []SanctionsAssociationDetail `json:"associations,omitempty"`
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
	AssociateID   uint32     `json:"associate_id"`
	AssociateName string     `json:"associate_name"`
	Relationship  NullString `json:"relationship"`
	IsEx          bool       `json:"is_ex"`
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
	Name           string `json:"name"`
	SearchType     string `json:"search_type"`
	MinScore       int    `json:"min_score"`
	IncludeNotes   bool   `json:"include_notes"`
	IncludeDetails bool   `json:"include_details"`

	// Optional secondary identifiers. They adjust the score of records already
	// found by name and never affect which records are retrieved. Omitting
	// them screens exactly as before.
	DateOfBirth string       `json:"date_of_birth,omitempty"`
	Citizenship StringOrList `json:"citizenship,omitempty"`
}

type BatchScreenRequest struct {
	Names          []string `json:"names"`
	SearchType     string   `json:"search_type"`
	MinScore       int      `json:"min_score"`
	IncludeNotes   bool     `json:"include_notes"`
	IncludeDetails bool     `json:"include_details"`

	DateOfBirth string       `json:"date_of_birth,omitempty"`
	Citizenship StringOrList `json:"citizenship,omitempty"`
}

// StringOrList accepts either a single value or an array, so a caller with one
// citizenship does not have to wrap it in an array.
type StringOrList []string

func (s *StringOrList) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		if single == "" {
			*s = nil
		} else {
			*s = StringOrList{single}
		}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*s = StringOrList(many)
	return nil
}

type BatchScreenResult struct {
	Query    string         `json:"query"`
	MinScore int            `json:"min_score"`
	Total    int            `json:"total"`
	Results  []ScreenResult `json:"results"`
	Error    string         `json:"error,omitempty"`
}

type BatchScreenResponse struct {
	Total   int                 `json:"total"`
	Results []BatchScreenResult `json:"results"`
}

type ScreenResult struct {
	Record         SanctionsRecord `json:"record"`
	Score          int             `json:"score"`
	MatchedName    string          `json:"matched_name"`
	IsCustomList   bool            `json:"is_custom_list"`
	CustomListName string          `json:"custom_list_name,omitempty"`

	// NameScore is the score before secondary identifiers were applied,
	// present only when the caller supplied one. Score carries the adjusted
	// figure, so the two differ exactly by the sum of the adjustments in
	// MatchFactors.
	NameScore    *int          `json:"name_score,omitempty"`
	MatchFactors *MatchFactors `json:"match_factors,omitempty"`

	// ShadowScore is the candidate scorer's verdict on the same record,
	// present only while SCREEN_SHADOW_SCORING is on. It is reported for
	// comparison and has no effect on Score, on ordering, or on which records
	// appear at all.
	ShadowScore       *int   `json:"shadow_score,omitempty"`
	ShadowMatchedName string `json:"shadow_matched_name,omitempty"`
}

// MatchFactor records what one secondary identifier contributed, so a reviewer
// can see why a score moved rather than only that it did.
type MatchFactor struct {
	Status      string `json:"status"`
	Adjustment  int    `json:"adjustment"`
	RecordValue string `json:"record_value,omitempty"`
}

type MatchFactors struct {
	DOB         *MatchFactor `json:"dob,omitempty"`
	Citizenship *MatchFactor `json:"citizenship,omitempty"`
}

type ScreenResponse struct {
	Query    string         `json:"query"`
	MinScore int            `json:"min_score"`
	Total    int            `json:"total"`
	Results  []ScreenResult `json:"results"`
}

type PaginatedResponse struct {
	Page    int         `json:"page"`
	PerPage int         `json:"per_page"`
	Total   int         `json:"total"`
	Data    interface{} `json:"data"`
}

// Custom list upload types

type CustomListEntry struct {
	RecordType  string            `json:"record_type"`
	Gender      string            `json:"gender,omitempty"`
	FirstName   string            `json:"first_name,omitempty"`
	MiddleName  string            `json:"middle_name,omitempty"`
	Surname     string            `json:"surname,omitempty"`
	EntityName  string            `json:"entity_name,omitempty"`
	DateOfBirth string            `json:"date_of_birth,omitempty"`
	Nationality string            `json:"nationality,omitempty"`
	IDType      string            `json:"id_type,omitempty"`
	IDValue     string            `json:"id_value,omitempty"`
	Notes       string            `json:"notes,omitempty"`
	Aliases     []CustomListAlias `json:"aliases,omitempty"`
}

type CustomListAlias struct {
	FirstName  string `json:"first_name,omitempty"`
	MiddleName string `json:"middle_name,omitempty"`
	Surname    string `json:"surname,omitempty"`
	EntityName string `json:"entity_name,omitempty"`
}

type CustomListUploadRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Entries     []CustomListEntry `json:"entries"`
}

type CustomListUploadResponse struct {
	ListID       uint32 `json:"list_id"`
	Name         string `json:"name"`
	EntriesAdded int    `json:"entries_added"`
	Message      string `json:"message"`
}

type CustomListSummary struct {
	ID          uint32 `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	EntryCount  int    `json:"entry_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type CustomListDeleteResponse struct {
	Message string `json:"message"`
}

// Seed history (GET /api/historical_updates)

type SeedRunChange struct {
	ChangeType string `json:"change_type"`
	RecordType string `json:"record_type,omitempty"`
	Count      int    `json:"count"`
}

type SeedRunCountry struct {
	Type string `json:"type,omitempty"`
	Code string `json:"code,omitempty"`
	Name string `json:"name,omitempty"`
}

type SeedRunRecordChange struct {
	RecordID     uint32           `json:"record_id"`
	ChangeType   string           `json:"change_type"`
	RecordType   string           `json:"record_type,omitempty"`
	ActiveStatus string           `json:"active_status,omitempty"`
	DisplayName  string           `json:"display_name,omitempty"`
	DateOfBirth  string           `json:"date_of_birth,omitempty"`
	Countries    []SeedRunCountry `json:"countries,omitempty"`
}

type HistoricalUpdateEntry struct {
	ID                    uint64                `json:"id"`
	SeededAt              string                `json:"seeded_at"`
	CompletedAt           string                `json:"completed_at,omitempty"`
	Status                string                `json:"status"`
	IntervalSincePrevious string                `json:"interval_since_previous,omitempty"`
	IntervalHours         *float64              `json:"interval_hours,omitempty"`
	Changes               []SeedRunChange       `json:"changes"`
	TotalRecordsAffected  int                   `json:"total_records_affected"`
	RecordChanges         []SeedRunRecordChange `json:"record_changes,omitempty"`
	RecordChangesTotal    int                   `json:"record_changes_total,omitempty"`
}

type HistoricalUpdatesResponse struct {
	Page    int                     `json:"page"`
	PerPage int                     `json:"per_page"`
	Total   int                     `json:"total"`
	Data    []HistoricalUpdateEntry `json:"data"`
}
