package handler

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nnn/sanctions-service/internal/model"
)

type HistoricalUpdatesHandler struct {
	db *sql.DB
}

func NewHistoricalUpdatesHandler(db *sql.DB) *HistoricalUpdatesHandler {
	return &HistoricalUpdatesHandler{db: db}
}

func (h *HistoricalUpdatesHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 25
	}
	offset := (page - 1) * perPage

	var total int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM seed_runs`).Scan(&total); err != nil {
		if isMissingTable(err) {
			writeJSON(w, model.HistoricalUpdatesResponse{
				Page: page, PerPage: perPage, Total: 0, Data: []model.HistoricalUpdateEntry{},
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to count seed runs")
		return
	}

	rows, err := h.db.Query(`
		SELECT id, started_at, completed_at, json_source, status, duration_ms
		FROM seed_runs
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?`, perPage, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list seed runs")
		return
	}
	defer rows.Close()

	type runRow struct {
		entry model.HistoricalUpdateEntry
		start time.Time
	}
	var runs []runRow

	for rows.Next() {
		var id uint64
		var startedAt time.Time
		var completedAt sql.NullTime
		var jsonSource sql.NullString
		var status string
		var durationMs sql.NullInt64

		if err := rows.Scan(&id, &startedAt, &completedAt, &jsonSource, &status, &durationMs); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read seed run")
			return
		}

		entry := model.HistoricalUpdateEntry{
			ID:       id,
			SeededAt: startedAt.UTC().Format(time.RFC3339),
			Status:   status,
		}
		if completedAt.Valid {
			entry.CompletedAt = completedAt.Time.UTC().Format(time.RFC3339)
		}
		if jsonSource.Valid {
			entry.JSONSource = jsonSource.String
		}
		if durationMs.Valid && durationMs.Int64 >= 0 {
			ms := uint64(durationMs.Int64)
			entry.DurationMs = &ms
		}

		runs = append(runs, runRow{entry: entry, start: startedAt})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list seed runs")
		return
	}

	for i := range runs {
		includeRecords := r.URL.Query().Get("include_records") != "false"
		recordsLimit, _ := strconv.Atoi(r.URL.Query().Get("records_limit"))
		if recordsLimit < 1 {
			recordsLimit = 100
		}
		if recordsLimit > 500 {
			recordsLimit = 500
		}

		changes, affected, err := h.loadChanges(runs[i].entry.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load seed changes")
			return
		}
		runs[i].entry.Changes = changes
		runs[i].entry.TotalRecordsAffected = affected

		if includeRecords {
			records, recTotal, err := h.loadRecordChanges(runs[i].entry.ID, recordsLimit)
			if err != nil {
				if !isMissingTable(err) {
					writeError(w, http.StatusInternalServerError, "failed to load record changes")
					return
				}
			} else {
				runs[i].entry.RecordChanges = records
				runs[i].entry.RecordChangesTotal = recTotal
			}
		}

		if i+1 < len(runs) {
			prev := runs[i+1].start
			cur := runs[i].start
			d := cur.Sub(prev)
			if d > 0 {
				runs[i].entry.IntervalSincePrevious = d.String()
				hours := math.Round(d.Hours()*100) / 100
				runs[i].entry.IntervalHours = &hours
			}
		}
	}

	data := make([]model.HistoricalUpdateEntry, len(runs))
	for i := range runs {
		data[i] = runs[i].entry
	}

	writeJSON(w, model.HistoricalUpdatesResponse{
		Page: page, PerPage: perPage, Total: total, Data: data,
	})
}

func (h *HistoricalUpdatesHandler) loadChanges(runID uint64) ([]model.SeedRunChange, int, error) {
	rows, err := h.db.Query(`
		SELECT change_type, entity, record_type, count
		FROM seed_run_changes
		WHERE seed_run_id = ?
		ORDER BY entity, change_type, record_type`, runID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var changes []model.SeedRunChange
	totalAffected := 0
	recordChangeTypes := map[string]bool{
		"added": true, "updated": true, "removed_from_feed": true,
		"inactivated": true, "reactivated": true,
	}

	for rows.Next() {
		var ch model.SeedRunChange
		var recordType sql.NullString
		var count int
		if err := rows.Scan(&ch.ChangeType, &ch.Entity, &recordType, &count); err != nil {
			return nil, 0, err
		}
		if recordType.Valid {
			ch.RecordType = recordType.String
		}
		ch.Count = count
		changes = append(changes, ch)

		if ch.Entity == "sanctions_record" && recordChangeTypes[ch.ChangeType] {
			totalAffected += count
		}
	}
	return changes, totalAffected, rows.Err()
}

func (h *HistoricalUpdatesHandler) loadRecordChanges(runID uint64, limit int) ([]model.SeedRunRecordChange, int, error) {
	var total int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM seed_run_record_changes WHERE seed_run_id = ?`, runID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := h.db.Query(`
		SELECT record_id, change_type, record_type, active_status, gender, action, action_date, deceased,
		       display_name, date_of_birth, countries_json
		FROM seed_run_record_changes
		WHERE seed_run_id = ?
		ORDER BY id
		LIMIT ?`, runID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []model.SeedRunRecordChange
	for rows.Next() {
		var rec model.SeedRunRecordChange
		var recordType, activeStatus, gender, action, actionDate, deceased sql.NullString
		var displayName, dateOfBirth sql.NullString
		var countriesJSON sql.NullString

		if err := rows.Scan(
			&rec.RecordID, &rec.ChangeType, &recordType, &activeStatus, &gender, &action, &actionDate, &deceased,
			&displayName, &dateOfBirth, &countriesJSON,
		); err != nil {
			return nil, 0, err
		}
		rec.RecordType = recordType.String
		rec.ActiveStatus = activeStatus.String
		rec.Gender = gender.String
		rec.Action = action.String
		rec.ActionDate = actionDate.String
		rec.Deceased = deceased.String
		rec.DisplayName = displayName.String
		rec.DateOfBirth = dateOfBirth.String
		if countriesJSON.Valid && countriesJSON.String != "" {
			_ = json.Unmarshal([]byte(countriesJSON.String), &rec.Countries)
		}
		out = append(out, rec)
	}
	return out, total, rows.Err()
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "1146")
}
