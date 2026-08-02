package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/nnn/sanctions-service/internal/model"
)

type RecordsHandler struct {
	db *sql.DB
}

func NewRecordsHandler(db *sql.DB) *RecordsHandler {
	return &RecordsHandler{db: db}
}

type recordListFilters struct {
	firstName    string
	lastName     string
	recordType   string
	activeStatus string
}

func (f recordListFilters) needsNameJoin() bool {
	return f.firstName != "" || f.lastName != ""
}

// likePrefix builds a prefix pattern for LIKE, escaping wildcards in the caller's
// input so a supplied % or _ cannot change what matches or turn the query back
// into an unindexed leading-wildcard scan.
func likePrefix(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 1)
	for _, r := range value {
		if r == '\\' || r == '%' || r == '_' {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}

func buildRecordListQuery(f recordListFilters) (fromClause, whereClause string, args []interface{}) {
	var conditions []string

	if f.recordType != "" {
		conditions = append(conditions, "sr.record_type = ?")
		args = append(args, f.recordType)
	}
	if f.activeStatus != "" {
		conditions = append(conditions, "sr.active_status = ?")
		args = append(args, f.activeStatus)
	}
	// Prefix matching so the indexes from migration 008 can serve these as range
	// scans. A leading wildcard would make them unusable and scan every name row.
	// Use POST /api/screen for fuzzy or mid-word matching.
	if f.firstName != "" {
		conditions = append(conditions, "sn.first_name LIKE ?")
		args = append(args, likePrefix(f.firstName))
	}
	if f.lastName != "" {
		conditions = append(conditions, "sn.surname LIKE ?")
		args = append(args, likePrefix(f.lastName))
	}

	fromClause = "FROM sanctions_records sr"
	if f.needsNameJoin() {
		fromClause += " INNER JOIN sanctions_names sn ON sn.record_id = sr.id AND sn.name_type = 'Primary Name'"
	}
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}
	return fromClause, whereClause, args
}

func (h *RecordsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 25
	}
	offset := (page - 1) * perPage

	filters := recordListFilters{
		firstName:    strings.TrimSpace(q.Get("first_name")),
		lastName:     strings.TrimSpace(q.Get("last_name")),
		recordType:   q.Get("record_type"),
		activeStatus: q.Get("active_status"),
	}

	fromClause, whereClause, args := buildRecordListQuery(filters)

	// sr.id is the primary key, so rows can only be duplicated when the name
	// join multiplies them. Deduplicating otherwise costs a temporary table for
	// nothing.
	countExpr, distinct := "COUNT(*)", ""
	if filters.needsNameJoin() {
		countExpr, distinct = "COUNT(DISTINCT sr.id)", "DISTINCT "
	}

	var total int
	countQuery := "SELECT " + countExpr + " " + fromClause + " " + whereClause
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "count query failed")
		return
	}

	dataQuery := "SELECT " + distinct + "sr.id, sr.record_type, sr.action, sr.action_date, sr.gender, sr.active_status, sr.deceased, sr.custom_list_id, COALESCE(cl.name, '') " +
		fromClause + " LEFT JOIN custom_lists cl ON cl.id = sr.custom_list_id " + whereClause + " ORDER BY sr.id LIMIT ? OFFSET ?"

	dataArgs := make([]interface{}, 0, len(args)+2)
	dataArgs = append(dataArgs, args...)
	dataArgs = append(dataArgs, perPage, offset)

	rows, err := h.db.Query(dataQuery, dataArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	records := make([]model.SanctionsRecord, 0, perPage)
	for rows.Next() {
		var rec model.SanctionsRecord
		var customListID sql.NullInt64
		var listName string
		if err := rows.Scan(&rec.ID, &rec.RecordType, &rec.Action, &rec.ActionDate, &rec.Gender, &rec.ActiveStatus, &rec.Deceased, &customListID, &listName); err != nil {
			continue
		}
		applyCustomListMeta(&rec, customListID, listName)
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	h.attachPrimaryNames(records)

	writeJSON(w, model.PaginatedResponse{
		Page:    page,
		PerPage: perPage,
		Total:   total,
		Data:    records,
	})
}

func (h *RecordsHandler) Show(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var rec model.SanctionsRecord
	var customListID sql.NullInt64
	var listName string
	err = h.db.QueryRow(`
		SELECT sr.id, sr.record_type, sr.action, sr.action_date, sr.gender, sr.active_status,
		       sr.deceased, sr.profile_notes, sr.custom_list_id, COALESCE(cl.name, '')
		FROM sanctions_records sr
		LEFT JOIN custom_lists cl ON cl.id = sr.custom_list_id
		WHERE sr.id = ?
	`, id).Scan(&rec.ID, &rec.RecordType, &rec.Action, &rec.ActionDate, &rec.Gender, &rec.ActiveStatus, &rec.Deceased, &rec.ProfileNotes, &customListID, &listName)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	applyCustomListMeta(&rec, customListID, listName)

	h.loadNames(&rec)
	h.loadDates(&rec)
	h.loadCountries(&rec)
	h.loadImages(&rec)
	h.loadDescriptions(&rec)
	h.loadAssociations(&rec)

	writeJSON(w, rec)
}

// attachPrimaryNames fills in one primary name per record using a single query
// rather than one lookup per row.
func (h *RecordsHandler) attachPrimaryNames(records []model.SanctionsRecord) {
	if len(records) == 0 {
		return
	}

	ids := make([]uint32, len(records))
	for i := range records {
		ids[i] = records[i].ID
	}
	inClause, args := uint32INClause(ids)

	rows, err := h.db.Query(fmt.Sprintf(`
		SELECT id, record_id, name_type, title_honorific, first_name, middle_name, surname,
		       maiden_name, suffix, single_string_name, original_script_name, entity_name
		FROM sanctions_names
		WHERE record_id IN %s AND name_type = 'Primary Name'
		ORDER BY record_id, id
	`, inClause), args...)
	if err != nil {
		return
	}
	defer rows.Close()

	byRecord := make(map[uint32]model.SanctionsName, len(records))
	for rows.Next() {
		var n model.SanctionsName
		if err := rows.Scan(&n.ID, &n.RecordID, &n.NameType, &n.TitleHonorific, &n.FirstName, &n.MiddleName,
			&n.Surname, &n.MaidenName, &n.Suffix, &n.SingleStringName, &n.OriginalScriptName, &n.EntityName); err != nil {
			continue
		}
		if _, ok := byRecord[n.RecordID]; !ok {
			byRecord[n.RecordID] = n
		}
	}

	for i := range records {
		if n, ok := byRecord[records[i].ID]; ok {
			records[i].Names = append(records[i].Names, n)
		}
	}
}

func (h *RecordsHandler) loadNames(rec *model.SanctionsRecord) {
	rows, err := h.db.Query(`
		SELECT id, record_id, name_type, title_honorific, first_name, middle_name, surname,
		       maiden_name, suffix, single_string_name, original_script_name, entity_name
		FROM sanctions_names WHERE record_id = ?
	`, rec.ID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var n model.SanctionsName
		if err := rows.Scan(&n.ID, &n.RecordID, &n.NameType, &n.TitleHonorific, &n.FirstName, &n.MiddleName,
			&n.Surname, &n.MaidenName, &n.Suffix, &n.SingleStringName, &n.OriginalScriptName, &n.EntityName); err != nil {
			continue
		}
		rec.Names = append(rec.Names, n)
	}
}

func (h *RecordsHandler) loadDates(rec *model.SanctionsRecord) {
	rows, err := h.db.Query("SELECT id, record_id, date_type, day, month, year, note FROM sanctions_dates WHERE record_id = ?", rec.ID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var d model.SanctionsDate
		if err := rows.Scan(&d.ID, &d.RecordID, &d.DateType, &d.Day, &d.Month, &d.Year, &d.Note); err != nil {
			continue
		}
		rec.Dates = append(rec.Dates, d)
	}
}

func (h *RecordsHandler) loadCountries(rec *model.SanctionsRecord) {
	rows, err := h.db.Query(`
		SELECT sc.id, sc.record_id, sc.country_type, sc.country_code, rc.name
		FROM sanctions_countries sc
		LEFT JOIN sanctions_ref_countries rc ON rc.code = sc.country_code
		WHERE sc.record_id = ?
	`, rec.ID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var c model.SanctionsCountry
		if err := rows.Scan(&c.ID, &c.RecordID, &c.CountryType, &c.CountryCode, &c.CountryName); err != nil {
			continue
		}
		rec.Countries = append(rec.Countries, c)
	}
}

func (h *RecordsHandler) loadImages(rec *model.SanctionsRecord) {
	rows, err := h.db.Query("SELECT id, record_id, url FROM sanctions_images WHERE record_id = ? LIMIT 5", rec.ID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var img model.SanctionsImage
		if err := rows.Scan(&img.ID, &img.RecordID, &img.URL); err != nil {
			continue
		}
		rec.Images = append(rec.Images, img)
	}
}

func (h *RecordsHandler) loadDescriptions(rec *model.SanctionsRecord) {
	rows, err := h.db.Query(`
		SELECT d1.name, d2.name, d3.name
		FROM sanctions_descriptions sd
		LEFT JOIN sanctions_ref_description1 d1 ON d1.description1_id = sd.description1_id
		LEFT JOIN sanctions_ref_description2 d2 ON d2.description2_id = sd.description2_id
		LEFT JOIN sanctions_ref_description3 d3 ON d3.description3_id = sd.description3_id
		WHERE sd.record_id = ?
	`, rec.ID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var d model.SanctionsDescriptionDetail
		if err := rows.Scan(&d.Description1, &d.Description2, &d.Description3); err != nil {
			continue
		}
		rec.Descriptions = append(rec.Descriptions, d)
	}
}

func (h *RecordsHandler) loadAssociations(rec *model.SanctionsRecord) {
	rows, err := h.db.Query(`
		SELECT sa.associate_id,
		       COALESCE(sn.entity_name, sn.single_string_name, CONCAT_WS(' ', sn.first_name, sn.middle_name, sn.surname), '') AS associate_name,
		       rr.name,
		       sa.is_ex
		FROM sanctions_associations sa
		LEFT JOIN sanctions_names sn ON sn.record_id = sa.associate_id AND sn.name_type = 'Primary Name'
		LEFT JOIN sanctions_ref_relationships rr ON rr.code = sa.relationship_code
		WHERE sa.record_id = ?
	`, rec.ID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var a model.SanctionsAssociationDetail
		if err := rows.Scan(&a.AssociateID, &a.AssociateName, &a.Relationship, &a.IsEx); err != nil {
			continue
		}
		rec.Associations = append(rec.Associations, a)
	}
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
