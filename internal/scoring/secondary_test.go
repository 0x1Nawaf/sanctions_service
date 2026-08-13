package scoring

import "testing"

func TestParsePartialDate(t *testing.T) {
	tests := []struct {
		input string
		want  PartialDate
	}{
		{"1985", PartialDate{Year: 1985}},
		{"1985-03", PartialDate{Year: 1985, Month: 3}},
		{"1985-03-12", PartialDate{Year: 1985, Month: 3, Day: 12}},
		{"1985/03/12", PartialDate{Year: 1985, Month: 3, Day: 12}},
		{"12-Mar-1985", PartialDate{Year: 1985, Month: 3, Day: 12}},
		{"12 March 1985", PartialDate{Year: 1985, Month: 3, Day: 12}},
		{"Mar-1985", PartialDate{Year: 1985, Month: 3}},
		{"", PartialDate{}},
	}
	for _, tt := range tests {
		got, err := ParsePartialDate(tt.input)
		if err != nil {
			t.Errorf("ParsePartialDate(%q) returned error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParsePartialDate(%q) = %+v, want %+v", tt.input, got, tt.want)
		}
	}
}

// TestParsePartialDateRejectsAmbiguous covers the case worth being strict
// about: 03/04/1985 is 3 April in most of the world and 4 March in the United
// States, and guessing would produce a confident comparison against a date the
// caller never meant.
func TestParsePartialDateRejectsAmbiguous(t *testing.T) {
	for _, input := range []string{"03/04/1985", "3-4-1985", "not a date", "85", "1985-13-01"} {
		if _, err := ParsePartialDate(input); err == nil {
			t.Errorf("ParsePartialDate(%q) succeeded, want an error", input)
		}
	}
}

func TestPartialDateString(t *testing.T) {
	tests := []struct {
		date PartialDate
		want string
	}{
		{PartialDate{Year: 1985}, "1985"},
		{PartialDate{Year: 1985, Month: 3}, "1985-03"},
		{PartialDate{Year: 1985, Month: 3, Day: 12}, "1985-03-12"},
		{PartialDate{}, ""},
	}
	for _, tt := range tests {
		if got := tt.date.String(); got != tt.want {
			t.Errorf("PartialDate%+v.String() = %q, want %q", tt.date, got, tt.want)
		}
	}
}

func TestNewPartialDateFromFeedColumns(t *testing.T) {
	// The feed stores day/month/year separately, with the month as three
	// letters, and leaves day and month empty on nearly half its rows.
	if got := NewPartialDate("22", "Jun", "1964"); got != (PartialDate{Year: 1964, Month: 6, Day: 22}) {
		t.Errorf("NewPartialDate(22, Jun, 1964) = %+v", got)
	}
	if got := NewPartialDate("", "", "1964"); got != (PartialDate{Year: 1964}) {
		t.Errorf("year-only row = %+v, want just the year", got)
	}
	if got := NewPartialDate("22", "Jun", ""); !got.IsZero() {
		t.Errorf("row without a year = %+v, want zero", got)
	}
}

func TestCompareDOB(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		records    []PartialDate
		wantStatus string
		wantAdj    int
	}{
		{
			name: "no date supplied is neutral", query: "",
			records:    []PartialDate{{Year: 1985}},
			wantStatus: StatusNotSupplied, wantAdj: 0,
		},
		{
			name: "record has no date is neutral", query: "1985-03-12",
			records:    nil,
			wantStatus: StatusUnavailable, wantAdj: 0,
		},
		{
			name: "full date agrees", query: "1985-03-12",
			records:    []PartialDate{{Year: 1985, Month: 3, Day: 12}},
			wantStatus: StatusConfirmedExact, wantAdj: DOBExactAdjustment,
		},
		{
			name: "record carries only a year", query: "1985-03-12",
			records:    []PartialDate{{Year: 1985}},
			wantStatus: StatusConfirmedYear, wantAdj: DOBYearAdjustment,
		},
		{
			name: "record has no day", query: "1985-03-12",
			records:    []PartialDate{{Year: 1985, Month: 3}},
			wantStatus: StatusConfirmedMonth, wantAdj: DOBMonthAdjustment,
		},
		{
			name: "one year apart is calendar-conversion drift", query: "1985",
			records:    []PartialDate{{Year: 1986}},
			wantStatus: StatusNear, wantAdj: DOBNearAdjustment,
		},
		{
			name: "a few years apart", query: "1985",
			records:    []PartialDate{{Year: 1988}},
			wantStatus: StatusContradicted, wantAdj: DOBSlightMismatch,
		},
		{
			name: "a different generation", query: "1985",
			records:    []PartialDate{{Year: 1955}},
			wantStatus: StatusContradicted, wantAdj: DOBStrongMismatch,
		},
		{
			name: "best of several listed dates wins", query: "1985-03-12",
			records: []PartialDate{
				{Year: 1955},
				{Year: 1985, Month: 3, Day: 12},
				{Year: 1970},
			},
			wantStatus: StatusConfirmedExact, wantAdj: DOBExactAdjustment,
		},
		{
			name: "same year, different month", query: "1985-03-12",
			records:    []PartialDate{{Year: 1985, Month: 9, Day: 12}},
			wantStatus: StatusContradicted, wantAdj: DOBSlightMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := ParsePartialDate(tt.query)
			if err != nil {
				t.Fatalf("parsing %q: %v", tt.query, err)
			}
			status, adjustment, _ := CompareDOB(query, tt.records)
			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}
			if adjustment != tt.wantAdj {
				t.Errorf("adjustment = %d, want %d", adjustment, tt.wantAdj)
			}
		})
	}
}

func TestCompareCitizenship(t *testing.T) {
	tests := []struct {
		name       string
		query      []string
		record     []string
		wantStatus string
		wantAdj    int
	}{
		{"nothing supplied", nil, []string{"SAARAB"}, StatusNotSupplied, 0},
		{"record has none", []string{"SAARAB"}, nil, StatusUnavailable, 0},
		{"agrees", []string{"SAARAB"}, []string{"SAARAB"}, StatusConfirmed, CitizenshipMatch},
		{"dual nationality on the record", []string{"SAARAB"}, []string{"UAE", "SAARAB"}, StatusConfirmed, CitizenshipMatch},
		{"dual nationality on the query", []string{"UK", "SAARAB"}, []string{"SAARAB"}, StatusConfirmed, CitizenshipMatch},
		{"disagrees", []string{"SAARAB"}, []string{"YEMAR"}, StatusContradicted, CitizenshipMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, adjustment, _ := CompareCitizenship(tt.query, tt.record)
			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}
			if adjustment != tt.wantAdj {
				t.Errorf("adjustment = %d, want %d", adjustment, tt.wantAdj)
			}
		})
	}
}

func TestCountryIndexResolves(t *testing.T) {
	// The feed does not use ISO 3166; these are its actual codes.
	idx := NewCountryIndex(map[string]string{
		"SAARAB": "Saudi Arabia",
		"UAE":    "United Arab Emirates",
		"BAHRN":  "Bahrain",
		"YEMAR":  "Yemen",
		"USA":    "United States",
	})

	tests := []struct {
		input string
		want  string
	}{
		{"SAARAB", "SAARAB"},       // the feed's own code
		{"SA", "SAARAB"},           // ISO alpha-2
		{"SAU", "SAARAB"},          // ISO alpha-3
		{"Saudi Arabia", "SAARAB"}, // name
		{"saudi arabia", "SAARAB"}, // case-insensitive
		{"BH", "BAHRN"},            // ISO code onto a feed code that is neither
		{"Bahrain", "BAHRN"},       //
		{"United Arab Emirates", "UAE"},
		{"AE", "UAE"},
		{"Atlantis", ""}, // unresolvable stays empty rather than guessing
		{"", ""},
	}
	for _, tt := range tests {
		if got := idx.Resolve(tt.input); got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestCountryIndexPrefersFeedCodes guards against an ISO alias shadowing a feed
// code that happens to spell the same thing. "UAE" is both.
func TestCountryIndexPrefersFeedCodes(t *testing.T) {
	idx := NewCountryIndex(map[string]string{
		"UAE": "United Arab Emirates",
		"USA": "United States",
	})
	if got := idx.Resolve("USA"); got != "USA" {
		t.Errorf("Resolve(USA) = %q, want USA", got)
	}
	if got := idx.Resolve("US"); got != "USA" {
		t.Errorf("Resolve(US) = %q, want USA", got)
	}
}
