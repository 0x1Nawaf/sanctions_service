package handler

import (
	"testing"

	"github.com/nnn/sanctions-service/internal/scoring"
)

func TestParseFactorInputs(t *testing.T) {
	scoring.SetCountryIndex(scoring.NewCountryIndex(map[string]string{
		"SAARAB": "Saudi Arabia",
		"UAE":    "United Arab Emirates",
	}))

	t.Run("nothing supplied", func(t *testing.T) {
		in, err := parseFactorInputs("", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if in.supplied() {
			t.Error("expected no factors to be reported as supplied")
		}
	})

	t.Run("both supplied", func(t *testing.T) {
		in, err := parseFactorInputs("1985-03-12", []string{"SA"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !in.dobSupplied || in.dob.Year != 1985 {
			t.Errorf("dob = %+v, want 1985-03-12", in.dob)
		}
		if len(in.citizenship) != 1 || in.citizenship[0] != "SAARAB" {
			t.Errorf("citizenship = %v, want [SAARAB]", in.citizenship)
		}
	})

	t.Run("unparseable date is a caller error", func(t *testing.T) {
		_, err := parseFactorInputs("03/04/1985", nil)
		if err == nil {
			t.Fatal("expected an error for an ambiguous date")
		}
		if _, ok := err.(badRequestError); !ok {
			t.Errorf("error type = %T, want badRequestError so it maps to a 400", err)
		}
	})

	t.Run("unknown country stays neutral rather than contradicting", func(t *testing.T) {
		in, err := parseFactorInputs("", []string{"Atlantis"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !in.citizenshipSupplied {
			t.Error("expected the citizenship to count as supplied")
		}
		if !in.citizenshipUnresolved() {
			t.Error("expected the citizenship to be reported unresolved")
		}
	})
}

func TestEvaluateFactors(t *testing.T) {
	scoring.SetCountryIndex(scoring.NewCountryIndex(map[string]string{
		"SAARAB": "Saudi Arabia",
		"YEMAR":  "Yemen",
	}))

	ids := &secondaryIdentifiers{
		dates: map[uint32][]scoring.PartialDate{
			1: {{Year: 1985, Month: 3, Day: 12}},
			2: {{Year: 1985}},
			3: {{Year: 1950}},
		},
		citizenship: map[uint32][]string{
			1: {"SAARAB"},
			2: {"YEMAR"},
		},
	}

	in, err := parseFactorInputs("1985-03-12", []string{"SA"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("both confirm", func(t *testing.T) {
		factors, total := evaluateFactors(in, ids, 1)
		if factors.DOB.Status != scoring.StatusConfirmedExact {
			t.Errorf("dob status = %q", factors.DOB.Status)
		}
		if factors.Citizenship.Status != scoring.StatusConfirmed {
			t.Errorf("citizenship status = %q", factors.Citizenship.Status)
		}
		if want := scoring.DOBExactAdjustment + scoring.CitizenshipMatch; total != want {
			t.Errorf("total = %d, want %d", total, want)
		}
	})

	t.Run("year confirms, citizenship contradicts", func(t *testing.T) {
		factors, total := evaluateFactors(in, ids, 2)
		if factors.DOB.Status != scoring.StatusConfirmedYear {
			t.Errorf("dob status = %q", factors.DOB.Status)
		}
		if factors.Citizenship.Status != scoring.StatusContradicted {
			t.Errorf("citizenship status = %q", factors.Citizenship.Status)
		}
		if want := scoring.DOBYearAdjustment + scoring.CitizenshipMismatch; total != want {
			t.Errorf("total = %d, want %d", total, want)
		}
	})

	t.Run("missing identifiers are neutral, not negative", func(t *testing.T) {
		factors, total := evaluateFactors(in, ids, 99)
		if factors.DOB.Status != scoring.StatusUnavailable {
			t.Errorf("dob status = %q, want %q", factors.DOB.Status, scoring.StatusUnavailable)
		}
		if factors.Citizenship.Status != scoring.StatusUnavailable {
			t.Errorf("citizenship status = %q", factors.Citizenship.Status)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0 — a record the feed has no data for must not be penalised", total)
		}
	})
}

func TestAdjustedScore(t *testing.T) {
	tests := []struct {
		name       string
		nameScore  int
		adjustment int
		minScore   int
		policy     factorPolicy
		want       int
	}{
		{"confirmation lifts", 74, 16, 75, factorPolicyDownweight, 90},
		{"no adjustment leaves the score alone", 80, 0, 75, factorPolicyDownweight, 80},
		{"capped at 100", 96, 10, 75, factorPolicyDownweight, 100},
		{"never negative", 10, -25, 75, factorPolicyDownweight, 0},
		{"weak match can be pushed out of the alert set", 78, -25, 75, factorPolicyDownweight, 53},
		{"strong name match is held at the threshold", 95, -25, 75, factorPolicyDownweight, 75},
		{"filter policy lets a strong match drop out", 95, -25, 75, factorPolicyFilter, 70},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustedScore(tt.nameScore, tt.adjustment, tt.minScore, tt.policy)
			if got != tt.want {
				t.Errorf("adjustedScore(%d, %d, %d, %s) = %d, want %d",
					tt.nameScore, tt.adjustment, tt.minScore, tt.policy, got, tt.want)
			}
		})
	}
}

// TestSortByScoreThenCorroboration covers the case the tiebreak exists for:
// four records sharing one very common name all score 100, so the score alone
// cannot separate them and a confirmed date of birth cannot lift the one that
// matches, because it is already at the ceiling.
func TestSortByScoreThenCorroboration(t *testing.T) {
	scored := []recordScore{
		{recordID: 3, finalScore: 100, factorAdjustment: 0},   // no identifiers on file
		{recordID: 2, finalScore: 100, factorAdjustment: -12}, // citizenship disagrees
		{recordID: 1, finalScore: 100, factorAdjustment: 16},  // both confirm
		{recordID: 4, finalScore: 100, factorAdjustment: 9},   // partly confirms
		{recordID: 5, finalScore: 87, factorAdjustment: 16},   // weaker name, both confirm
	}
	sortByScoreThenCorroboration(scored)

	want := []uint32{1, 4, 3, 2, 5}
	for i, id := range want {
		if scored[i].recordID != id {
			got := make([]uint32, len(scored))
			for j, s := range scored {
				got[j] = s.recordID
			}
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestParseFactorPolicy(t *testing.T) {
	tests := map[string]factorPolicy{
		"":           factorPolicyDownweight,
		"downweight": factorPolicyDownweight,
		"filter":     factorPolicyFilter,
		"FILTER":     factorPolicyFilter,
		"nonsense":   factorPolicyDownweight,
	}
	for input, want := range tests {
		if got := parseFactorPolicy(input); got != want {
			t.Errorf("parseFactorPolicy(%q) = %q, want %q", input, got, want)
		}
	}
}
