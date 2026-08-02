package handler

import (
	"strings"
	"testing"
)

func TestBuildRecordListQueryNoFilters(t *testing.T) {
	from, where, args := buildRecordListQuery(recordListFilters{})

	if strings.Contains(from, "sanctions_names") {
		t.Fatalf("unfiltered list must not join names: %s", from)
	}
	if where != "" {
		t.Fatalf("where = %q, want empty", where)
	}
	if len(args) != 0 {
		t.Fatalf("args = %v, want none", args)
	}
}

func TestBuildRecordListQueryStatusOnlyAvoidsNameJoin(t *testing.T) {
	filters := recordListFilters{recordType: "Individual", activeStatus: "Active"}
	if filters.needsNameJoin() {
		t.Fatal("status/type filters must not require the name join")
	}

	from, where, args := buildRecordListQuery(filters)
	if strings.Contains(from, "sanctions_names") {
		t.Fatalf("unexpected name join: %s", from)
	}
	if !strings.Contains(where, "sr.record_type = ?") || !strings.Contains(where, "sr.active_status = ?") {
		t.Fatalf("missing conditions: %s", where)
	}
	if len(args) != 2 {
		t.Fatalf("args = %v, want 2", args)
	}
}

func TestBuildRecordListQueryNameFilters(t *testing.T) {
	filters := recordListFilters{firstName: "nouf", lastName: "alkahtani"}
	if !filters.needsNameJoin() {
		t.Fatal("name filters must require the name join")
	}

	from, where, args := buildRecordListQuery(filters)
	if !strings.Contains(from, "INNER JOIN sanctions_names sn") {
		t.Fatalf("missing name join: %s", from)
	}
	if !strings.Contains(where, "sn.first_name LIKE ?") || !strings.Contains(where, "sn.surname LIKE ?") {
		t.Fatalf("missing name conditions: %s", where)
	}
	if len(args) != 2 {
		t.Fatalf("args = %v, want 2", args)
	}
	if args[0] != "nouf%" || args[1] != "alkahtani%" {
		t.Fatalf("unexpected args: %v", args)
	}
}

// A leading wildcard makes the prefix indexes unusable and scans every name row,
// so patterns must never start with %.
func TestLikePrefixHasNoLeadingWildcard(t *testing.T) {
	for _, input := range []string{"nouf", "%nouf", "%", "_ouf", `back\slash`} {
		got := likePrefix(input)
		if strings.HasPrefix(got, "%") {
			t.Fatalf("likePrefix(%q) = %q, must not start with %%", input, got)
		}
		if !strings.HasSuffix(got, "%") {
			t.Fatalf("likePrefix(%q) = %q, must end with %%", input, got)
		}
	}
}

func TestLikePrefixEscapesWildcards(t *testing.T) {
	tests := map[string]string{
		"nouf":  "nouf%",
		"%nouf": `\%nouf%`,
		"n_uf":  `n\_uf%`,
		`a\b`:   `a\\b%`,
	}
	for input, want := range tests {
		if got := likePrefix(input); got != want {
			t.Fatalf("likePrefix(%q) = %q, want %q", input, got, want)
		}
	}
}

// The count and data queries share the filter args slice, so building the data
// args must not write into it.
func TestBuildRecordListQueryArgsNotAliased(t *testing.T) {
	_, _, args := buildRecordListQuery(recordListFilters{recordType: "Individual"})

	dataArgs := make([]interface{}, 0, len(args)+2)
	dataArgs = append(dataArgs, args...)
	dataArgs = append(dataArgs, 25, 0)

	if len(args) != 1 {
		t.Fatalf("filter args mutated: %v", args)
	}
	if len(dataArgs) != 3 {
		t.Fatalf("dataArgs = %v, want 3 entries", dataArgs)
	}
}
