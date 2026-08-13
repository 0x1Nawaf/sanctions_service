package handler

import (
	"database/sql"
	"log"
	"time"

	"github.com/nnn/sanctions-service/internal/scoring"
)

// LoadTokenWeights builds the name-frequency table ScoreNameV2 weights tokens
// with, and installs it.
//
// Rows are read ordered by record so that every name variant of a record can be
// folded into one document; counting rows instead would let a record carrying
// dozens of spelling variations inflate the apparent frequency of its own
// surname. idx_record_id serves the ordering.
//
// This is a full pass over sanctions_names and is meant to be called once at
// startup, off the request path, and again after a seeder run. On a
// multi-million-row feed it is minutes of work, which is why the caller runs it
// in the background and why ScoreNameV2 falls back to a static table of common
// name parts until it completes.
func LoadTokenWeights(db *sql.DB) error {
	start := time.Now()

	rows, err := queryRowsWithRetry(db, `
		SELECT sn.record_id,
		       sn.first_name, sn.middle_name, sn.surname,
		       sn.single_string_name, sn.entity_name, sn.original_script_name
		FROM sanctions_names sn
		INNER JOIN sanctions_records sr ON sr.id = sn.record_id
		WHERE sr.active_status = 'Active'
		ORDER BY sn.record_id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	builder := scoring.NewTokenWeightsBuilder()
	var currentID uint32
	var currentNames []string
	scanned := 0

	for rows.Next() {
		var recordID uint32
		var firstName, middleName, surname, singleStringName, entityName, originalScriptName sql.NullString
		if err := rows.Scan(&recordID, &firstName, &middleName, &surname,
			&singleStringName, &entityName, &originalScriptName); err != nil {
			continue
		}
		scanned++

		if recordID != currentID && len(currentNames) > 0 {
			builder.AddRecord(currentNames)
			currentNames = currentNames[:0]
		}
		currentID = recordID

		for _, c := range buildNameCandidates(recordID, firstName, middleName, surname,
			singleStringName, entityName, originalScriptName) {
			currentNames = append(currentNames, c.name)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(currentNames) > 0 {
		builder.AddRecord(currentNames)
	}

	weights := builder.Build()
	scoring.SetTokenWeights(weights)

	log.Printf("token weights loaded records=%d distinct_tokens=%d name_rows=%d in %s",
		weights.Documents(), weights.Distinct(), scanned, time.Since(start).Round(time.Millisecond))
	return nil
}
