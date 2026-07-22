package seeder

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

type recordEnrichment struct {
	displayName string
	dateOfBirth string
	countries   []countrySnapshot
}

type countrySnapshot struct {
	Type string `json:"type,omitempty"`
	Code string `json:"code,omitempty"`
	Name string `json:"name,omitempty"`
}

func (s *Seeder) persistRecordChangeDetails() error {
	if s.history == nil || len(s.history.recordEvents) == 0 {
		return nil
	}

	events := s.history.recordEvents
	uniqueIDs := make(map[uint32]struct{}, len(events))
	for _, ev := range events {
		uniqueIDs[ev.recordID] = struct{}{}
	}

	enriched := make(map[uint32]recordEnrichment, len(uniqueIDs))
	ids := make([]uint32, 0, len(uniqueIDs))
	for id := range uniqueIDs {
		ids = append(ids, id)
	}

	for i := 0; i < len(ids); i += recordEventBatchSize {
		end := i + recordEventBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := s.enrichRecordBatch(ids[i:end], enriched); err != nil {
			return err
		}
	}

	const insertCols = `seed_run_id, record_id, change_type, record_type, active_status, gender, action, action_date, deceased, display_name, date_of_birth, countries_json`
	const insertPh = `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	args := make([]interface{}, 0, recordEventBatchSize*12)
	rows := 0

	flush := func() {
		if rows == 0 {
			return
		}
		placeholders := strings.TrimRight(strings.Repeat(insertPh+",", rows), ",")
		q := fmt.Sprintf("INSERT INTO seed_run_record_changes (%s) VALUES %s", insertCols, placeholders)
		if _, err := s.db.Exec(q, args...); err != nil {
			log.Printf("WARN: insert seed_run_record_changes batch: %v", err)
		}
		args = args[:0]
		rows = 0
	}

	for _, ev := range events {
		extra := enriched[ev.recordID]
		var countriesJSON interface{}
		if len(extra.countries) > 0 {
			b, err := json.Marshal(extra.countries)
			if err != nil {
				return err
			}
			countriesJSON = string(b)
		}

		args = append(args,
			s.history.runID,
			ev.recordID,
			ev.changeType,
			nullIfEmpty(ev.recordType),
			nullIfEmpty(ev.activeStatus),
			nullIfEmpty(ev.gender),
			nullIfEmpty(ev.action),
			nullIfEmpty(ev.actionDate),
			nullIfEmpty(ev.deceased),
			nullIfEmpty(extra.displayName),
			nullIfEmpty(extra.dateOfBirth),
			countriesJSON,
		)
		rows++
		if rows >= recordEventBatchSize {
			flush()
		}
	}
	flush()
	return nil
}

func (s *Seeder) enrichRecordBatch(ids []uint32, out map[uint32]recordEnrichment) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	nameQuery := fmt.Sprintf(`
		SELECT record_id, entity_name, single_string_name, first_name, middle_name, surname
		FROM sanctions_names
		WHERE record_id IN (%s)
		ORDER BY record_id, FIELD(name_type, 'Primary Name', 'Also Known As', 'Spelling Variation'), id`, placeholders)
	nameRows, err := s.db.Query(nameQuery, args...)
	if err != nil {
		return err
	}
	defer nameRows.Close()

	for nameRows.Next() {
		var recordID uint32
		var entityName, singleName, firstName, middleName, surname sql.NullString
		if err := nameRows.Scan(&recordID, &entityName, &singleName, &firstName, &middleName, &surname); err != nil {
			continue
		}
		cur := out[recordID]
		if cur.displayName != "" {
			continue
		}
		cur.displayName = pickDisplayName(entityName, singleName, firstName, middleName, surname)
		out[recordID] = cur
	}

	dobQuery := fmt.Sprintf(`
		SELECT record_id, day, month, year
		FROM sanctions_dates
		WHERE record_id IN (%s) AND date_type LIKE '%%Birth%%'
		ORDER BY record_id, id`, placeholders)
	dobRows, err := s.db.Query(dobQuery, args...)
	if err != nil {
		return err
	}
	defer dobRows.Close()

	for dobRows.Next() {
		var recordID uint32
		var day, month, year sql.NullString
		if err := dobRows.Scan(&recordID, &day, &month, &year); err != nil {
			continue
		}
		cur := out[recordID]
		if cur.dateOfBirth != "" {
			continue
		}
		cur.dateOfBirth = formatDOB(day, month, year)
		out[recordID] = cur
	}

	countryQuery := fmt.Sprintf(`
		SELECT sc.record_id, sc.country_type, sc.country_code, COALESCE(rc.name, sc.country_code)
		FROM sanctions_countries sc
		LEFT JOIN sanctions_ref_countries rc ON rc.code = sc.country_code
		WHERE sc.record_id IN (%s)
		ORDER BY sc.record_id, sc.id`, placeholders)
	countryRows, err := s.db.Query(countryQuery, args...)
	if err != nil {
		return err
	}
	defer countryRows.Close()

	for countryRows.Next() {
		var recordID uint32
		var countryType, countryCode, countryName sql.NullString
		if err := countryRows.Scan(&recordID, &countryType, &countryCode, &countryName); err != nil {
			continue
		}
		cur := out[recordID]
		cur.countries = append(cur.countries, countrySnapshot{
			Type: countryType.String,
			Code: countryCode.String,
			Name: countryName.String,
		})
		out[recordID] = cur
	}

	return nil
}

func pickDisplayName(entityName, singleName, firstName, middleName, surname sql.NullString) string {
	if entityName.Valid && entityName.String != "" {
		return entityName.String
	}
	if singleName.Valid && singleName.String != "" {
		return singleName.String
	}
	name := strings.TrimSpace(strings.TrimSpace(firstName.String+" "+middleName.String) + " " + surname.String)
	return strings.TrimSpace(name)
}

func formatDOB(day, month, year sql.NullString) string {
	var parts []string
	if year.Valid && year.String != "" {
		parts = append(parts, year.String)
	}
	if month.Valid && month.String != "" {
		parts = append(parts, month.String)
	}
	if day.Valid && day.String != "" {
		parts = append(parts, day.String)
	}
	return strings.Join(parts, "-")
}
