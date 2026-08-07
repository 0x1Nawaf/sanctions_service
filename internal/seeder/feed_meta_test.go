package seeder

import (
	"os"
	"testing"
)

func TestIsCompleteFeed(t *testing.T) {
	s := &Seeder{}

	s.feedMeta = feedMeta{FeedScope: "complete"}
	if !s.isCompleteFeed(3_900_000, 3_900_000) {
		t.Fatal("expected complete feed")
	}

	s.feedMeta = feedMeta{FeedScope: "delta_only"}
	if s.isCompleteFeed(3_900_000, 3_900_000) {
		t.Fatal("expected partial feed for delta_only")
	}

	// A delta mislabelled as complete must not license inactivation.
	s.feedMeta = feedMeta{FeedScope: "complete"}
	if s.isCompleteFeed(3_330, 2_500_000) {
		t.Fatal("expected partial feed when a complete label contradicts the row counts")
	}

	s.feedMeta = feedMeta{FeedScope: "complete", RecordCount: 3_330}
	if s.isCompleteFeed(3_900_000, 3_900_000) {
		t.Fatal("expected partial feed when declared record_count is far below existing")
	}

	// An unlabelled feed states nothing about its scope, so it never inactivates.
	s.feedMeta = feedMeta{}
	if s.isCompleteFeed(1_000, 3_900_000) {
		t.Fatal("expected partial feed for unlabelled feed")
	}
	if s.isCompleteFeed(3_800_000, 3_900_000) {
		t.Fatal("expected partial feed for unlabelled feed even when sizes agree")
	}

	// Empty database: nothing to inactivate, and the label still governs.
	s.feedMeta = feedMeta{FeedScope: "complete", RecordCount: 3_330}
	if !s.isCompleteFeed(3_330, 0) {
		t.Fatal("expected complete feed when seeding an empty database")
	}
}

func TestRecordNeedsUpdate(t *testing.T) {
	ex := existingRecord{activeStatus: "Inactive"}
	row := map[string]interface{}{"active_status": "Active"}
	if !recordNeedsUpdate(ex, row) {
		t.Fatal("expected update when status changes")
	}

	ex = existingRecord{activeStatus: "Active"}
	row = map[string]interface{}{"active_status": "Active", "action": "chg"}
	if recordNeedsUpdate(ex, row) {
		t.Fatal("expected no update when unchanged")
	}

	ex = existingRecord{activeStatus: "Active"}
	row = map[string]interface{}{"active_status": "Inactive", "action": "del"}
	if !recordNeedsUpdate(ex, row) {
		t.Fatal("expected update when feed marks delete on active record")
	}
}

func TestReadFeedMeta(t *testing.T) {
	path := t.TempDir() + "/sample.json"
	if err := writeTempJSON(path, `{"_meta":{"feed_scope":"complete","record_count":42},"record":[]}`); err != nil {
		t.Fatal(err)
	}
	meta, err := readFeedMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.FeedScope != "complete" || meta.RecordCount != 42 {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}

func writeTempJSON(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
