package seeder

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

const batchSize = 2000

type Seeder struct {
	db *sql.DB
}

func New(db *sql.DB) *Seeder {
	return &Seeder{db: db}
}

func (s *Seeder) Run(jsonPath string) error {
	start := time.Now()
	log.Printf("Starting sanctions seeder from: %s", jsonPath)

	s.execOrLog("SET FOREIGN_KEY_CHECKS=0")
	s.execOrLog("SET unique_checks=0")
	s.execOrLog("SET autocommit=0")

	sections := []struct {
		pointer string
		fn      func(dec *json.Decoder) error
	}{
		{"/ref_country", s.seedRefCountries},
		{"/ref_occupation", s.seedRefOccupations},
		{"/ref_relationship", s.seedRefRelationships},
		{"/ref_sanctions", s.seedRefSanctionsLists},
		{"/ref_description1", s.seedRefDescription1},
		{"/ref_description2", s.seedRefDescription2},
		{"/ref_description3", s.seedRefDescription3},
		{"/record", s.seedRecords},
		{"/record_name", s.seedGenericChild("sanctions_names", nameColumns, nameValues)},
		{"/record_description", s.seedGenericChild("sanctions_descriptions", descColumns, descValues)},
		{"/record_role", s.seedGenericChild("sanctions_roles", roleColumns, roleValues)},
		{"/record_date", s.seedGenericChild("sanctions_dates", dateColumns, dateValues)},
		{"/record_birth_place", s.seedGenericChild("sanctions_birth_places", birthPlaceColumns, birthPlaceValues)},
		{"/record_sanctions_ref", s.seedGenericChild("sanctions_refs", refColumns, refValues)},
		{"/record_country", s.seedGenericChild("sanctions_countries", countryColumns, countryValues)},
		{"/record_id_number", s.seedGenericChild("sanctions_id_numbers", idNumberColumns, idNumberValues)},
		{"/record_source", s.seedGenericChild("sanctions_sources", sourceColumns, sourceValues)},
		{"/record_image", s.seedGenericChild("sanctions_images", imageColumns, imageValues)},
		{"/record_address", s.seedGenericChild("sanctions_addresses", addressColumns, addressValues)},
		{"/association", s.seedGenericChild("sanctions_associations", assocColumns, assocValues)},
	}

	for _, sec := range sections {
		log.Printf("Seeding section: %s", sec.pointer)
		key := strings.TrimPrefix(sec.pointer, "/")

		f, err := os.Open(jsonPath)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}

		dec := json.NewDecoder(f)
		if err := seekToKey(dec, key); err != nil {
			f.Close()
			log.Printf("  Section %s not found, skipping", key)
			continue
		}

		if err := sec.fn(dec); err != nil {
			f.Close()
			return fmt.Errorf("seed %s: %w", key, err)
		}
		f.Close()
	}

	s.execOrLog("COMMIT")
	s.execOrLog("SET unique_checks=1")
	s.execOrLog("SET FOREIGN_KEY_CHECKS=1")
	s.execOrLog("SET autocommit=1")

	log.Printf("Seeding complete in %s", time.Since(start))
	return nil
}

func seekToKey(dec *json.Decoder, key string) error {
	for {
		t, err := dec.Token()
		if err == io.EOF {
			return fmt.Errorf("key %q not found", key)
		}
		if err != nil {
			return err
		}
		if s, ok := t.(string); ok && s == key {
			t2, err := dec.Token()
			if err != nil {
				return err
			}
			if delim, ok := t2.(json.Delim); !ok || delim != '[' {
				return fmt.Errorf("expected array for key %q", key)
			}
			return nil
		}
	}
}

func (s *Seeder) execOrLog(query string) {
	if _, err := s.db.Exec(query); err != nil {
		log.Printf("WARN: %s — %v", query, err)
	}
}

func (s *Seeder) bulkInsert(table string, columns string, placeholderRow string, args []interface{}, rowCount int) error {
	if rowCount == 0 {
		return nil
	}
	rows := make([]string, rowCount)
	for i := range rows {
		rows[i] = placeholderRow
	}
	q := fmt.Sprintf("INSERT IGNORE INTO %s (%s) VALUES %s", table, columns, strings.Join(rows, ","))
	_, err := s.db.Exec(q, args...)
	return err
}

// --- Reference tables ---

func (s *Seeder) seedRefCountries(dec *json.Decoder) error {
	count := 0
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return err
		}
		_, err := s.db.Exec(
			"INSERT INTO sanctions_ref_countries (code, name, is_territory, profile_url) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE name=VALUES(name), is_territory=VALUES(is_territory), profile_url=VALUES(profile_url)",
			str(row, "code"), str(row, "name"), boolVal(row, "is_territory"), str(row, "profile_url"),
		)
		if err != nil {
			log.Printf("ref_countries error: %v", err)
		}
		count++
	}
	log.Printf("  ref_countries: %d rows", count)
	return nil
}

func (s *Seeder) seedRefOccupations(dec *json.Decoder) error {
	count := 0
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return err
		}
		_, err := s.db.Exec(
			"INSERT INTO sanctions_ref_occupations (code, name) VALUES (?, ?) ON DUPLICATE KEY UPDATE name=VALUES(name)",
			intVal(row, "code"), str(row, "name"),
		)
		if err != nil {
			log.Printf("ref_occupations error: %v", err)
		}
		count++
	}
	log.Printf("  ref_occupations: %d rows", count)
	return nil
}

func (s *Seeder) seedRefRelationships(dec *json.Decoder) error {
	count := 0
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return err
		}
		_, err := s.db.Exec(
			"INSERT INTO sanctions_ref_relationships (code, name) VALUES (?, ?) ON DUPLICATE KEY UPDATE name=VALUES(name)",
			intVal(row, "code"), str(row, "name"),
		)
		if err != nil {
			log.Printf("ref_relationships error: %v", err)
		}
		count++
	}
	log.Printf("  ref_relationships: %d rows", count)
	return nil
}

func (s *Seeder) seedRefSanctionsLists(dec *json.Decoder) error {
	count := 0
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return err
		}
		_, err := s.db.Exec(
			"INSERT INTO sanctions_ref_sanctions_lists (id, name, status, description2_id) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE name=VALUES(name), status=VALUES(status), description2_id=VALUES(description2_id)",
			intVal(row, "id"), str(row, "name"), str(row, "status"), intOrNil(row, "description2_id"),
		)
		if err != nil {
			log.Printf("ref_sanctions_lists error: %v", err)
		}
		count++
	}
	log.Printf("  ref_sanctions_lists: %d rows", count)
	return nil
}

func (s *Seeder) seedRefDescription1(dec *json.Decoder) error {
	count := 0
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return err
		}
		_, err := s.db.Exec(
			"INSERT INTO sanctions_ref_description1 (description1_id, record_type, name) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE record_type=VALUES(record_type), name=VALUES(name)",
			intVal(row, "description1_id"), str(row, "record_type"), str(row, "name"),
		)
		if err != nil {
			log.Printf("ref_description1 error: %v", err)
		}
		count++
	}
	log.Printf("  ref_description1: %d rows", count)
	return nil
}

func (s *Seeder) seedRefDescription2(dec *json.Decoder) error {
	count := 0
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return err
		}
		_, err := s.db.Exec(
			"INSERT INTO sanctions_ref_description2 (description2_id, description1_id, name) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE description1_id=VALUES(description1_id), name=VALUES(name)",
			intVal(row, "description2_id"), intOrNil(row, "description1_id"), str(row, "name"),
		)
		if err != nil {
			log.Printf("ref_description2 error: %v", err)
		}
		count++
	}
	log.Printf("  ref_description2: %d rows", count)
	return nil
}

func (s *Seeder) seedRefDescription3(dec *json.Decoder) error {
	count := 0
	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return err
		}
		_, err := s.db.Exec(
			"INSERT INTO sanctions_ref_description3 (description3_id, description2_id, name) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE description2_id=VALUES(description2_id), name=VALUES(name)",
			intVal(row, "description3_id"), intOrNil(row, "description2_id"), str(row, "name"),
		)
		if err != nil {
			log.Printf("ref_description3 error: %v", err)
		}
		count++
	}
	log.Printf("  ref_description3: %d rows", count)
	return nil
}

// --- Main records ---

func (s *Seeder) seedRecords(dec *json.Decoder) error {
	const cols = "id, record_type, action, action_date, gender, active_status, deceased, profile_notes"
	const ph = "(?, ?, ?, ?, ?, ?, ?, ?)"
	const colCount = 8

	args := make([]interface{}, 0, batchSize*colCount)
	count := 0
	batchRows := 0

	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return err
		}

		args = append(args,
			intVal(row, "id"),
			str(row, "record_type"),
			str(row, "action"),
			str(row, "action_date"),
			str(row, "gender"),
			str(row, "active_status"),
			str(row, "deceased"),
			str(row, "profile_notes"),
		)
		batchRows++
		count++

		if batchRows >= batchSize {
			if err := s.bulkInsert("sanctions_records", cols, ph, args, batchRows); err != nil {
				log.Printf("records insert error: %v", err)
			}
			args = args[:0]
			batchRows = 0
		}

		if count%100000 == 0 {
			log.Printf("  records: %dk rows...", count/1000)
		}
	}

	if batchRows > 0 {
		if err := s.bulkInsert("sanctions_records", cols, ph, args, batchRows); err != nil {
			log.Printf("records insert error: %v", err)
		}
	}

	log.Printf("  records: %d total rows", count)
	return nil
}

// --- Generic child table seeder ---

type columnExtractor func(row map[string]interface{}) []interface{}

var (
	nameColumns      = "record_id, name_type, title_honorific, first_name, middle_name, surname, maiden_name, suffix, single_string_name, original_script_name, entity_name"
	nameValues       = func(row map[string]interface{}) []interface{} {
		return []interface{}{
			intVal(row, "record_id"), str(row, "name_type"), str(row, "title_honorific"),
			str(row, "first_name"), str(row, "middle_name"), str(row, "surname"),
			str(row, "maiden_name"), str(row, "suffix"), str(row, "single_string_name"),
			str(row, "original_script_name"), str(row, "entity_name"),
		}
	}

	descColumns = "record_id, description1_id, description2_id, description3_id"
	descValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{
			intVal(row, "record_id"), intOrNil(row, "description1_id"),
			intOrNil(row, "description2_id"), intOrNil(row, "description3_id"),
		}
	}

	roleColumns = "record_id, role_type, occ_cat_code, title, since_day, since_month, since_year, to_day, to_month, to_year"
	roleValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{
			intVal(row, "record_id"), str(row, "role_type"), intOrNil(row, "occ_cat_code"),
			str(row, "title"), str(row, "since_day"), str(row, "since_month"),
			str(row, "since_year"), str(row, "to_day"), str(row, "to_month"), str(row, "to_year"),
		}
	}

	dateColumns = "record_id, date_type, day, month, year, note"
	dateValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{
			intVal(row, "record_id"), str(row, "date_type"),
			str(row, "day"), str(row, "month"), str(row, "year"), str(row, "note"),
		}
	}

	birthPlaceColumns = "record_id, place"
	birthPlaceValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{intVal(row, "record_id"), str(row, "place")}
	}

	refColumns = "record_id, sanctions_ref_id, since_day, since_month, since_year, to_day, to_month, to_year"
	refValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{
			intVal(row, "record_id"), intOrNil(row, "sanctions_ref_id"),
			str(row, "since_day"), str(row, "since_month"), str(row, "since_year"),
			str(row, "to_day"), str(row, "to_month"), str(row, "to_year"),
		}
	}

	countryColumns = "record_id, country_type, country_code"
	countryValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{intVal(row, "record_id"), str(row, "country_type"), str(row, "country_code")}
	}

	idNumberColumns = "record_id, id_type, id_value, id_notes"
	idNumberValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{intVal(row, "record_id"), str(row, "id_type"), str(row, "id_value"), str(row, "id_notes")}
	}

	sourceColumns = "record_id, url"
	sourceValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{intVal(row, "record_id"), str(row, "url")}
	}

	imageColumns = "record_id, url"
	imageValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{intVal(row, "record_id"), str(row, "url")}
	}

	addressColumns = "record_id, address_line, address_city, address_country, url"
	addressValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{
			intVal(row, "record_id"), str(row, "address_line"),
			str(row, "address_city"), str(row, "address_country"), str(row, "url"),
		}
	}

	assocColumns = "record_id, associate_id, relationship_code, is_ex"
	assocValues  = func(row map[string]interface{}) []interface{} {
		return []interface{}{
			intVal(row, "record_id"), intVal(row, "associate_id"),
			intOrNil(row, "relationship_code"), boolVal(row, "is_ex"),
		}
	}
)

func (s *Seeder) seedGenericChild(table, columns string, extractor columnExtractor) func(dec *json.Decoder) error {
	return func(dec *json.Decoder) error {
		colCount := strings.Count(columns, ",") + 1
		placeholders := "(" + strings.TrimRight(strings.Repeat("?,", colCount), ",") + ")"

		args := make([]interface{}, 0, batchSize*colCount)
		count := 0
		batchRows := 0

		for dec.More() {
			var row map[string]interface{}
			if err := dec.Decode(&row); err != nil {
				return err
			}

			vals := extractor(row)
			args = append(args, vals...)
			batchRows++
			count++

			if batchRows >= batchSize {
				if err := s.bulkInsert(table, columns, placeholders, args, batchRows); err != nil {
					log.Printf("%s insert error: %v", table, err)
				}
				args = args[:0]
				batchRows = 0
			}

			if count%100000 == 0 {
				log.Printf("  %s: %dk rows...", table, count/1000)
			}
		}

		if batchRows > 0 {
			if err := s.bulkInsert(table, columns, placeholders, args, batchRows); err != nil {
				log.Printf("%s insert error: %v", table, err)
			}
		}

		log.Printf("  %s: %d total rows", table, count)
		return nil
	}
}

// --- Helpers ---

func str(row map[string]interface{}, key string) interface{} {
	v, ok := row[key]
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil
		}
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func intVal(row map[string]interface{}, key string) interface{} {
	v, ok := row[key]
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	case string:
		return val
	default:
		return v
	}
}

func intOrNil(row map[string]interface{}, key string) interface{} {
	v, ok := row[key]
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case float64:
		return int64(val)
	default:
		return nil
	}
}

func boolVal(row map[string]interface{}, key string) int {
	v, ok := row[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case bool:
		if val {
			return 1
		}
		return 0
	case float64:
		if val != 0 {
			return 1
		}
		return 0
	default:
		return 0
	}
}
