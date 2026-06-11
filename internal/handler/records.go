package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/nnn/sanctions-service/internal/model"
)

type RecordsHandler struct {
	db *sql.DB
}

func NewRecordsHandler(db *sql.DB) *RecordsHandler {
	return &RecordsHandler{db: db}
}

func (h *RecordsHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 25
	}
	offset := (page - 1) * perPage

	q := r.URL.Query()
	firstName := q.Get("first_name")
	lastName := q.Get("last_name")
	recordType := q.Get("record_type")
	activeStatus := q.Get("active_status")

	needsNameJoin := firstName != "" || lastName != ""

	var conditions []string
	var args []interface{}

	if recordType != "" {
		conditions = append(conditions, "sr.record_type = ?")
		args = append(args, recordType)
	}
	if activeStatus != "" {
		conditions = append(conditions, "sr.active_status = ?")
		args = append(args, activeStatus)
	}
	if firstName != "" {
		conditions = append(conditions, "sn.first_name LIKE ?")
		args = append(args, "%"+firstName+"%")
	}
	if lastName != "" {
		conditions = append(conditions, "sn.surname LIKE ?")
		args = append(args, "%"+lastName+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			whereClause += " AND " + c
		}
	}

	fromClause := "FROM sanctions_records sr"
	if needsNameJoin {
		fromClause += " INNER JOIN sanctions_names sn ON sn.record_id = sr.id AND sn.name_type = 'Primary Name'"
	}

	countQuery := "SELECT COUNT(DISTINCT sr.id) " + fromClause + " " + whereClause
	var total int
	if err := h.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "count query failed")
		return
	}

	dataQuery := "SELECT DISTINCT sr.id, sr.record_type, sr.action, sr.action_date, sr.gender, sr.active_status, sr.deceased " +
		fromClause + " " + whereClause + " ORDER BY sr.id LIMIT ? OFFSET ?"
	dataArgs := append(args, perPage, offset)

	rows, err := h.db.Query(dataQuery, dataArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	records := make([]model.SanctionsRecord, 0, perPage)
	for rows.Next() {
		var rec model.SanctionsRecord
		if err := rows.Scan(&rec.ID, &rec.RecordType, &rec.Action, &rec.ActionDate, &rec.Gender, &rec.ActiveStatus, &rec.Deceased); err != nil {
			continue
		}
		h.loadPrimaryName(&rec)
		records = append(records, rec)
	}

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
	err = h.db.QueryRow(`
		SELECT id, record_type, action, action_date, gender, active_status, deceased, profile_notes
		FROM sanctions_records WHERE id = ?
	`, id).Scan(&rec.ID, &rec.RecordType, &rec.Action, &rec.ActionDate, &rec.Gender, &rec.ActiveStatus, &rec.Deceased, &rec.ProfileNotes)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	h.loadNames(&rec)
	h.loadDates(&rec)
	h.loadCountries(&rec)
	h.loadImages(&rec)
	h.loadDescriptions(&rec)
	h.loadAssociations(&rec)

	writeJSON(w, rec)
}

func (h *RecordsHandler) loadPrimaryName(rec *model.SanctionsRecord) {
	rows, err := h.db.Query(`
		SELECT id, record_id, name_type, title_honorific, first_name, middle_name, surname,
		       maiden_name, suffix, single_string_name, original_script_name, entity_name
		FROM sanctions_names
		WHERE record_id = ? AND name_type = 'Primary Name'
		LIMIT 1
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
