package seeder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSectionIndexAndOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	const body = `{
  "ref_country": [
    {"code": "US", "name": "United States", "is_territory": false, "profile_url": ""},
    {"code": "CA", "name": "Canada", "is_territory": false, "profile_url": ""}
  ],
  "record": [
    {"id": 1, "record_type": "P", "action": "add", "active_status": "Active"}
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	index, err := buildSectionIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := index["ref_country"]; !ok {
		t.Fatalf("missing ref_country in index: %v", index)
	}

	f, dec, err := openSectionDecoder(path, "ref_country", index)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	count := 0
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("expected 2 countries, got %d", count)
	}
}

func TestRealSanctionsSeederJSONLayout(t *testing.T) {
	path := filepath.Join("..", "..", "sanctions_seeder.json")
	if _, err := os.Stat(path); err != nil {
		t.Skip("sanctions_seeder.json not present")
	}

	index, err := buildSectionIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(index) < 20 {
		t.Fatalf("expected at least 20 array sections, got %d", len(index))
	}
	for _, key := range []string{"ref_country", "record", "association", "_meta"} {
		if key == "_meta" {
			continue
		}
		if _, ok := index[key]; !ok {
			t.Fatalf("missing indexed section %q", key)
		}
	}

	f, dec, err := openSectionDecoder(path, "ref_country", index)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var n int
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			t.Fatal(err)
		}
		if n == 0 && row["code"] == nil {
			t.Fatalf("unexpected ref_country row shape: %v", row)
		}
		n++
	}
	if n != 252 {
		t.Fatalf("ref_country rows: got %d want 252", n)
	}
}

func TestSkipValueTopLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	body := `{"ref_country":[{"code":"X"}],"record":[{"id":1}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	index, err := buildSectionIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 2 {
		t.Fatalf("index len %d", len(index))
	}
}
