package handler

import (
	"testing"
)

func TestNormalizeSearchType(t *testing.T) {
	tests := map[string]string{
		"":           "individual",
		"individual": "individual",
		"Individual": "individual",
		"person":     "individual",
		"Person":     "individual",
		"entity":     "entity",
		"Entity":     "entity",
		"unknown":    "individual",
	}

	for input, want := range tests {
		if got := normalizeSearchType(input); got != want {
			t.Errorf("normalizeSearchType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRecordTypeFilterSQL(t *testing.T) {
	individual := recordTypeFilterSQL("individual")
	if !containsAll(individual, "person", "individual", "p") {
		t.Fatalf("individual filter missing person variants: %s", individual)
	}
	if contains(individual, "entity") {
		t.Fatalf("individual filter must not include entity: %s", individual)
	}

	entity := recordTypeFilterSQL("entity")
	if !containsAll(entity, "entity", "e") {
		t.Fatalf("entity filter missing entity variants: %s", entity)
	}
	if contains(entity, "person") {
		t.Fatalf("entity filter must not include person: %s", entity)
	}
}

func TestMergeCandidates(t *testing.T) {
	a := []nameCandidate{{recordID: 1, name: "Nasser Ahmed kamel ali"}}
	b := []nameCandidate{
		{recordID: 1, name: "Nasser Ahmed kamel ali"},
		{recordID: 2, name: "Nasser Bin Rashid Al Nuaimi"},
	}
	merged := mergeCandidates(a, b)
	if len(merged) != 2 {
		t.Fatalf("mergeCandidates() len = %d, want 2", len(merged))
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !contains(s, part) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
