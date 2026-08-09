package handler

import (
	"strings"
	"testing"
)

func TestRecordDedupeIsPerRecord(t *testing.T) {
	d := make(recordDedupe)

	if d.seen(1, "ahmed") {
		t.Fatal("first sighting must not be reported as seen")
	}
	if !d.seen(1, "ahmed") {
		t.Fatal("repeat within the same record must be reported as seen")
	}
	if d.seen(2, "ahmed") {
		t.Fatal("the same value under a different record is a distinct row")
	}
}

// Field values must not be able to run together into the same key: a surname of
// "al" with an empty first name is a different row from an empty surname with a
// first name of "al".
func TestDedupeKeySeparatesFields(t *testing.T) {
	if dedupeKey("al", "") == dedupeKey("", "al") {
		t.Fatal("distinct field combinations collided into one key")
	}
}

// Duplicated rows must not consume the per-record limit, or a record re-seeded
// from several feeds returns ten copies of one name instead of ten names.
func TestWithinLimitCountsOnlyWhatItAdmits(t *testing.T) {
	counts := make(map[uint32]int)

	for i := 0; i < 3; i++ {
		if !withinLimit(counts, 1, 3) {
			t.Fatalf("row %d rejected below the limit", i)
		}
	}
	if withinLimit(counts, 1, 3) {
		t.Fatal("limit not enforced")
	}
	if !withinLimit(counts, 2, 3) {
		t.Fatal("limit must be tracked per record")
	}
}

func TestWithinLimitUnlimitedWhenNotPositive(t *testing.T) {
	counts := make(map[uint32]int)
	for i := 0; i < 100; i++ {
		if !withinLimit(counts, 1, 0) {
			t.Fatalf("row %d rejected under an unlimited limit", i)
		}
	}
}

// Joining associates on name_type alone repeats the association once per
// "Primary Name" row, and records routinely carry a Latin and an
// original-script row under that type.
func TestAssociationQueryResolvesOneNamePerAssociate(t *testing.T) {
	query := associationQuery("(?)")
	if strings.Contains(query, "sn.record_id = sa.associate_id") {
		t.Fatalf("association join must not fan out over every primary name row:\n%s", query)
	}
	if !strings.Contains(query, "SELECT MIN(pn.id)") {
		t.Fatalf("association join must pick a single name row:\n%s", query)
	}
}

func TestUint32INClause(t *testing.T) {
	clause, args := uint32INClause([]uint32{10, 20, 30})
	if clause != "(?,?,?)" {
		t.Fatalf("clause = %q, want (?,?,?)", clause)
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
	}
	if args[0] != uint32(10) || args[1] != uint32(20) || args[2] != uint32(30) {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestUint32INClauseEmpty(t *testing.T) {
	clause, args := uint32INClause(nil)
	if clause != "()" {
		t.Fatalf("clause = %q, want ()", clause)
	}
	if len(args) != 0 {
		t.Fatalf("args len = %d, want 0", len(args))
	}
}
