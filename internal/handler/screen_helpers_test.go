package handler

import (
	"database/sql"
	"testing"

	"github.com/nnn/sanctions-service/internal/model"
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

func TestRecordMatchesSearchType(t *testing.T) {
	customListID := uint32(1)
	customRec := model.SanctionsRecord{
		CustomListID: &customListID,
		RecordType:   model.NullString{NullString: sql.NullString{String: "Entity", Valid: true}},
	}
	if !recordMatchesSearchType(customRec, "individual") {
		t.Fatal("custom list records should always match")
	}

	personCases := []string{"Person", "person", "Individual", "P"}
	for _, rt := range personCases {
		rec := model.SanctionsRecord{
			RecordType: model.NullString{NullString: sql.NullString{String: rt, Valid: true}},
		}
		if !recordMatchesSearchType(rec, "individual") {
			t.Fatalf("record_type %q should match individual search", rt)
		}
		if recordMatchesSearchType(rec, "entity") {
			t.Fatalf("record_type %q should not match entity search", rt)
		}
	}

	entityRec := model.SanctionsRecord{
		RecordType: model.NullString{NullString: sql.NullString{String: "Entity", Valid: true}},
	}
	if !recordMatchesSearchType(entityRec, "entity") {
		t.Fatal("Entity records should match entity search")
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
