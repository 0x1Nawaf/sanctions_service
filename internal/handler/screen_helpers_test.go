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

func TestBuildBooleanFTQueryStripsBooleanOperators(t *testing.T) {
	got := buildBooleanFTQuery([]string{`"john"`, `+smith`, `o'brien`})
	want := "john* +smith* +obrien*"
	if got != want {
		t.Fatalf("buildBooleanFTQuery() = %q, want %q", got, want)
	}
}

func TestBuildBooleanFTQueryEmptyAfterSanitize(t *testing.T) {
	if got := buildBooleanFTQuery([]string{"+", "()", `"`}); got != "" {
		t.Fatalf("buildBooleanFTQuery() = %q, want empty", got)
	}
}

func TestBuildBooleanFTQuery(t *testing.T) {
	got := buildBooleanFTQuery([]string{"nasser", "ahmed", "kamel", "ali"})
	want := "+nasser* +ahmed* kamel* ali*"
	if got != want {
		t.Fatalf("buildBooleanFTQuery() = %q, want %q", got, want)
	}
}

// "noura alkahtani" returned a 500 in production: the word index found nothing,
// which handed the query to the ngram fallback and MySQL error 188. Both halves
// of the miss are covered here — the surname has to be the required term, and
// it has to also search the spelling the word parser produces for "Al-Kahtani".
func TestBuildFTQueryForNouraAlkahtani(t *testing.T) {
	tokens := tokenizeSearchName("noura alkahtani")
	if len(tokens) != 2 || tokens[0] != "noura" || tokens[1] != "alkahtani" {
		t.Fatalf("tokenizeSearchName() = %v", tokens)
	}

	got := buildBooleanFTQuery(tokens)
	want := "noura* +(alkahtani* kahtani*)"
	if got != want {
		t.Fatalf("buildBooleanFTQuery() = %q, want %q", got, want)
	}
}

// The broad retry is what runs when the precise query finds nothing, so it must
// require exactly one term: any more and a differently transliterated token
// still excludes the record.
func TestBuildBroadFTQueryRequiresOnlyMostDistinctiveToken(t *testing.T) {
	got := buildBroadFTQuery([]string{"nasser", "ahmed", "kamel", "ali"})
	want := "+nasser* ahmed* kamel* ali*"
	if got != want {
		t.Fatalf("buildBroadFTQuery() = %q, want %q", got, want)
	}
}

// Sources write the article joined, separated, or not at all. The word parser
// splits "Al-Kahtani" into "al" and "kahtani", so the joined spelling a user
// types has to search the stripped form too.
func TestFTTermGroupSearchesArticleStrippedSpelling(t *testing.T) {
	tests := []struct {
		token string
		want  string
	}{
		{token: "alkahtani", want: "(alkahtani* kahtani*)"},
		{token: "الشهري", want: "(الشهري* شهري*)"},
		{token: "kahtani", want: "kahtani*"},
		{token: "ali", want: "ali*"},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			if got := ftTermGroup(tt.token); got != tt.want {
				t.Fatalf("ftTermGroup(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
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
			want:   "سالم* بن* +عبدالله* بن* احمد* +(الشهري* شهري*)",
		},
		{
			name:   "latin connectors and article",
			tokens: []string{"Nouf", "Bint", "Fahd", "Al", "Saud"},
			want:   "+Nouf* Bint* +Fahd* Al* Saud*",
		},
		{
			name:   "no connectors keeps half the tokens required",
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
	// A second FULLTEXT index may cover the same columns, so the query has to
	// name the one it wants.
	word := nameSearchQuery(wordFulltextIndex, "")
	if !strings.Contains(word, "FORCE INDEX ("+wordFulltextIndex+")") {
		t.Fatalf("word query does not pin %s:\n%s", wordFulltextIndex, word)
	}
	if strings.Contains(word, "ngram") {
		t.Fatalf("screening must not reference the ngram index:\n%s", word)
	}
}

func TestNameSearchQueryPlaceholderCount(t *testing.T) {
	// runFulltextQuery passes the search string
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
