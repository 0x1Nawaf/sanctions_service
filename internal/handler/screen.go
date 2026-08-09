package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nnn/sanctions-service/internal/model"
	"github.com/nnn/sanctions-service/internal/scoring"
)

const maxBatchScreenNames = 50

type ScreenHandler struct {
	db              *sql.DB
	useLikeFallback bool
}

func NewScreenHandler(db *sql.DB, useLikeFallback bool) *ScreenHandler {
	return &ScreenHandler{db: db, useLikeFallback: useLikeFallback}
}

var sanitizeRe = regexp.MustCompile(`[+\-><()~*"\\@.,;:!?']+`)

func normalizeScreenRequest(req *model.ScreenRequest) {
	if req.MinScore < 1 || req.MinScore > 100 {
		req.MinScore = 50
	}
	if req.SearchType == "" {
		req.SearchType = "individual"
	}
}

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
	normalizeScreenRequest(&req)

	results, err := h.screenWithScore(req)
	if err != nil {
		log.Printf("screen failed query=%q type=%s err=%v", req.Name, req.SearchType, err)
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

func (h *ScreenHandler) ScreenBatch(w http.ResponseWriter, r *http.Request) {
	var req model.BatchScreenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Names) == 0 {
		writeError(w, http.StatusBadRequest, "names is required")
		return
	}
	if len(req.Names) > maxBatchScreenNames {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("names exceeds maximum of %d", maxBatchScreenNames))
		return
	}

	screenReq := model.ScreenRequest{
		SearchType:     req.SearchType,
		MinScore:       req.MinScore,
		IncludeNotes:   req.IncludeNotes,
		IncludeDetails: req.IncludeDetails,
	}
	normalizeScreenRequest(&screenReq)

	batchResults := make([]model.BatchScreenResult, 0, len(req.Names))
	for _, name := range req.Names {
		name = strings.TrimSpace(name)
		if name == "" {
			batchResults = append(batchResults, model.BatchScreenResult{
				Query: name,
				Error: "name is required",
			})
			continue
		}
		screenReq.Name = name
		results, err := h.screenWithScore(screenReq)
		if err != nil {
			log.Printf("screen batch failed query=%q type=%s err=%v", name, screenReq.SearchType, err)
			batchResults = append(batchResults, model.BatchScreenResult{
				Query:    name,
				MinScore: screenReq.MinScore,
				Error:    "screening failed",
			})
			continue
		}
		batchResults = append(batchResults, model.BatchScreenResult{
			Query:    name,
			MinScore: screenReq.MinScore,
			Total:    len(results),
			Results:  results,
		})
	}

	writeJSON(w, model.BatchScreenResponse{
		Total:   len(batchResults),
		Results: batchResults,
	})
}

type nameCandidate struct {
	recordID uint32
	name     string
}

func (h *ScreenHandler) screenWithScore(req model.ScreenRequest) ([]model.ScreenResult, error) {
	start := time.Now()
	var timings screenPhaseTimings
	initialCandidateCount := 0
	expandedRecordCount := 0
	usedLike := false
	usedBroad := false
	resultCount := 0

	defer func() {
		timings.total = time.Since(start)
		logScreenTimings(req.Name, req.SearchType, timings, initialCandidateCount, expandedRecordCount, resultCount, usedLike, usedBroad)
	}()

	t0 := time.Now()
	candidates, usedBroadRetry, err := h.fetchCandidates(req.Name, req.SearchType)
	timings.fetchCandidates = time.Since(t0)
	if err != nil {
		return nil, err
	}
	initialCandidateCount = len(candidates)
	usedBroad = usedBroadRetry

	seen := make(map[string]bool)
	markCandidatesSeen(seen, candidates)

	bestByRecord := make(map[uint32]recordScore)
	t0 = time.Now()
	mergeCandidateScores(bestByRecord, candidates, req.Name, req.SearchType, req.MinScore)

	preliminary := preliminaryScores(candidates, req.Name, req.SearchType)
	expandIDs := selectRecordIDsForAliasExpansion(preliminary, req.MinScore)
	if len(expandIDs) > 0 {
		expandedRecordCount = len(expandIDs)
		tExpand := time.Now()
		expanded, err := h.fetchAllNamesForRecords(expandIDs)
		timings.expandNames = time.Since(tExpand)
		if err != nil {
			return nil, err
		}
		mergeCandidateScores(bestByRecord, dedupeExpandedCandidates(seen, expanded), req.Name, req.SearchType, req.MinScore)
		markCandidatesSeen(seen, expanded)
	}
	timings.score = time.Since(t0)

	if len(bestByRecord) == 0 && h.useLikeFallback {
		t0 = time.Now()
		tokens := tokenizeSearchName(req.Name)
		if len(tokens) > 0 {
			fallbackCandidates, err := h.fetchLikeCandidates(tokens, req.SearchType)
			if err != nil {
				return nil, err
			}
			if len(fallbackCandidates) > 0 {
				usedLike = true
				likePreliminary := preliminaryScores(fallbackCandidates, req.Name, req.SearchType)
				likeExpandIDs := selectRecordIDsForAliasExpansion(likePreliminary, req.MinScore)
				mergeCandidateScores(bestByRecord, fallbackCandidates, req.Name, req.SearchType, req.MinScore)
				fallbackSeen := make(map[string]bool)
				markCandidatesSeen(fallbackSeen, fallbackCandidates)
				if len(likeExpandIDs) > 0 {
					expanded, err := h.fetchAllNamesForRecords(likeExpandIDs)
					if err != nil {
						return nil, err
					}
					mergeCandidateScores(bestByRecord, dedupeExpandedCandidates(fallbackSeen, expanded), req.Name, req.SearchType, req.MinScore)
				}
			}
		}
		timings.likeRetry = time.Since(t0)
	}

	if len(bestByRecord) == 0 {
		return []model.ScreenResult{}, nil
	}

	sortedResults := make([]recordScore, 0, len(bestByRecord))
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

	t0 = time.Now()
	records, err := h.loadRecordsForScreen(ids, req)
	timings.hydrate = time.Since(t0)
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
			Record:         rec,
			Score:          s.score,
			MatchedName:    s.name,
			IsCustomList:   rec.CustomListID != nil,
			CustomListName: rec.CustomListName,
		})
	}

	resultCount = len(results)
	return results, nil
}

func tokenizeSearchName(searchName string) []string {
	sanitized := sanitizeRe.ReplaceAllString(searchName, "")
	rawTokens := strings.Fields(sanitized)
	tokens := make([]string, 0, len(rawTokens))
	for _, t := range rawTokens {
		if len([]rune(t)) >= 2 {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// fetchCandidates runs the precise FULLTEXT lookup and, only when it finds
// nothing, retries with the broader one. Both are served by the same index
// under the same LIMIT, so a miss costs one extra millisecond-scale query
// instead of the table scan the ngram and LIKE fallbacks used to cost.
//
// The bool reports whether the broad retry produced the candidates.
func (h *ScreenHandler) fetchCandidates(searchName, searchType string) ([]nameCandidate, bool, error) {
	tokens := tokenizeSearchName(searchName)
	if len(tokens) == 0 {
		return nil, false, nil
	}

	candidates, err := h.fetchFulltextCandidates(tokens, searchType)
	if err != nil {
		log.Printf("screen fulltext failed type=%s tokens=%v err=%v", searchType, tokens, err)
		return nil, false, err
	}
	if len(candidates) > 0 {
		return candidates, false, nil
	}

	broad, err := h.fetchBroadFulltextCandidates(tokens, searchType)
	if err != nil {
		log.Printf("screen broad fulltext failed type=%s tokens=%v err=%v", searchType, tokens, err)
		return nil, false, err
	}
	return broad, true, nil
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

	typeFilter := recordTypeFilterSQL(searchType)

	query := fmt.Sprintf(`
		SELECT sn.record_id,
		       sn.first_name, sn.middle_name, sn.surname,
		       sn.single_string_name, sn.entity_name, sn.original_script_name
		FROM sanctions_names sn
		INNER JOIN sanctions_records sr ON sr.id = sn.record_id
		WHERE sr.active_status = 'Active'
		  %s
		  AND %s
		LIMIT 300
	`, typeFilter, whereClause)

	rows, err := queryRowsWithRetry(h.db, query, args...)
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

	return candidates, rows.Err()
}

func wrapConditions(conditions []string) []string {
	wrapped := make([]string, len(conditions))
	for i, c := range conditions {
		wrapped[i] = "CASE WHEN " + c + " THEN 1 ELSE 0 END"
	}
	return wrapped
}

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
		if scoring.IsArabic(originalScriptName.String) {
			add(scoring.TransliterateArabic(originalScriptName.String))
		}
	}

	return candidates
}

func (h *ScreenHandler) fetchAllNamesForRecords(recordIDs map[uint32]bool) ([]nameCandidate, error) {
	if len(recordIDs) == 0 {
		return nil, nil
	}

	ids := make([]uint32, 0, len(recordIDs))
	for id := range recordIDs {
		ids = append(ids, id)
	}
	inClause, args := uint32INClause(ids)

	query := fmt.Sprintf(`
		SELECT record_id, first_name, middle_name, surname,
		       single_string_name, entity_name, original_script_name
		FROM sanctions_names
		WHERE record_id IN %s
	`, inClause)

	rows, err := queryRowsWithRetry(h.db, query, args...)
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

	return candidates, rows.Err()
}

func (h *ScreenHandler) loadRecordsForScreen(ids []uint32, req model.ScreenRequest) ([]model.SanctionsRecord, error) {
	return loadRecordsBatch(h.db, ids, batchLoadOptions{
		nameLimit:   10,
		imageLimit:  1,
		slim:        !req.IncludeNotes,
		skipDetails: !req.IncludeDetails,
	})
}
