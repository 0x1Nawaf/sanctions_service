package handler

import "testing"

func TestSelectRecordIDsForAliasExpansion(t *testing.T) {
	preliminary := map[uint32]recordScore{
		1: {recordID: 1, score: 80},
		2: {recordID: 2, score: 45},
		3: {recordID: 3, score: 10},
		4: {recordID: 4, score: 35},
	}

	ids := selectRecordIDsForAliasExpansion(preliminary, 50)
	if ids[1] != true {
		t.Fatal("expected record 1 to expand")
	}
	if ids[2] != true {
		t.Fatal("expected record 2 to expand (45 >= 30 threshold)")
	}
	if ids[4] != true {
		t.Fatal("expected record 4 to expand (35 >= 30 threshold)")
	}
	if ids[3] {
		t.Fatal("expected record 3 to be excluded (10 < 30 threshold)")
	}
}

func TestDedupeExpandedCandidates(t *testing.T) {
	initial := []nameCandidate{
		{recordID: 1, name: "John Smith"},
		{recordID: 2, name: "Jane Doe"},
	}
	seen := make(map[string]bool)
	markCandidatesSeen(seen, initial)

	expanded := []nameCandidate{
		{recordID: 1, name: "John Smith"},
		{recordID: 1, name: "Johnny Smith"},
		{recordID: 2, name: "Jane Doe"},
	}
	extra := dedupeExpandedCandidates(seen, expanded)
	if len(extra) != 1 {
		t.Fatalf("len(extra) = %d, want 1", len(extra))
	}
	if extra[0].name != "Johnny Smith" {
		t.Fatalf("extra name = %q, want Johnny Smith", extra[0].name)
	}
}

func TestSelectRecordIDsForAliasExpansionCapsRecords(t *testing.T) {
	preliminary := make(map[uint32]recordScore, maxAliasExpandRecords+10)
	for i := uint32(1); i <= uint32(maxAliasExpandRecords+10); i++ {
		preliminary[i] = recordScore{recordID: i, score: int(i)}
	}

	ids := selectRecordIDsForAliasExpansion(preliminary, 0)
	if len(ids) != maxAliasExpandRecords {
		t.Fatalf("len(ids) = %d, want %d", len(ids), maxAliasExpandRecords)
	}
	if !ids[uint32(maxAliasExpandRecords+10)] {
		t.Fatal("expected highest-scoring records to be kept")
	}
	if ids[1] {
		t.Fatal("expected lowest-scoring records to be dropped")
	}
}
