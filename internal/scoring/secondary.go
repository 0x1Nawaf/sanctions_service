package scoring

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

// Secondary identifiers — date of birth and citizenship — adjust a name score
// once a name match has been found. They are never used to retrieve or to
// pre-filter candidates.
//
// Three rules govern all of them, and they are what make the feature safe:
//
//  1. Absence is neutral. Only 57% of persons in the feed carry a date of
//     birth. Penalising a missing one would hide real hits behind gaps in the
//     vendor's data rather than behind anything about the person.
//  2. Confirmation lifts, contradiction lowers. A confirmed identifier is the
//     evidence a name alone cannot supply, and it is what rescues a match built
//     from very common name parts.
//  3. Contradiction is a review note, not a verdict. Feed dates are frequently
//     approximate and citizenship is under-recorded, so a contradiction reduces
//     confidence without silently discarding the alert.

// Adjustments applied to a name score, in points.
const (
	DOBExactAdjustment  = 10
	DOBMonthAdjustment  = 8
	DOBYearAdjustment   = 6
	DOBNearAdjustment   = 3
	DOBSlightMismatch   = -10
	DOBStrongMismatch   = -25
	CitizenshipMatch    = 6
	CitizenshipMismatch = -12

	// MaxFactorBoost is the largest possible positive adjustment. Screening
	// widens its shortlist by this much before applying the factors, so a
	// record that a confirmed identifier would promote above the threshold is
	// still in hand when the adjustment is made.
	MaxFactorBoost = DOBExactAdjustment + CitizenshipMatch
)

// Status values reported alongside each adjustment.
const (
	// StatusNotSupplied — the caller sent no value for this factor.
	StatusNotSupplied = "not_supplied"
	// StatusUnavailable — the record carries no value for this factor.
	StatusUnavailable = "unavailable"
	// StatusUnresolved — the caller's value could not be interpreted.
	StatusUnresolved = "unresolved"

	StatusConfirmedExact = "confirmed_exact"
	StatusConfirmedMonth = "confirmed_month"
	StatusConfirmedYear  = "confirmed_year"
	StatusNear           = "near"
	StatusConfirmed      = "confirmed"
	StatusContradicted   = "contradicted"
)

// PartialDate is a date that may be missing its day, or its day and month.
//
// Nearly half the birth dates in the feed are a bare year, so a type that
// insisted on a complete date would have to discard them. Zero means absent.
type PartialDate struct {
	Year  int
	Month int
	Day   int
}

func (d PartialDate) IsZero() bool { return d.Year == 0 }

func (d PartialDate) String() string {
	switch {
	case d.Year == 0:
		return ""
	case d.Month == 0:
		return fmt.Sprintf("%04d", d.Year)
	case d.Day == 0:
		return fmt.Sprintf("%04d-%02d", d.Year, d.Month)
	default:
		return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
	}
}

var monthNames = map[string]int{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3,
	"apr": 4, "april": 4, "may": 5, "jun": 6, "june": 6,
	"jul": 7, "july": 7, "aug": 8, "august": 8, "sep": 9, "sept": 9, "september": 9,
	"oct": 10, "october": 10, "nov": 11, "november": 11, "dec": 12, "december": 12,
}

// ParsePartialDate reads the date formats a caller or the feed may supply:
// "1985", "1985-03", "1985-03-12" and the feed's own "12-Mar-1985".
//
// A purely numeric day-first date such as "03/04/1985" is rejected rather than
// guessed at: it is 3 April in most of the world and 4 March in the United
// States, and silently choosing one would produce a confident comparison
// against a date the caller never meant.
func ParsePartialDate(s string) (PartialDate, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return PartialDate{}, nil
	}

	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '/' || r == '.' || r == ' '
	})

	switch len(parts) {
	case 1:
		year, err := parseYear(parts[0])
		if err != nil {
			return PartialDate{}, err
		}
		return PartialDate{Year: year}, nil

	case 2:
		if year, err := parseYear(parts[0]); err == nil {
			month, err := parseMonth(parts[1])
			if err != nil {
				return PartialDate{}, err
			}
			return PartialDate{Year: year, Month: month}, nil
		}
		year, err := parseYear(parts[1])
		if err != nil {
			return PartialDate{}, fmt.Errorf("date %q: expected a four-digit year", s)
		}
		month, err := parseMonth(parts[0])
		if err != nil {
			return PartialDate{}, err
		}
		return PartialDate{Year: year, Month: month}, nil

	case 3:
		// Year first: unambiguous ISO ordering.
		if year, err := parseYear(parts[0]); err == nil {
			month, err := parseMonth(parts[1])
			if err != nil {
				return PartialDate{}, err
			}
			day, err := parseDay(parts[2])
			if err != nil {
				return PartialDate{}, err
			}
			return PartialDate{Year: year, Month: month, Day: day}, nil
		}
		// Year last: only accepted when the month is spelled out, which is what
		// makes the day/month order unambiguous.
		year, err := parseYear(parts[2])
		if err != nil {
			return PartialDate{}, fmt.Errorf("date %q: expected a four-digit year", s)
		}
		if !isAlphabetic(parts[1]) {
			return PartialDate{}, fmt.Errorf(
				"date %q is ambiguous: use YYYY-MM-DD, or spell the month as in 12-Mar-1985", s)
		}
		month, err := parseMonth(parts[1])
		if err != nil {
			return PartialDate{}, err
		}
		day, err := parseDay(parts[0])
		if err != nil {
			return PartialDate{}, err
		}
		return PartialDate{Year: year, Month: month, Day: day}, nil
	}

	return PartialDate{}, fmt.Errorf("date %q: unrecognised format", s)
}

// NewPartialDate builds a date from the feed's separate day/month/year columns,
// any of which may be empty.
func NewPartialDate(day, month, year string) PartialDate {
	var d PartialDate
	if y, err := parseYear(strings.TrimSpace(year)); err == nil {
		d.Year = y
	} else {
		return PartialDate{}
	}
	if m, err := parseMonth(strings.TrimSpace(month)); err == nil {
		d.Month = m
	}
	if day, err := parseDay(strings.TrimSpace(day)); err == nil {
		d.Day = day
	}
	return d
}

func parseYear(s string) (int, error) {
	if len(s) != 4 {
		return 0, fmt.Errorf("year %q: expected four digits", s)
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1000 {
		return 0, fmt.Errorf("year %q: not a valid year", s)
	}
	return n, nil
}

func parseMonth(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty month")
	}
	if n, ok := monthNames[strings.ToLower(s)]; ok {
		return n, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 12 {
		return 0, fmt.Errorf("month %q: not a valid month", s)
	}
	return n, nil
}

func parseDay(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 31 {
		return 0, fmt.Errorf("day %q: not a valid day", s)
	}
	return n, nil
}

func isAlphabetic(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// CompareDOB scores a supplied date of birth against every date the record
// carries, and returns the best outcome. Records routinely list several — one
// holds fourteen — because sources disagree, and any one of them agreeing is
// what matters.
//
// Comparison happens only at the granularity both sides carry. A record holding
// a bare year is compared on the year alone rather than being treated as a
// mismatch for the day it never had.
func CompareDOB(query PartialDate, records []PartialDate) (status string, adjustment int, matched PartialDate) {
	if query.IsZero() {
		return StatusNotSupplied, 0, PartialDate{}
	}
	if len(records) == 0 {
		return StatusUnavailable, 0, PartialDate{}
	}

	best := StatusContradicted
	bestAdjustment := DOBStrongMismatch
	var bestDate PartialDate
	found := false

	for _, record := range records {
		if record.IsZero() {
			continue
		}
		s, adj := compareOneDOB(query, record)
		if !found || adj > bestAdjustment {
			best, bestAdjustment, bestDate, found = s, adj, record, true
		}
	}
	if !found {
		return StatusUnavailable, 0, PartialDate{}
	}
	return best, bestAdjustment, bestDate
}

func compareOneDOB(query, record PartialDate) (string, int) {
	if query.Year != record.Year {
		diff := query.Year - record.Year
		if diff < 0 {
			diff = -diff
		}
		switch {
		case diff == 1:
			// Not slack. Gulf clients frequently hold a Hijri date of birth,
			// and converting between the Hijri and Gregorian calendars lands a
			// year either side often enough that treating it as a mismatch
			// would suppress genuine matches.
			return StatusNear, DOBNearAdjustment
		case diff <= 3:
			return StatusContradicted, DOBSlightMismatch
		default:
			return StatusContradicted, DOBStrongMismatch
		}
	}

	if query.Month != 0 && record.Month != 0 {
		if query.Month != record.Month {
			return StatusContradicted, DOBSlightMismatch
		}
		if query.Day != 0 && record.Day != 0 {
			if query.Day != record.Day {
				return StatusContradicted, DOBSlightMismatch
			}
			return StatusConfirmedExact, DOBExactAdjustment
		}
		return StatusConfirmedMonth, DOBMonthAdjustment
	}

	return StatusConfirmedYear, DOBYearAdjustment
}

// CompareCitizenship scores supplied citizenships against the record's.
//
// Any-of on both sides: dual nationality is common, a record may list up to
// four, and one agreement is enough. The mismatch penalty is deliberately
// milder than the date-of-birth one because citizenship is the more
// under-recorded of the two.
func CompareCitizenship(query, record []string) (status string, adjustment int, matched string) {
	if len(query) == 0 {
		return StatusNotSupplied, 0, ""
	}
	if len(record) == 0 {
		return StatusUnavailable, 0, ""
	}
	for _, q := range query {
		if q == "" {
			continue
		}
		for _, r := range record {
			if q == r {
				return StatusConfirmed, CitizenshipMatch, r
			}
		}
	}
	return StatusContradicted, CitizenshipMismatch, strings.Join(record, ",")
}

// CountryIndex resolves what a caller writes — an ISO code, a feed code, or a
// country name — to the feed's own country code, which is the only form the
// records are stored in.
//
// The feed does not use ISO 3166: Saudi Arabia is SAARAB, Bahrain is BAHRN,
// Yemen is YEMAR. Comparing a caller's "SA" against those directly would
// contradict every record it should confirm.
type CountryIndex struct {
	byKey map[string]string
}

// NewCountryIndex builds the resolver from the feed's country reference table,
// keyed by both code and name, plus the ISO aliases below.
func NewCountryIndex(codeToName map[string]string) *CountryIndex {
	idx := &CountryIndex{byKey: make(map[string]string, len(codeToName)*3)}
	for code, name := range codeToName {
		canonical := strings.ToUpper(strings.TrimSpace(code))
		if canonical == "" {
			continue
		}
		idx.byKey[countryKey(code)] = canonical
		if name != "" {
			idx.byKey[countryKey(name)] = canonical
		}
	}
	for alias, name := range isoCountryAliases {
		if canonical, ok := idx.byKey[countryKey(name)]; ok {
			if _, taken := idx.byKey[countryKey(alias)]; !taken {
				idx.byKey[countryKey(alias)] = canonical
			}
		}
	}
	return idx
}

// Resolve returns the feed's code for a caller-supplied country, or "" when it
// cannot be interpreted. An unresolvable value is reported as unresolved and
// left neutral rather than being compared as a literal, which would contradict
// every record it touched.
func (c *CountryIndex) Resolve(input string) string {
	if c == nil {
		return ""
	}
	return c.byKey[countryKey(input)]
}

// Size reports how many lookup keys the index holds.
func (c *CountryIndex) Size() int {
	if c == nil {
		return 0
	}
	return len(c.byKey)
}

func countryKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var activeCountryIndex atomic.Pointer[CountryIndex]

// SetCountryIndex installs the resolver. Safe to call while screening is in
// flight, so it can be rebuilt after a seeder run.
func SetCountryIndex(idx *CountryIndex) { activeCountryIndex.Store(idx) }

// ResolveCountry maps a caller-supplied country onto the feed's code.
func ResolveCountry(input string) string {
	return activeCountryIndex.Load().Resolve(input)
}

// CountryIndexLoaded reports whether a resolver has been installed. Without one
// every supplied citizenship resolves to nothing and the factor stays neutral.
func CountryIndexLoaded() bool { return activeCountryIndex.Load() != nil }

// isoCountryAliases maps ISO 3166 alpha-2 and alpha-3 codes onto the country
// names the feed uses, so callers can send the codes their own systems hold.
//
// It covers the nationalities that actually appear in a Gulf client book rather
// than all 249 ISO entries; anything absent still resolves by name. Extend it
// as new nationalities appear — an unresolved value is reported as such in the
// response, so gaps are visible rather than silent.
var isoCountryAliases = map[string]string{
	"SA": "Saudi Arabia", "SAU": "Saudi Arabia",
	"AE": "United Arab Emirates", "ARE": "United Arab Emirates",
	"KW": "Kuwait", "KWT": "Kuwait",
	"QA": "Qatar", "QAT": "Qatar",
	"BH": "Bahrain", "BHR": "Bahrain",
	"OM": "Oman", "OMN": "Oman",
	"YE": "Yemen", "YEM": "Yemen",
	"EG": "Egypt", "EGY": "Egypt",
	"JO": "Jordan", "JOR": "Jordan",
	"SY": "Syria", "SYR": "Syria",
	"IQ": "Iraq", "IRQ": "Iraq",
	"IR": "Iran", "IRN": "Iran",
	"LB": "Lebanon", "LBN": "Lebanon",
	"PS": "Palestine", "PSE": "Palestine",
	"SD": "Sudan", "SDN": "Sudan",
	"SS": "South Sudan", "SSD": "South Sudan",
	"LY": "Libya", "LBY": "Libya",
	"TN": "Tunisia", "TUN": "Tunisia",
	"DZ": "Algeria", "DZA": "Algeria",
	"MA": "Morocco", "MAR": "Morocco",
	"SO": "Somalia", "SOM": "Somalia",
	"DJ": "Djibouti", "DJI": "Djibouti",
	"ER": "Eritrea", "ERI": "Eritrea",
	"ET": "Ethiopia", "ETH": "Ethiopia",
	"TR": "Turkey", "TUR": "Turkey",
	"AF": "Afghanistan", "AFG": "Afghanistan",
	"PK": "Pakistan", "PAK": "Pakistan",
	"IN": "India", "IND": "India",
	"BD": "Bangladesh", "BGD": "Bangladesh",
	"LK": "Sri Lanka", "LKA": "Sri Lanka",
	"NP": "Nepal", "NPL": "Nepal",
	"ID": "Indonesia", "IDN": "Indonesia",
	"MY": "Malaysia", "MYS": "Malaysia",
	"PH": "Philippines", "PHL": "Philippines",
	"CN": "China", "CHN": "China",
	"US": "United States", "USA": "United States",
	"GB": "United Kingdom", "GBR": "United Kingdom",
	"FR": "France", "FRA": "France",
	"DE": "Germany", "DEU": "Germany",
	"IT": "Italy", "ITA": "Italy",
	"ES": "Spain", "ESP": "Spain",
	"NL": "Netherlands", "NLD": "Netherlands",
	"CH": "Switzerland", "CHE": "Switzerland",
	"RU": "Russia", "RUS": "Russia",
	"NG": "Nigeria", "NGA": "Nigeria",
	"KE": "Kenya", "KEN": "Kenya",
	"ZA": "South Africa", "ZAF": "South Africa",
}
