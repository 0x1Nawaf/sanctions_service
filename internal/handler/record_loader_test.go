package handler

import "testing"

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
