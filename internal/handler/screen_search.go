package handler

import (
	"database/sql"
	"fmt"
	"strings"
)

func recordTypeFilterSQL(searchType string) string {
	if searchType == "entity" {
		return "AND sr.record_type = 'Entity'"
	}
	if searchType == "individual" {
		return "AND sr.record_type IN ('Individual', 'Person')"
	}
	return ""
}

func buildBooleanFTQuery(tokens []string) string {
	required := (len(tokens) + 1) / 2
	ftTerms := make([]string, len(tokens))
	for i, t := range tokens {
		if i < required {
			ftTerms[i] = "+" + t + "*"
		} else {
			ftTerms[i] = t + "*"
		}
	}
	return strings.Join(ftTerms, " ")
}

func buildNgramFTQuery(tokens []string) string {
	required := (len(tokens) + 1) / 2
	ftTerms := make([]string, len(tokens))
	for i, t := range tokens {
		if i < required {
			ftTerms[i] = "+" + t
		} else {
			ftTerms[i] = t
		}
	}
	return strings.Join(ftTerms, " ")
}

const (
	nameSearchColumns = "sn.first_name, sn.middle_name, sn.surname, sn.single_string_name, sn.original_script_name, sn.entity_name"
	nameSearchLimit   = 300

	wordFulltextIndex  = "sanctions_names_fulltext"
	ngramFulltextIndex = "sanctions_names_ngram_fulltext"
)

// nameSearchQuery builds the candidate lookup against one of the FULLTEXT
// indexes on sanctions_names.
//
// The MATCH predicate has to sit in the WHERE clause: that is the only form
// MySQL can serve from the FULLTEXT index. Selecting MATCH as a column and
// filtering it with HAVING instead forces a full scan of sanctions_names plus a
// join per row, which costs tens of seconds on a multi-million-row feed.
//
// Both copies of the expression use BOOLEAN MODE so MySQL treats them as one
// function and runs a single full-text search, and ORDER BY on its alias lets
// the optimizer stop reading once LIMIT rows have passed the filters.
//
// ftIndex is pinned with FORCE INDEX because the word-parser and ngram indexes
// cover an identical column list, so MATCH alone does not identify which one to
// use.
func nameSearchQuery(ftIndex, typeFilter string) string {
	matchExpr := fmt.Sprintf("MATCH(%s) AGAINST(? IN BOOLEAN MODE)", nameSearchColumns)

	return fmt.Sprintf(`
		SELECT sn.record_id,
		       sn.first_name, sn.middle_name, sn.surname,
		       sn.single_string_name, sn.entity_name, sn.original_script_name,
		       %s AS relevance
		FROM sanctions_names sn FORCE INDEX (%s)
		INNER JOIN sanctions_records sr ON sr.id = sn.record_id
		WHERE %s
		  AND sr.active_status = 'Active'
		  %s
		ORDER BY relevance DESC
		LIMIT %d
	`, matchExpr, ftIndex, matchExpr, typeFilter, nameSearchLimit)
}

func (h *ScreenHandler) fetchFulltextCandidates(tokens []string, searchType string) ([]nameCandidate, error) {
	ftQuery := buildBooleanFTQuery(tokens)
	query := nameSearchQuery(wordFulltextIndex, recordTypeFilterSQL(searchType))

	return h.queryNameCandidates(query, []interface{}{ftQuery, ftQuery})
}

func (h *ScreenHandler) fetchNgramCandidates(tokens []string, searchType string) ([]nameCandidate, error) {
	ftQuery := buildNgramFTQuery(tokens)
	query := nameSearchQuery(ngramFulltextIndex, recordTypeFilterSQL(searchType))

	return h.queryNameCandidates(query, []interface{}{ftQuery, ftQuery})
}

func (h *ScreenHandler) queryNameCandidates(query string, args []interface{}) ([]nameCandidate, error) {
	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
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
	return candidates, rows.Err()
}

func (h *ScreenHandler) fetchFallbackCandidates(tokens []string, searchType string) ([]nameCandidate, bool, error) {
	candidates, err := h.fetchNgramCandidates(tokens, searchType)
	if err != nil {
		if h.useLikeFallback {
			likeCandidates, likeErr := h.fetchLikeCandidates(tokens, searchType)
			if likeErr != nil {
				return nil, true, likeErr
			}
			return likeCandidates, true, nil
		}
		return nil, false, fmt.Errorf("ngram query: %w", err)
	}
	if len(candidates) > 0 {
		return candidates, false, nil
	}

	if h.useLikeFallback {
		likeCandidates, err := h.fetchLikeCandidates(tokens, searchType)
		if err != nil {
			return nil, true, err
		}
		return likeCandidates, true, nil
	}

	return nil, false, nil
}
