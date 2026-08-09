package handler

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"

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

// Boolean-mode operators in raw input would either change the meaning of the
// query or make MySQL reject it outright.
var ftTokenSanitizeRe = regexp.MustCompile(`[+\-><()~*"\\@',;:!?.]+`)

func sanitizeFTToken(token string) string {
	return strings.TrimSpace(ftTokenSanitizeRe.ReplaceAllString(token, ""))
}

func sanitizeFTTokens(tokens []string) []string {
	cleaned := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if s := sanitizeFTToken(t); s != "" {
			cleaned = append(cleaned, s)
		}
	}
	return cleaned
}

// buildBooleanFTQuery is the precise lookup: half of the query's tokens are
// required, so the candidate set stays small enough for the FULLTEXT index to
// serve it in milliseconds.
func buildBooleanFTQuery(tokens []string) string {
	cleaned := sanitizeFTTokens(tokens)
	significant := 0
	for _, t := range cleaned {
		if !scoring.IsNameConnector(t) {
			significant++
		}
	}
	return buildFTQuery(cleaned, (significant+1)/2)
}

// buildBroadFTQuery is the retry used when the precise lookup finds nothing. It
// requires only the single most distinctive token, so a record still surfaces
// when every other token is transliterated differently ("Nura" for "Noura").
//
// It stays on the same FULLTEXT index and the same LIMIT, so the retry costs
// one more indexed query rather than the table scan an ngram or LIKE fallback
// would cost on a multi-million-row feed.
func buildBroadFTQuery(tokens []string) string {
	return buildFTQuery(sanitizeFTTokens(tokens), 1)
}

// requiredTokenIndices picks which tokens the FULLTEXT query insists on.
//
// Longest first, because transliteration varies most in short given names while
// the surname carries the distinguishing letters. Picking by position instead
// pins the query to the given name, and a feed spelling it "Nura" or "Noora"
// then excludes the record before scoring ever sees it.
//
// Connectors are never eligible. "بن" and "ال" appear in a large share of Gulf
// names but plenty of records spell the same person without them, so requiring
// one excludes every such record. They stay in the query as optional terms,
// where they still contribute to relevance ordering.
func requiredTokenIndices(tokens []string, count int) map[int]bool {
	type candidate struct {
		index  int
		length int
	}

	eligible := make([]candidate, 0, len(tokens))
	for i, t := range tokens {
		if scoring.IsNameConnector(t) {
			continue
		}
		eligible = append(eligible, candidate{index: i, length: len([]rune(t))})
	}

	sort.SliceStable(eligible, func(a, b int) bool {
		return eligible[a].length > eligible[b].length
	})

	required := make(map[int]bool, count)
	for i := 0; i < count && i < len(eligible); i++ {
		required[eligible[i].index] = true
	}
	return required
}

// ftTermGroup renders one query token, together with its article-stripped
// spelling when it has one. "alkahtani" becomes "(alkahtani* kahtani*)" so the
// term also matches records the word parser split into "al" and "kahtani".
func ftTermGroup(token string) string {
	variant := scoring.ArticleStrippedVariant(token)
	if variant == "" || variant == token {
		return token + "*"
	}
	return "(" + token + "* " + variant + "*)"
}

func buildFTQuery(cleaned []string, requiredCount int) string {
	if len(cleaned) == 0 {
		return ""
	}

	required := requiredTokenIndices(cleaned, requiredCount)

	ftTerms := make([]string, 0, len(cleaned))
	for i, t := range cleaned {
		group := ftTermGroup(t)
		if required[i] {
			group = "+" + group
		}
		ftTerms = append(ftTerms, group)
	}
	return strings.Join(ftTerms, " ")
}

const (
	nameSearchColumns = "sn.first_name, sn.middle_name, sn.surname, sn.single_string_name, sn.original_script_name, sn.entity_name"
	nameSearchLimit   = 300

	wordFulltextIndex = "sanctions_names_fulltext"
)

// nameSearchQuery builds the candidate lookup against the word-parser FULLTEXT
// index on sanctions_names.
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
// ftIndex is pinned with FORCE INDEX because a second FULLTEXT index may cover
// an identical column list, and MATCH alone does not identify which one to use.
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
	return h.runFulltextQuery(buildBooleanFTQuery(tokens), searchType)
}

func (h *ScreenHandler) fetchBroadFulltextCandidates(tokens []string, searchType string) ([]nameCandidate, error) {
	return h.runFulltextQuery(buildBroadFTQuery(tokens), searchType)
}

func (h *ScreenHandler) runFulltextQuery(ftQuery, searchType string) ([]nameCandidate, error) {
	if ftQuery == "" {
		return nil, nil
	}
	query := nameSearchQuery(wordFulltextIndex, recordTypeFilterSQL(searchType))

	return h.queryNameCandidates(query, []interface{}{ftQuery, ftQuery})
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
