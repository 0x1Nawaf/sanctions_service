package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/nnn/sanctions-service/internal/model"
)

type CustomListHandler struct {
	db *sql.DB
}

func NewCustomListHandler(db *sql.DB) *CustomListHandler {
	return &CustomListHandler{db: db}
}

func (h *CustomListHandler) Upload(w http.ResponseWriter, r *http.Request) {
	var req model.CustomListUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "list name is required")
		return
	}
	if len(req.Entries) == 0 {
		writeError(w, http.StatusBadRequest, "at least one entry is required")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		"INSERT INTO custom_lists (name, description) VALUES (?, ?)",
		strings.TrimSpace(req.Name), req.Description,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create list")
		return
	}
	listID, _ := res.LastInsertId()

	entriesAdded := 0
	for _, entry := range req.Entries {
		recordType := normalizeRecordType(entry.RecordType)

		recRes, err := tx.Exec(`
			INSERT INTO sanctions_records (record_type, gender, active_status, profile_notes, custom_list_id)
			VALUES (?, ?, 'Active', ?, ?)`,
			recordType,
			nullIfEmpty(entry.Gender),
			nullIfEmpty(entry.Notes),
			listID,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to insert record")
			return
		}
		recordID, _ := recRes.LastInsertId()

		if err := h.insertName(tx, uint32(recordID), "Primary Name", entry.FirstName, entry.MiddleName, entry.Surname, entry.EntityName); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to insert name")
			return
		}

		for _, alias := range entry.Aliases {
			if err := h.insertName(tx, uint32(recordID), "Also Known As", alias.FirstName, alias.MiddleName, alias.Surname, alias.EntityName); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to insert alias")
				return
			}
		}

		if entry.DateOfBirth != "" {
			tx.Exec(
				"INSERT INTO sanctions_dates (record_id, date_type, year) VALUES (?, 'Date of Birth', ?)",
				recordID, entry.DateOfBirth,
			)
		}

		if entry.Nationality != "" {
			tx.Exec(
				"INSERT INTO sanctions_countries (record_id, country_type, country_code) VALUES (?, 'Nationality', ?)",
				recordID, entry.Nationality,
			)
		}

		if entry.IDType != "" && entry.IDValue != "" {
			tx.Exec(
				"INSERT INTO sanctions_id_numbers (record_id, id_type, id_value) VALUES (?, ?, ?)",
				recordID, entry.IDType, entry.IDValue,
			)
		}

		entriesAdded++
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.CustomListUploadResponse{
		ListID:       uint32(listID),
		Name:         strings.TrimSpace(req.Name),
		EntriesAdded: entriesAdded,
		Message:      "custom list uploaded successfully",
	})
}

func (h *CustomListHandler) insertName(tx *sql.Tx, recordID uint32, nameType, firstName, middleName, surname, entityName string) error {
	_, err := tx.Exec(`
		INSERT INTO sanctions_names (record_id, name_type, first_name, middle_name, surname, entity_name)
		VALUES (?, ?, ?, ?, ?, ?)`,
		recordID, nameType,
		nullIfEmpty(firstName),
		nullIfEmpty(middleName),
		nullIfEmpty(surname),
		nullIfEmpty(entityName),
	)
	return err
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// normalizeRecordType maps upload values to the same record_type values used by
// the official sanctions feed ("Person" or "Entity").
func normalizeRecordType(recordType string) string {
	switch strings.ToLower(strings.TrimSpace(recordType)) {
	case "entity":
		return "Entity"
	default:
		return "Person"
	}
}

func applyCustomListMeta(rec *model.SanctionsRecord, customListID sql.NullInt64, listName string) {
	if customListID.Valid {
		clID := uint32(customListID.Int64)
		rec.CustomListID = &clID
		rec.CustomListName = listName
		if listName != "" {
			rec.Source = "custom_list:" + listName
		} else {
			rec.Source = "custom_list"
		}
	} else {
		rec.Source = "sanctions_list"
	}
	if rec.RecordType.Valid {
		rec.RecordType.String = normalizeRecordType(rec.RecordType.String)
	}
}

func (h *CustomListHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT cl.id, cl.name, COALESCE(cl.description, ''), cl.created_at, cl.updated_at,
		       COUNT(sr.id) AS entry_count
		FROM custom_lists cl
		LEFT JOIN sanctions_records sr ON sr.custom_list_id = cl.id
		GROUP BY cl.id
		ORDER BY cl.created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	lists := make([]model.CustomListSummary, 0)
	for rows.Next() {
		var item model.CustomListSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.CreatedAt, &item.UpdatedAt, &item.EntryCount); err != nil {
			continue
		}
		lists = append(lists, item)
	}

	writeJSON(w, lists)
}

func (h *CustomListHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid list id")
		return
	}

	var exists bool
	h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM custom_lists WHERE id = ?)", id).Scan(&exists)
	if !exists {
		writeError(w, http.StatusNotFound, "list not found")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()

	tx.Exec("DELETE FROM sanctions_records WHERE custom_list_id = ?", id)
	tx.Exec("DELETE FROM custom_lists WHERE id = ?", id)

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete list")
		return
	}

	writeJSON(w, model.CustomListDeleteResponse{
		Message: "list deleted successfully",
	})
}
