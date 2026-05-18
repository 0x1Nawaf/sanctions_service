package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/nnn/sanctions-service/internal/model"
	"github.com/nnn/sanctions-service/internal/scoring"
)

type ScreenHandler struct {
	db *sql.DB
}

func NewScreenHandler(db *sql.DB) *ScreenHandler {
	return &ScreenHandler{db: db}
}

var sanitizeRe = regexp.MustCompile(`[+\-><()~*"@]+`)

func (h *ScreenHandler) Screen(w http.ResponseWriter, r *http.Request) {
	var req model.ScreenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.MinScore < 1 || req.MinScore > 100 {
		req.MinScore = 50
	}
	if req.SearchType == "" {
		req.SearchType = "individual"
	}

	results, err := h.screenWithScore(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "screening failed")
		return
	}

	writeJSON(w, model.ScreenResponse{
		Query:    req.Name,
		MinScore: req.MinScore,
		Total:    len(results),
		Results:  results,
	})
}

type nameCandidate struct {
	recordID uint32
	name     string
}

func (h *ScreenHandler) screenWithScore(req model.ScreenRequest) ([]model.ScreenResult, error) {
	candidates, err := h.fetchCandidates(req.Name)
	if err != nil {
		return nil, err
	}

	type scored struct {
		recordID uint32
		score    int
		name     string
	}

	bestByRecord := make(map[uint32]scored)
	for _, c := range candidates {
		s := scoring.ScoreName(req.Name, c.name)
		if s < req.MinScore {
			continue
		}
		if existing, ok := bestByRecord[c.recordID]; !ok || s > existing.score {
			bestByRecord[c.recordID] = scored{recordID: c.recordID, score: s, name: c.name}
		}
	}

	if len(bestByRecord) == 0 {
		return []model.ScreenResult{}, nil
	}

	sortedResults := make([]scored, 0, len(bestByRecord))
	for _, s := range bestByRecord {
		sortedResults = append(sortedResults, s)
	}
	sort.Slice(sortedResults, func(i, j int) bool {
		return sortedResults[i].score > sortedResults[j].score
	})

	limit := 50
	if len(sortedResults) > limit {
		sortedResults = sortedResults[:limit]
	}

	ids := make([]uint32, len(sortedResults))
	for i, s := range sortedResults {
		ids[i] = s.recordID
	}

	records, err := h.loadRecords(ids)
	if err != nil {
		return nil, err
	}

	recordMap := make(map[uint32]model.SanctionsRecord, len(records))
	for _, rec := range records {
		recordMap[rec.ID] = rec
	}

	results := make([]model.ScreenResult, 0, len(sortedResults))
	for _, s := range sortedResults {
		rec, ok := recordMap[s.recordID]
		if !ok {
			continue
		}
		results = append(results, model.ScreenResult{
			Record:      rec,
			Score:       s.score,
			MatchedName: s.name,
		})
	}

	return results, nil
}

func (h *ScreenHandler) fetchCandidates(searchName string) ([]nameCandidate, error) {
	sanitized := sanitizeRe.ReplaceAllString(searchName, "")
	tokens := strings.Fields(sanitized)
	if len(tokens) == 0 {
		return nil, nil
	}

	ftTerms := make([]string, len(tokens))
	for i, t := range tokens {
		ftTerms[i] = "+" + t + "*"
	}
	ftQuery := strings.Join(ftTerms, " ")

	query := `
		SELECT sn.record_id,
		       COALESCE(sn.entity_name, sn.single_string_name, CONCAT_WS(' ', sn.first_name, sn.middle_name, sn.surname)) AS display_name,
		       MATCH(sn.first_name, sn.surname, sn.single_string_name, sn.entity_name) AGAINST(? IN NATURAL LANGUAGE MODE) AS relevance
		FROM sanctions_names sn
		INNER JOIN sanctions_records sr ON sr.id = sn.record_id
		WHERE sr.active_status = 'Active'
		  AND MATCH(sn.first_name, sn.surname, sn.single_string_name, sn.entity_name) AGAINST(? IN BOOLEAN MODE)
		ORDER BY relevance DESC
		LIMIT 300
	`

	rows, err := h.db.Query(query, ftQuery, ftQuery)
	if err != nil {
		return nil, fmt.Errorf("fulltext query: %w", err)
	}
	defer rows.Close()

	var candidates []nameCandidate
	for rows.Next() {
		var recordID uint32
		var name sql.NullString
		var relevance float64
		if err := rows.Scan(&recordID, &name, &relevance); err != nil {
			continue
		}
		if name.Valid && name.String != "" {
			candidates = append(candidates, nameCandidate{recordID: recordID, name: name.String})
		}
	}

	return candidates, nil
}

func (h *ScreenHandler) loadRecords(ids []uint32) ([]model.SanctionsRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, record_type, action, action_date, gender, active_status, deceased, profile_notes
		FROM sanctions_records
		WHERE id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]model.SanctionsRecord, 0, len(ids))
	for rows.Next() {
		var rec model.SanctionsRecord
		if err := rows.Scan(&rec.ID, &rec.RecordType, &rec.Action, &rec.ActionDate, &rec.Gender, &rec.ActiveStatus, &rec.Deceased, &rec.ProfileNotes); err != nil {
			continue
		}
		records = append(records, rec)
	}

	for i := range records {
		h.loadRecordNames(&records[i])
		h.loadRecordDates(&records[i])
		h.loadRecordCountries(&records[i])
		h.loadRecordImages(&records[i])
	}

	return records, nil
}

func (h *ScreenHandler) loadRecordNames(rec *model.SanctionsRecord) {
	rows, err := h.db.Query(`
		SELECT id, record_id, name_type, title_honorific, first_name, middle_name, surname,
		       maiden_name, suffix, single_string_name, original_script_name, entity_name
		FROM sanctions_names WHERE record_id = ? LIMIT 10
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

func (h *ScreenHandler) loadRecordDates(rec *model.SanctionsRecord) {
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

func (h *ScreenHandler) loadRecordCountries(rec *model.SanctionsRecord) {
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

func (h *ScreenHandler) loadRecordImages(rec *model.SanctionsRecord) {
	rows, err := h.db.Query("SELECT id, record_id, url FROM sanctions_images WHERE record_id = ? LIMIT 1", rec.ID)
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
