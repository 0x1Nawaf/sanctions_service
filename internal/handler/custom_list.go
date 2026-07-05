package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

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
		recordType := entry.RecordType
		if recordType == "" {
			recordType = "individual"
		}

		recRes, err := tx.Exec(`
			INSERT INTO sanctions_records (record_type, gender, active_status, profile_notes, custom_list_id)
			VALUES (?, ?, 'active', ?, ?)`,
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
