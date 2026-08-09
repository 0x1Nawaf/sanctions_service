package handler

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/nnn/sanctions-service/internal/model"
)

func uint32INClause(ids []uint32) (string, []interface{}) {
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return fmt.Sprintf("(%s)", strings.Join(placeholders, ",")), args
}

// batchLoadOptions controls how related record data is loaded in bulk.
// A nameLimit or imageLimit of zero or less means "no limit".
type batchLoadOptions struct {
	nameLimit   int
	imageLimit  int
	slim        bool
	skipDetails bool
}

// recordDedupe collapses repeated child rows per record.
//
// The seeder appends child rows with INSERT IGNORE, and every child table
// except sanctions_associations carries no unique key for that to fire on. A
// record that appears in more than one feed therefore accumulates a fresh copy
// of each of its names, dates, countries and images, and the API hands those
// copies straight to the caller.
//
// Collapsing on content here also keeps the per-record limits meaningful: ten
// name rows should be ten different names, not the same one ten times.
type recordDedupe map[uint32]map[string]bool

// seen reports whether key was already recorded for recordID, and marks it.
func (d recordDedupe) seen(recordID uint32, key string) bool {
	perRecord, ok := d[recordID]
	if !ok {
		perRecord = make(map[string]bool)
		d[recordID] = perRecord
	}
	if perRecord[key] {
		return true
	}
	perRecord[key] = true
	return false
}

// dedupeKey joins values with a separator that cannot occur in the data, so
// distinct field combinations cannot collide into one key.
func dedupeKey(values ...string) string {
	return strings.Join(values, "\x00")
}

func nullText(ns model.NullString) string {
	if !ns.Valid {
		return ""
	}
	return ns.String
}

// withinLimit reports whether another row may be added for recordID, counting
// it when so. A limit of zero or less is unlimited.
func withinLimit(counts map[uint32]int, recordID uint32, limit int) bool {
	if limit > 0 && counts[recordID] >= limit {
		return false
	}
	counts[recordID]++
	return true
}

// loadRecordsBatch loads sanctions records and related data using a fixed number
// of queries instead of per-record N+1 lookups.
func loadRecordsBatch(db *sql.DB, ids []uint32, opts batchLoadOptions) ([]model.SanctionsRecord, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	inClause, args := uint32INClause(ids)

	var query string
	if opts.slim {
		query = fmt.Sprintf(`
			SELECT sr.id, sr.record_type, sr.action, sr.action_date, sr.gender,
			       sr.active_status, sr.deceased,
			       sr.custom_list_id, COALESCE(cl.name, '')
			FROM sanctions_records sr
			LEFT JOIN custom_lists cl ON cl.id = sr.custom_list_id
			WHERE sr.id IN %s
		`, inClause)
	} else {
		query = fmt.Sprintf(`
			SELECT sr.id, sr.record_type, sr.action, sr.action_date, sr.gender,
			       sr.active_status, sr.deceased, sr.profile_notes,
			       sr.custom_list_id, COALESCE(cl.name, '')
			FROM sanctions_records sr
			LEFT JOIN custom_lists cl ON cl.id = sr.custom_list_id
			WHERE sr.id IN %s
		`, inClause)
	}

	rows, err := queryRowsWithRetry(db, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recordMap := make(map[uint32]*model.SanctionsRecord, len(ids))
	records := make([]model.SanctionsRecord, 0, len(ids))
	for rows.Next() {
		var rec model.SanctionsRecord
		var customListID sql.NullInt64
		var listName string
		var err error
		if opts.slim {
			err = rows.Scan(&rec.ID, &rec.RecordType, &rec.Action, &rec.ActionDate,
				&rec.Gender, &rec.ActiveStatus, &rec.Deceased,
				&customListID, &listName)
		} else {
			err = rows.Scan(&rec.ID, &rec.RecordType, &rec.Action, &rec.ActionDate,
				&rec.Gender, &rec.ActiveStatus, &rec.Deceased, &rec.ProfileNotes,
				&customListID, &listName)
		}
		if err != nil {
			continue
		}
		applyCustomListMeta(&rec, customListID, listName)
		recordMap[rec.ID] = &rec
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := batchLoadRecordNames(db, recordMap, inClause, args, opts.nameLimit); err != nil {
		return nil, err
	}
	if opts.skipDetails {
		out := make([]model.SanctionsRecord, 0, len(records))
		for i := range records {
			if enriched, ok := recordMap[records[i].ID]; ok {
				out = append(out, *enriched)
			}
		}
		return out, nil
	}
	if err := batchLoadRecordDates(db, recordMap, inClause, args); err != nil {
		return nil, err
	}
	if err := batchLoadRecordCountries(db, recordMap, inClause, args); err != nil {
		return nil, err
	}
	if err := batchLoadRecordImages(db, recordMap, inClause, args, opts.imageLimit); err != nil {
		return nil, err
	}
	if err := batchLoadRecordDescriptions(db, recordMap, inClause, args); err != nil {
		return nil, err
	}
	if err := batchLoadRecordAssociations(db, recordMap, inClause, args); err != nil {
		return nil, err
	}

	out := make([]model.SanctionsRecord, 0, len(records))
	for i := range records {
		if enriched, ok := recordMap[records[i].ID]; ok {
			out = append(out, *enriched)
		}
	}
	return out, nil
}

func batchLoadRecordNames(db *sql.DB, recordMap map[uint32]*model.SanctionsRecord, inClause string, args []interface{}, limit int) error {
	query := fmt.Sprintf(`
		SELECT id, record_id, name_type, title_honorific, first_name, middle_name, surname,
		       maiden_name, suffix, single_string_name, original_script_name, entity_name
		FROM sanctions_names
		WHERE record_id IN %s
		ORDER BY record_id, id
	`, inClause)

	rows, err := queryRowsWithRetry(db, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	counts := make(map[uint32]int, len(recordMap))
	dedupe := make(recordDedupe, len(recordMap))
	for rows.Next() {
		var n model.SanctionsName
		if err := rows.Scan(&n.ID, &n.RecordID, &n.NameType, &n.TitleHonorific, &n.FirstName, &n.MiddleName,
			&n.Surname, &n.MaidenName, &n.Suffix, &n.SingleStringName, &n.OriginalScriptName, &n.EntityName); err != nil {
			continue
		}
		rec, ok := recordMap[n.RecordID]
		if !ok {
			continue
		}
		key := dedupeKey(nullText(n.NameType), nullText(n.TitleHonorific), nullText(n.FirstName),
			nullText(n.MiddleName), nullText(n.Surname), nullText(n.MaidenName), nullText(n.Suffix),
			nullText(n.SingleStringName), nullText(n.OriginalScriptName), nullText(n.EntityName))
		if dedupe.seen(n.RecordID, key) {
			continue
		}
		if !withinLimit(counts, n.RecordID, limit) {
			continue
		}
		rec.Names = append(rec.Names, n)
	}
	return rows.Err()
}

func batchLoadRecordDates(db *sql.DB, recordMap map[uint32]*model.SanctionsRecord, inClause string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT id, record_id, date_type, day, month, year, note
		FROM sanctions_dates
		WHERE record_id IN %s
		ORDER BY record_id, id
	`, inClause)

	rows, err := queryRowsWithRetry(db, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	dedupe := make(recordDedupe, len(recordMap))
	for rows.Next() {
		var d model.SanctionsDate
		if err := rows.Scan(&d.ID, &d.RecordID, &d.DateType, &d.Day, &d.Month, &d.Year, &d.Note); err != nil {
			continue
		}
		rec, ok := recordMap[d.RecordID]
		if !ok {
			continue
		}
		key := dedupeKey(nullText(d.DateType), nullText(d.Day), nullText(d.Month), nullText(d.Year), nullText(d.Note))
		if dedupe.seen(d.RecordID, key) {
			continue
		}
		rec.Dates = append(rec.Dates, d)
	}
	return rows.Err()
}

func batchLoadRecordCountries(db *sql.DB, recordMap map[uint32]*model.SanctionsRecord, inClause string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT sc.id, sc.record_id, sc.country_type, sc.country_code, rc.name
		FROM sanctions_countries sc
		LEFT JOIN sanctions_ref_countries rc ON rc.code = sc.country_code
		WHERE sc.record_id IN %s
		ORDER BY sc.record_id, sc.id
	`, inClause)

	rows, err := queryRowsWithRetry(db, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	dedupe := make(recordDedupe, len(recordMap))
	for rows.Next() {
		var c model.SanctionsCountry
		if err := rows.Scan(&c.ID, &c.RecordID, &c.CountryType, &c.CountryCode, &c.CountryName); err != nil {
			continue
		}
		rec, ok := recordMap[c.RecordID]
		if !ok {
			continue
		}
		if dedupe.seen(c.RecordID, dedupeKey(nullText(c.CountryType), nullText(c.CountryCode))) {
			continue
		}
		rec.Countries = append(rec.Countries, c)
	}
	return rows.Err()
}

func batchLoadRecordImages(db *sql.DB, recordMap map[uint32]*model.SanctionsRecord, inClause string, args []interface{}, limit int) error {
	query := fmt.Sprintf(`
		SELECT id, record_id, url
		FROM sanctions_images
		WHERE record_id IN %s
		ORDER BY record_id, id
	`, inClause)

	rows, err := queryRowsWithRetry(db, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	counts := make(map[uint32]int, len(recordMap))
	dedupe := make(recordDedupe, len(recordMap))
	for rows.Next() {
		var img model.SanctionsImage
		if err := rows.Scan(&img.ID, &img.RecordID, &img.URL); err != nil {
			continue
		}
		rec, ok := recordMap[img.RecordID]
		if !ok {
			continue
		}
		if dedupe.seen(img.RecordID, nullText(img.URL)) {
			continue
		}
		if !withinLimit(counts, img.RecordID, limit) {
			continue
		}
		rec.Images = append(rec.Images, img)
	}
	return rows.Err()
}

func batchLoadRecordDescriptions(db *sql.DB, recordMap map[uint32]*model.SanctionsRecord, inClause string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT sd.record_id, d1.name, d2.name, d3.name
		FROM sanctions_descriptions sd
		LEFT JOIN sanctions_ref_description1 d1 ON d1.description1_id = sd.description1_id
		LEFT JOIN sanctions_ref_description2 d2 ON d2.description2_id = sd.description2_id
		LEFT JOIN sanctions_ref_description3 d3 ON d3.description3_id = sd.description3_id
		WHERE sd.record_id IN %s
		ORDER BY sd.record_id, sd.id
	`, inClause)

	rows, err := queryRowsWithRetry(db, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	dedupe := make(recordDedupe, len(recordMap))
	for rows.Next() {
		var recordID uint32
		var d model.SanctionsDescriptionDetail
		if err := rows.Scan(&recordID, &d.Description1, &d.Description2, &d.Description3); err != nil {
			continue
		}
		rec, ok := recordMap[recordID]
		if !ok {
			continue
		}
		key := dedupeKey(nullText(d.Description1), nullText(d.Description2), nullText(d.Description3))
		if dedupe.seen(recordID, key) {
			continue
		}
		rec.Descriptions = append(rec.Descriptions, d)
	}
	return rows.Err()
}

// batchLoadRecordAssociations resolves each associate to a single display name.
//
// Joining on name_type alone duplicates the association once per matching name
// row, and a record routinely carries several rows marked "Primary Name" — the
// Latin spelling and the original-script spelling are separate rows. Matching
// on the lowest id picks one deterministically, and idx_primary_lookup
// (record_id, name_type, id) serves that as an index lookup.
func associationQuery(inClause string) string {
	return fmt.Sprintf(`
		SELECT sa.record_id, sa.associate_id,
		       COALESCE(sn.entity_name, sn.single_string_name, CONCAT_WS(' ', sn.first_name, sn.middle_name, sn.surname), '') AS associate_name,
		       rr.name,
		       sa.is_ex
		FROM sanctions_associations sa
		LEFT JOIN sanctions_names sn ON sn.id = (
		    SELECT MIN(pn.id)
		    FROM sanctions_names pn
		    WHERE pn.record_id = sa.associate_id AND pn.name_type = 'Primary Name'
		)
		LEFT JOIN sanctions_ref_relationships rr ON rr.code = sa.relationship_code
		WHERE sa.record_id IN %s
		ORDER BY sa.record_id, sa.id
	`, inClause)
}

func batchLoadRecordAssociations(db *sql.DB, recordMap map[uint32]*model.SanctionsRecord, inClause string, args []interface{}) error {
	rows, err := queryRowsWithRetry(db, associationQuery(inClause), args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	dedupe := make(recordDedupe, len(recordMap))
	for rows.Next() {
		var recordID uint32
		var a model.SanctionsAssociationDetail
		if err := rows.Scan(&recordID, &a.AssociateID, &a.AssociateName, &a.Relationship, &a.IsEx); err != nil {
			continue
		}
		rec, ok := recordMap[recordID]
		if !ok {
			continue
		}
		key := dedupeKey(strconv.FormatUint(uint64(a.AssociateID), 10), nullText(a.Relationship))
		if dedupe.seen(recordID, key) {
			continue
		}
		rec.Associations = append(rec.Associations, a)
	}
	return rows.Err()
}
