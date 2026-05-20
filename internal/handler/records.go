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
	search := q.Get("search")

	needsNameJoin := firstName != "" || lastName != "" || search != ""

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
	if search != "" {
		conditions = append(conditions, "(sn.first_name LIKE ? OR sn.surname LIKE ? OR sn.single_string_name LIKE ? OR sn.entity_name LIKE ?)")
		args = append(args, "%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
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
	rows, err := h.db.Query("SELECT id, record_id, country_type, country_code FROM sanctions_countries WHERE record_id = ?", rec.ID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var c model.SanctionsCountry
		if err := rows.Scan(&c.ID, &c.RecordID, &c.CountryType, &c.CountryCode); err != nil {
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

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
