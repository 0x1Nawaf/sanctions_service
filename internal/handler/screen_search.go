package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/nnn/sanctions-service/internal/scoring"
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
	return buildFTQuery(tokens, "*")
}

func buildNgramFTQuery(tokens []string) string {
	cleaned := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if s := sanitizeFTToken(t); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}

	// Ngram matching is far broader than word FULLTEXT. Requiring only half of the
	// tokens (as in buildFTQuery) can match millions of rows on a large feed and
	// trigger MySQL error 188 (FTS query exceeds result cache limit).
	ftTerms := make([]string, 0, len(cleaned))
	for _, t := range cleaned {
		if scoring.IsNameConnector(t) {
			ftTerms = append(ftTerms, t)
			continue
		}
		ftTerms = append(ftTerms, "+"+t)
	}
	return strings.Join(ftTerms, " ")
}

// buildFTQuery marks the leading half of the query's tokens as required so the
// candidate set stays small, and the rest as optional.
//
// Connectors are never made required. "بن" and "ال" appear in a large share of
// Gulf names but plenty of records spell the same person without them, so
// requiring one excludes every such record from the candidate set before
// scoring ever runs. They stay in the query as optional terms, where they still
// contribute to relevance ordering.
var ftTokenSanitizeRe = regexp.MustCompile(`[+\-><()~*"\\@',;:!?.]+`)

func sanitizeFTToken(token string) string {
	return strings.TrimSpace(ftTokenSanitizeRe.ReplaceAllString(token, ""))
}

func buildFTQuery(tokens []string, suffix string) string {
	cleaned := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if s := sanitizeFTToken(t); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}

	significant := 0
	for _, t := range cleaned {
		if !scoring.IsNameConnector(t) {
			significant++
		}
	}
	required := (significant + 1) / 2

	ftTerms := make([]string, 0, len(cleaned))
	for _, t := range cleaned {
		if required > 0 && !scoring.IsNameConnector(t) {
			ftTerms = append(ftTerms, "+"+t+suffix)
			required--
			continue
		}
		ftTerms = append(ftTerms, t+suffix)
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
	if ftQuery == "" {
		return nil, nil
	}
	query := nameSearchQuery(wordFulltextIndex, recordTypeFilterSQL(searchType))

	return h.queryNameCandidates(query, []interface{}{ftQuery, ftQuery})
}

func (h *ScreenHandler) tryLikeFallback(tokens []string, searchType string) ([]nameCandidate, bool, error) {
	likeCandidates, err := h.fetchLikeCandidates(tokens, searchType)
	if err != nil {
		return nil, true, err
	}
	return likeCandidates, true, nil
}

func (h *ScreenHandler) fetchNgramCandidates(tokens []string, searchType string) ([]nameCandidate, error) {
	ftQuery := buildNgramFTQuery(tokens)
	if ftQuery == "" {
		return nil, nil
	}
	query := nameSearchQuery(ngramFulltextIndex, recordTypeFilterSQL(searchType))

	return h.queryNameCandidates(query, []interface{}{ftQuery, ftQuery})
}

func isFTSCacheLimitErr(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 188 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "fts query exceeds result cache limit")
}

func (h *ScreenHandler) queryNameCandidates(query string, args []interface{}) ([]nameCandidate, error) {
	rows, err := queryRowsWithRetry(h.db, query, args...)
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
		if isFTSCacheLimitErr(err) {
			log.Printf("screen ngram FTS cache limit exceeded query=%q tokens=%v", buildNgramFTQuery(tokens), tokens)
			return h.tryLikeFallback(tokens, searchType)
		}
		if h.useLikeFallback {
			log.Printf("screen ngram failed, trying LIKE fallback tokens=%v err=%v", tokens, err)
			return h.tryLikeFallback(tokens, searchType)
		}
		return nil, false, fmt.Errorf("ngram query: %w", err)
	}
	if len(candidates) > 0 {
		return candidates, false, nil
	}

	if h.useLikeFallback {
		return h.tryLikeFallback(tokens, searchType)
	}

	return nil, false, nil
}
