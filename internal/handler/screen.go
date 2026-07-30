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

var sanitizeRe = regexp.MustCompile(`[+\-><()~*"@.,;:!?']+`)

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
	req.SearchType = normalizeSearchType(req.SearchType)

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
	candidates, err := h.fetchCandidates(req.Name, req.SearchType)
	if err != nil {
		return nil, err
	}

	// Collect record IDs from initial search hits, then fetch ALL name rows
	// for those records. A FULLTEXT/LIKE match may only hit one name row per
	// record, but the best-scoring variant could be on a different row (AKA,
	// alternate transliteration, etc.).
	recordIDs := make(map[uint32]bool, len(candidates))
	for _, c := range candidates {
		recordIDs[c.recordID] = true
	}
	allCandidates, err := h.fetchAllNamesForRecords(recordIDs)
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, allCandidates...)

	type scored struct {
		recordID uint32
		score    int
		name     string
	}

	bestByRecord := make(map[uint32]scored)
	for _, c := range candidates {
		var s int
		if req.SearchType == "entity" {
			s = scoring.ScoreEntityName(req.Name, c.name)
		} else {
			s = scoring.ScoreName(req.Name, c.name)
		}
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
		if !recordMatchesSearchType(rec, req.SearchType) {
			continue
		}
		results = append(results, model.ScreenResult{
			Record:         rec,
			Score:          s.score,
			MatchedName:    s.name,
			IsCustomList:   rec.CustomListID != nil,
			CustomListName: rec.CustomListName,
		})
	}

	return results, nil
}

func (h *ScreenHandler) fetchCandidates(searchName, searchType string) ([]nameCandidate, error) {
	sanitized := sanitizeRe.ReplaceAllString(searchName, "")
	rawTokens := strings.Fields(sanitized)
	// Drop single-character tokens â they're below FULLTEXT ft_min_token_size
	// and too noisy for LIKE (e.g. initials "M", "A", "K" match everything).
	tokens := make([]string, 0, len(rawTokens))
	for _, t := range rawTokens {
		if len([]rune(t)) >= 2 {
			tokens = append(tokens, t)
		}
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	candidates, err := h.fetchFulltextCandidates(tokens, searchType)
	if err != nil {
		return nil, err
	}

	likeCandidates, err := h.fetchLikeCandidates(tokens, searchType)
	if err != nil {
		return nil, err
	}

	return mergeCandidates(candidates, likeCandidates), nil
}

func (h *ScreenHandler) fetchFulltextCandidates(tokens []string, searchType string) ([]nameCandidate, error) {
	required := (len(tokens) + 1) / 2
	ftTerms := make([]string, len(tokens))
	for i, t := range tokens {
		if i < required {
			ftTerms[i] = "+" + t + "*"
		} else {
			ftTerms[i] = t + "*"
		}
	}
	ftQuery := strings.Join(ftTerms, " ")

	_ = searchType

	var args []interface{}
	args = append(args, ftQuery, ftQuery)

	query := fmt.Sprintf(`
		SELECT sn.record_id,
		       sn.first_name, sn.middle_name, sn.surname,
		       sn.single_string_name, sn.entity_name, sn.original_script_name,
		       MATCH(sn.first_name, sn.middle_name, sn.surname, sn.single_string_name, sn.original_script_name, sn.entity_name) AGAINST(? IN NATURAL LANGUAGE MODE) AS relevance
		FROM sanctions_names sn
		INNER JOIN sanctions_records sr ON sr.id = sn.record_id
		WHERE %s
		  AND MATCH(sn.first_name, sn.middle_name, sn.surname, sn.single_string_name, sn.original_script_name, sn.entity_name) AGAINST(? IN BOOLEAN MODE)
		ORDER BY relevance DESC
		LIMIT 300
	`, activeStatusFilterSQL())

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("fulltext query: %w", err)
	}
	defer rows.Close()

	var candidates []nameCandidate
	for rows.Next() {
		var recordID uint32
		var firstName, middleName, surname, singleStringName, entityName, originalScriptName sql.NullString
		var relevance float64
		if err := rows.Scan(&recordID, &firstName, &middleName, &surname,
			&singleStringName, &entityName, &originalScriptName, &relevance); err != nil {
			continue
		}
		candidates = append(candidates, buildNameCandidates(recordID, firstName, middleName, surname, singleStringName, entityName, originalScriptName)...)
	}

	return candidates, nil
}

func (h *ScreenHandler) fetchLikeCandidates(tokens []string, searchType string) ([]nameCandidate, error) {
	var conditions []string
	var args []interface{}

	searchCols := []string{"sn.first_name", "sn.middle_name", "sn.surname", "sn.single_string_name", "sn.original_script_name", "sn.entity_name"}

	for _, token := range tokens {
		var colConditions []string
		for _, col := range searchCols {
			colConditions = append(colConditions, col+" LIKE ?")
			args = append(args, "%"+token+"%")
		}
		conditions = append(conditions, "("+strings.Join(colConditions, " OR ")+")")
	}

	required := (len(tokens) + 1) / 2
	whereClause := ""
	if len(conditions) <= 2 {
		whereClause = strings.Join(conditions, " AND ")
	} else {
		whereClause = fmt.Sprintf("(%s) >= %d",
			strings.Join(wrapConditions(conditions), " + "), required)
	}

	_ = searchType

	query := fmt.Sprintf(`
		SELECT sn.record_id,
		       sn.first_name, sn.middle_name, sn.surname,
		       sn.single_string_name, sn.entity_name, sn.original_script_name
		FROM sanctions_names sn
		INNER JOIN sanctions_records sr ON sr.id = sn.record_id
		WHERE %s
		  AND %s
		LIMIT 300
	`, activeStatusFilterSQL(), whereClause)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("like fallback query: %w", err)
	}
	defer rows.Close()

	var candidates []nameCandidate
	for rows.Next() {
		var recordID uint32
		var firstName, middleName, surname, singleStringName, entityName, originalScriptName sql.NullString
		if err := rows.Scan(&recordID, &firstName, &middleName, &surname,
			&singleStringName, &entityName, &originalScriptName); err != nil {
			continue
		}
		candidates = append(candidates, buildNameCandidates(recordID, firstName, middleName, surname, singleStringName, entityName, originalScriptName)...)
	}

	return candidates, nil
}

func wrapConditions(conditions []string) []string {
	wrapped := make([]string, len(conditions))
	for i, c := range conditions {
		wrapped[i] = "CASE WHEN " + c + " THEN 1 ELSE 0 END"
	}
	return wrapped
}

// buildNameCandidates produces scoring candidates from every non-empty name
// representation in a sanctions_names row, so the scorer can pick the best.
func buildNameCandidates(recordID uint32, firstName, middleName, surname, singleStringName, entityName, originalScriptName sql.NullString) []nameCandidate {
	var candidates []nameCandidate
	seen := make(map[string]bool)

	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		lower := strings.ToLower(name)
		if seen[lower] {
			return
		}
		seen[lower] = true
		candidates = append(candidates, nameCandidate{recordID: recordID, name: name})
	}

	if entityName.Valid {
		add(entityName.String)
	}

	// Structured name (first + middle + surname) â the most complete representation
	var parts []string
	if firstName.Valid && firstName.String != "" {
		parts = append(parts, firstName.String)
	}
	if middleName.Valid && middleName.String != "" {
		parts = append(parts, middleName.String)
	}
	if surname.Valid && surname.String != "" {
		parts = append(parts, surname.String)
	}
	if len(parts) > 0 {
		add(strings.Join(parts, " "))
	}

	if singleStringName.Valid {
		add(singleStringName.String)
	}
	if originalScriptName.Valid {
		add(originalScriptName.String)
		// Transliterate non-Latin scripts to Latin for cross-script matching
		if scoring.IsArabic(originalScriptName.String) {
			add(scoring.TransliterateArabic(originalScriptName.String))
		}
	}

	return candidates
}

// fetchAllNamesForRecords retrieves every name row for the given records and
// builds candidates from all columns including transliterated Arabic names.
func (h *ScreenHandler) fetchAllNamesForRecords(recordIDs map[uint32]bool) ([]nameCandidate, error) {
	if len(recordIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(recordIDs))
	args := make([]interface{}, 0, len(recordIDs))
	for id := range recordIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT record_id, first_name, middle_name, surname,
		       single_string_name, entity_name, original_script_name
		FROM sanctions_names
		WHERE record_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []nameCandidate
	for rows.Next() {
		var recordID uint32
		var firstName, middleName, surname, singleStringName, entityName, originalScriptName sql.NullString
		if err := rows.Scan(&recordID, &firstName, &middleName, &surname,
			&singleStringName, &entityName, &originalScriptName); err != nil {
			continue
		}
		candidates = append(candidates, buildNameCandidates(recordID, firstName, middleName, surname, singleStringName, entityName, originalScriptName)...)
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
		SELECT sr.id, sr.record_type, sr.action, sr.action_date, sr.gender,
		       sr.active_status, sr.deceased, sr.profile_notes,
		       sr.custom_list_id, COALESCE(cl.name, '')
		FROM sanctions_records sr
		LEFT JOIN custom_lists cl ON cl.id = sr.custom_list_id
		WHERE sr.id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]model.SanctionsRecord, 0, len(ids))
	for rows.Next() {
		var rec model.SanctionsRecord
		var customListID sql.NullInt64
		var listName string
		if err := rows.Scan(&rec.ID, &rec.RecordType, &rec.Action, &rec.ActionDate,
			&rec.Gender, &rec.ActiveStatus, &rec.Deceased, &rec.ProfileNotes,
			&customListID, &listName); err != nil {
			continue
		}
		applyCustomListMeta(&rec, customListID, listName)
		records = append(records, rec)
	}

	for i := range records {
		h.loadRecordNames(&records[i])
		h.loadRecordDates(&records[i])
		h.loadRecordCountries(&records[i])
		h.loadRecordImages(&records[i])
		h.loadRecordDescriptions(&records[i])
		h.loadRecordAssociations(&records[i])
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

func (h *ScreenHandler) loadRecordDescriptions(rec *model.SanctionsRecord) {
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

func (h *ScreenHandler) loadRecordAssociations(rec *model.SanctionsRecord) {
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

// normalizeSearchType accepts common client variants (person, individual, entity).
func normalizeSearchType(searchType string) string {
	switch strings.ToLower(strings.TrimSpace(searchType)) {
	case "entity":
		return "entity"
	default:
		return "individual"
	}
}

func activeStatusFilterSQL() string {
	return "LOWER(TRIM(sr.active_status)) = 'active'"
}

func mergeCandidates(a, b []nameCandidate) []nameCandidate {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]nameCandidate, 0, len(a)+len(b))
	for _, list := range [][]nameCandidate{a, b} {
		for _, c := range list {
			key := fmt.Sprintf("%d:%s", c.recordID, strings.ToLower(c.name))
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, c)
		}
	}
	return out
}

func recordMatchesSearchType(rec model.SanctionsRecord, searchType string) bool {
	if rec.CustomListID != nil {
		return true
	}

	rt := ""
	if rec.RecordType.Valid {
		rt = strings.ToLower(strings.TrimSpace(rec.RecordType.String))
	}

	if searchType == "entity" {
		return rt == "entity" || rt == "e"
	}

	if rt == "" {
		return true
	}
	return rt == "person" || rt == "individual" || rt == "p"
}
