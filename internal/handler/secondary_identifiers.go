package handler

import (
	"database/sql"
	"log"
	"time"

	"github.com/nnn/sanctions-service/internal/scoring"
)

// secondaryIdentifiers holds the date-of-birth and citizenship values for a
// shortlist of records, fetched in one query each.
type secondaryIdentifiers struct {
	dates       map[uint32][]scoring.PartialDate
	citizenship map[uint32][]string
}

func (s *secondaryIdentifiers) datesFor(recordID uint32) []scoring.PartialDate {
	if s == nil {
		return nil
	}
	return s.dates[recordID]
}

func (s *secondaryIdentifiers) citizenshipFor(recordID uint32) []string {
	if s == nil {
		return nil
	}
	return s.citizenship[recordID]
}

// fetchSecondaryIdentifiers loads dates of birth and citizenships for the given
// records. Both queries are served by idx_record_id and run against a shortlist
// bounded by factorEvalLimit, so this stays off the expensive part of the
// request.
//
// needDates and needCitizenship keep the service from querying for a factor the
// caller did not supply.
func (h *ScreenHandler) fetchSecondaryIdentifiers(ids []uint32, needDates, needCitizenship bool) (*secondaryIdentifiers, error) {
	out := &secondaryIdentifiers{
		dates:       make(map[uint32][]scoring.PartialDate),
		citizenship: make(map[uint32][]string),
	}
	if len(ids) == 0 {
		return out, nil
	}

	if needDates {
		inClause, args := uint32INClause(ids)
		rows, err := queryRowsWithRetry(h.db, `
			SELECT record_id, day, month, year
			FROM sanctions_dates
			WHERE record_id IN `+inClause+`
			  AND date_type = 'Date of Birth'
		`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var recordID uint32
			var day, month, year sql.NullString
			if err := rows.Scan(&recordID, &day, &month, &year); err != nil {
				continue
			}
			d := scoring.NewPartialDate(day.String, month.String, year.String)
			if !d.IsZero() {
				out.dates[recordID] = append(out.dates[recordID], d)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}

	if needCitizenship {
		inClause, args := uint32INClause(ids)
		// 'Nationality' is included because custom lists are written with that
		// country_type; the vendor feed uses 'Citizenship'. The feed's
		// 'Resident of' and 'Jurisdiction' rows are deliberately excluded —
		// living somewhere is not holding its nationality, and treating them
		// as equivalent would confirm matches that nothing supports.
		rows, err := queryRowsWithRetry(h.db, `
			SELECT record_id, country_code
			FROM sanctions_countries
			WHERE record_id IN `+inClause+`
			  AND country_type IN ('Citizenship', 'Nationality')
		`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var recordID uint32
			var code sql.NullString
			if err := rows.Scan(&recordID, &code); err != nil {
				continue
			}
			if code.Valid && code.String != "" {
				out.citizenship[recordID] = append(out.citizenship[recordID], code.String)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// LoadCountryIndex builds the resolver that maps a caller's ISO code or country
// name onto the feed's own country codes, and installs it.
//
// Without it every supplied citizenship resolves to nothing and the factor
// stays neutral, so a failure here degrades the feature rather than breaking
// screening.
func LoadCountryIndex(db *sql.DB) error {
	start := time.Now()

	rows, err := queryRowsWithRetry(db, `SELECT code, name FROM sanctions_ref_countries`)
	if err != nil {
		return err
	}
	defer rows.Close()

	codeToName := make(map[string]string)
	for rows.Next() {
		var code string
		var name sql.NullString
		if err := rows.Scan(&code, &name); err != nil {
			continue
		}
		codeToName[code] = name.String
	}
	if err := rows.Err(); err != nil {
		return err
	}

	idx := scoring.NewCountryIndex(codeToName)
	scoring.SetCountryIndex(idx)

	log.Printf("country index loaded countries=%d lookup_keys=%d in %s",
		len(codeToName), idx.Size(), time.Since(start).Round(time.Millisecond))
	return nil
}
