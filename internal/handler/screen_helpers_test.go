package handler

import (
	"strings"
	"testing"
)

func TestRecordTypeFilterSQL(t *testing.T) {
	individual := recordTypeFilterSQL("individual")
	if !strings.Contains(individual, "'Individual'") || !strings.Contains(individual, "'Person'") {
		t.Fatalf("individual filter missing person variants: %s", individual)
	}
	if strings.Contains(individual, "Entity") {
		t.Fatalf("individual filter must not include entity: %s", individual)
	}

	entity := recordTypeFilterSQL("entity")
	if !strings.Contains(entity, "'Entity'") {
		t.Fatalf("entity filter missing entity: %s", entity)
	}
	if strings.Contains(entity, "Person") {
		t.Fatalf("entity filter must not include person: %s", entity)
	}

	if got := recordTypeFilterSQL(""); got != "" {
		t.Fatalf("empty search type filter = %q, want empty", got)
	}
}

func TestBuildBooleanFTQuery(t *testing.T) {
	got := buildBooleanFTQuery([]string{"nasser", "ahmed", "kamel", "ali"})
	want := "+nasser* +ahmed* kamel* ali*"
	if got != want {
		t.Fatalf("buildBooleanFTQuery() = %q, want %q", got, want)
	}
}

func TestBuildNgramFTQuery(t *testing.T) {
	got := buildNgramFTQuery([]string{"nasser", "ahmed", "kamel"})
	want := "+nasser +ahmed kamel"
	if got != want {
		t.Fatalf("buildNgramFTQuery() = %q, want %q", got, want)
	}
}

// Requiring a connector excludes every record that spells the same person
// without it, which is most of them: بن and ال are optional in practice.
func TestBuildFTQueryNeverRequiresConnectors(t *testing.T) {
	tests := []struct {
		name   string
		tokens []string
		want   string
	}{
		{
			name:   "arabic patronymic chain",
			tokens: []string{"سالم", "بن", "عبدالله", "بن", "احمد", "الشهري"},
			want:   "+سالم* بن* +عبدالله* بن* احمد* الشهري*",
		},
		{
			name:   "latin connectors and article",
			tokens: []string{"Nouf", "Bint", "Fahd", "Al", "Saud"},
			want:   "+Nouf* Bint* +Fahd* Al* Saud*",
		},
		{
			name:   "no connectors keeps the leading half required",
			tokens: []string{"nasser", "ahmed", "kamel", "ali"},
			want:   "+nasser* +ahmed* kamel* ali*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildBooleanFTQuery(tt.tokens); got != tt.want {
				t.Fatalf("buildBooleanFTQuery(%v) = %q, want %q", tt.tokens, got, tt.want)
			}
		})
	}
}

// The candidate lookup must filter on MATCH in the WHERE clause. Moving it to a
// selected column plus HAVING makes MySQL scan all of sanctions_names instead of
// using the FULLTEXT index, which turns a sub-second screen into ~50 seconds.
func TestNameSearchQueryFiltersOnMatchInWhere(t *testing.T) {
	query := nameSearchQuery(wordFulltextIndex, recordTypeFilterSQL("individual"))

	if strings.Contains(strings.ToUpper(query), "HAVING") {
		t.Fatalf("query must not filter relevance with HAVING:\n%s", query)
	}

	wherePos := strings.Index(query, "WHERE")
	if wherePos < 0 {
		t.Fatalf("query has no WHERE clause:\n%s", query)
	}
	if !strings.Contains(query[wherePos:], "MATCH(") {
		t.Fatalf("WHERE clause must contain the MATCH predicate:\n%s", query)
	}
}

func TestNameSearchQueryPinsFulltextIndex(t *testing.T) {
	// Both FULLTEXT indexes cover the same columns, so each query has to name
	// the one it wants.
	word := nameSearchQuery(wordFulltextIndex, "")
	if !strings.Contains(word, "FORCE INDEX ("+wordFulltextIndex+")") {
		t.Fatalf("word query does not pin %s:\n%s", wordFulltextIndex, word)
	}
	if strings.Contains(word, ngramFulltextIndex) {
		t.Fatalf("word query must not reference the ngram index:\n%s", word)
	}

	ngram := nameSearchQuery(ngramFulltextIndex, "")
	if !strings.Contains(ngram, "FORCE INDEX ("+ngramFulltextIndex+")") {
		t.Fatalf("ngram query does not pin %s:\n%s", ngramFulltextIndex, ngram)
	}
}

func TestNameSearchQueryPlaceholderCount(t *testing.T) {
	// fetchFulltextCandidates / fetchNgramCandidates pass the search string
	// twice: once for the selected relevance, once for the WHERE predicate.
	query := nameSearchQuery(wordFulltextIndex, recordTypeFilterSQL("entity"))
	if got := strings.Count(query, "?"); got != 2 {
		t.Fatalf("placeholder count = %d, want 2:\n%s", got, query)
	}
}

func TestTokenizeSearchNameDropsShortAndPunctuation(t *testing.T) {
	got := tokenizeSearchName("Nouf Mohammed M. Alkahtani")
	want := []string{"Nouf", "Mohammed", "Alkahtani"}
	if len(got) != len(want) {
		t.Fatalf("tokenizeSearchName() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokenizeSearchName() = %v, want %v", got, want)
		}
	}
}
