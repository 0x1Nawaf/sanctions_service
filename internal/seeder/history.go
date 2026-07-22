package seeder

import (
	"fmt"
	"log"
	"strings"
	"time"
)

const recordEventBatchSize = 500

type changeKey struct {
	changeType string
	entity     string
	recordType string
}

type recordChangeEvent struct {
	recordID     uint32
	changeType   string
	recordType   string
	activeStatus string
	action       string
	actionDate   string
	gender       string
	deceased     string
}

type seedHistory struct {
	runID        uint64
	counts       map[changeKey]int
	recordEvents []recordChangeEvent
}

func (s *Seeder) beginSeedRun(jsonPath string, startedAt time.Time) error {
	res, err := s.db.Exec(
		`INSERT INTO seed_runs (started_at, json_source, status) VALUES (?, ?, 'running')`,
		startedAt, nullIfEmpty(jsonPath),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.history = &seedHistory{
		runID:  uint64(id),
		counts: make(map[changeKey]int),
	}
	return nil
}

func (s *Seeder) addChange(changeType, entity, recordType string, delta int) {
	if s.history == nil || delta == 0 {
		return
	}
	rt := strings.TrimSpace(recordType)
	key := changeKey{changeType: changeType, entity: entity, recordType: rt}
	s.history.counts[key] += delta
}

func (s *Seeder) addRecordEventFromRow(id uint32, changeType, recordType string, row map[string]interface{}) {
	if s.history == nil {
		return
	}
	s.history.recordEvents = append(s.history.recordEvents, recordChangeEvent{
		recordID:     id,
		changeType:   changeType,
		recordType:   recordType,
		activeStatus: strString(row, "active_status"),
		action:       strString(row, "action"),
		actionDate:   strString(row, "action_date"),
		gender:       strString(row, "gender"),
		deceased:     strString(row, "deceased"),
	})
}

func (s *Seeder) addRecordEventFromExisting(id uint32, changeType string, ex existingRecord) {
	if s.history == nil {
		return
	}
	s.history.recordEvents = append(s.history.recordEvents, recordChangeEvent{
		recordID:     id,
		changeType:   changeType,
		recordType:   ex.recordType,
		activeStatus: ex.activeStatus,
		action:       ex.action,
		actionDate:   ex.actionDate,
		gender:       ex.gender,
		deceased:     ex.deceased,
	})
}

func (s *Seeder) finishSeedRun(startedAt time.Time, runErr error) {
	if s.history == nil {
		return
	}

	status := "completed"
	if runErr != nil {
		status = "failed"
	}
	durationMs := time.Since(startedAt).Milliseconds()
	completedAt := time.Now()

	_, err := s.db.Exec(
		`UPDATE seed_runs SET completed_at = ?, status = ?, duration_ms = ? WHERE id = ?`,
		completedAt, status, durationMs, s.history.runID,
	)
	if err != nil {
		log.Printf("WARN: update seed_run: %v", err)
	}

	if runErr != nil {
		return
	}

	for key, count := range s.history.counts {
		if count <= 0 {
			continue
		}
		_, err := s.db.Exec(
			`INSERT INTO seed_run_changes (seed_run_id, change_type, entity, record_type, count) VALUES (?, ?, ?, ?, ?)`,
			s.history.runID, key.changeType, key.entity, nullIfEmpty(key.recordType), count,
		)
		if err != nil {
			log.Printf("WARN: insert seed_run_change: %v", err)
		}
	}
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

type existingRecord struct {
	recordType   string
	activeStatus string
	action       string
	actionDate   string
	gender       string
	deceased     string
	profileNotes string
}

func (s *Seeder) loadExistingOfficialRecords() (map[uint32]existingRecord, error) {
	query := `
		SELECT id, COALESCE(record_type, ''), COALESCE(active_status, ''), COALESCE(action, ''),
		       COALESCE(action_date, ''), COALESCE(gender, ''), COALESCE(deceased, ''), COALESCE(profile_notes, '')
		FROM sanctions_records
		WHERE custom_list_id IS NULL`

	rows, err := s.db.Query(query)
	if err != nil && strings.Contains(err.Error(), "Unknown column") {
		query = `
			SELECT id, COALESCE(record_type, ''), COALESCE(active_status, ''), COALESCE(action, ''),
			       COALESCE(action_date, ''), COALESCE(gender, ''), COALESCE(deceased, ''), COALESCE(profile_notes, '')
			FROM sanctions_records`
		rows, err = s.db.Query(query)
	}
	if err != nil {
		if strings.Contains(err.Error(), "doesn't exist") {
			return map[uint32]existingRecord{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	out := make(map[uint32]existingRecord)
	for rows.Next() {
		var id uint32
		var rec existingRecord
		if err := rows.Scan(&id, &rec.recordType, &rec.activeStatus, &rec.action, &rec.actionDate, &rec.gender, &rec.deceased, &rec.profileNotes); err != nil {
			return nil, err
		}
		out[id] = rec
	}
	return out, rows.Err()
}

func isActiveStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "active")
}

func recordRowChanged(ex existingRecord, row map[string]interface{}) bool {
	return ex.recordType != strString(row, "record_type") ||
		ex.action != strString(row, "action") ||
		ex.actionDate != strString(row, "action_date") ||
		ex.gender != strString(row, "gender") ||
		ex.activeStatus != strString(row, "active_status") ||
		ex.deceased != strString(row, "deceased") ||
		ex.profileNotes != strString(row, "profile_notes")
}

func strString(row map[string]interface{}, key string) string {
	v := str(row, key)
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func (s *Seeder) classifyRecordChange(id uint32, ex existingRecord, row map[string]interface{}, recordType string) {
	newActive := strString(row, "active_status")
	if !isActiveStatus(ex.activeStatus) && isActiveStatus(newActive) {
		s.addChange("reactivated", "sanctions_record", recordType, 1)
		s.addRecordEventFromRow(id, "reactivated", recordType, row)
	} else if isActiveStatus(ex.activeStatus) && !isActiveStatus(newActive) {
		s.addChange("inactivated", "sanctions_record", recordType, 1)
		s.addRecordEventFromRow(id, "inactivated", recordType, row)
	}
	if recordRowChanged(ex, row) {
		s.addChange("updated", "sanctions_record", recordType, 1)
		s.addRecordEventFromRow(id, "updated", recordType, row)
	}
}
