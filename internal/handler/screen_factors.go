package handler

import (
	"sort"
	"strings"

	"github.com/nnn/sanctions-service/internal/model"
	"github.com/nnn/sanctions-service/internal/scoring"
)

const (
	// factorEvalLimit bounds how many records have their secondary identifiers
	// fetched and adjusted. The response is capped at 50 and the largest
	// possible promotion is MaxFactorBoost, so evaluating a few hundred of the
	// best name matches is comfortably more than enough to catch anything a
	// confirmed identifier could lift into the answer.
	factorEvalLimit = 200

	// strongNameMatchFloor is the name score at or above which a contradicting
	// identifier will not, on its own, remove a record from the alert set.
	//
	// Feed dates of birth are often approximate and citizenship is
	// under-recorded, so a contradiction is a reason to doubt a match, not
	// grounds to withhold it from a reviewer. The record still appears, marked
	// contradicted, with the penalty visible in match_factors.
	strongNameMatchFloor = 90
)

// factorPolicy decides what a contradicted identifier is allowed to do.
type factorPolicy string

const (
	// factorPolicyDownweight lowers the score but keeps a strong name match in
	// the alert set. The default, and the conservative choice.
	factorPolicyDownweight factorPolicy = "downweight"
	// factorPolicyFilter lets a contradiction carry a record out of the alert
	// set entirely. Lower alert volume, and a decision for compliance to own.
	factorPolicyFilter factorPolicy = "filter"
)

func parseFactorPolicy(s string) factorPolicy {
	if strings.EqualFold(strings.TrimSpace(s), string(factorPolicyFilter)) {
		return factorPolicyFilter
	}
	return factorPolicyDownweight
}

// badRequestError marks a caller mistake, so screening can report it as a 400
// instead of failing the request as a server error.
type badRequestError struct{ msg string }

func (e badRequestError) Error() string { return e.msg }

// factorInputs is the caller's secondary identifiers, parsed and resolved once
// per request rather than once per candidate record.
type factorInputs struct {
	dob         scoring.PartialDate
	dobSupplied bool

	// citizenship holds the feed's own country codes, resolved from whatever
	// the caller wrote.
	citizenship         []string
	citizenshipSupplied bool
}

func (f factorInputs) supplied() bool { return f.dobSupplied || f.citizenshipSupplied }

// citizenshipUnresolved reports that the caller supplied a citizenship but none
// of it could be mapped onto a country the feed knows. Comparing the raw text
// would contradict every record it touched, so the factor is reported as
// unresolved and left neutral instead.
func (f factorInputs) citizenshipUnresolved() bool {
	return f.citizenshipSupplied && len(f.citizenship) == 0
}

func parseFactorInputs(dateOfBirth string, citizenship []string) (factorInputs, error) {
	var in factorInputs

	if s := strings.TrimSpace(dateOfBirth); s != "" {
		d, err := scoring.ParsePartialDate(s)
		if err != nil {
			return in, badRequestError{msg: err.Error()}
		}
		if !d.IsZero() {
			in.dob = d
			in.dobSupplied = true
		}
	}

	for _, c := range citizenship {
		if strings.TrimSpace(c) == "" {
			continue
		}
		in.citizenshipSupplied = true
		if resolved := scoring.ResolveCountry(c); resolved != "" {
			in.citizenship = append(in.citizenship, resolved)
		}
	}

	return in, nil
}

// evaluateFactors compares one record's identifiers against the caller's and
// reports both what each factor concluded and the total adjustment.
func evaluateFactors(in factorInputs, ids *secondaryIdentifiers, recordID uint32) (*model.MatchFactors, int) {
	factors := &model.MatchFactors{}
	total := 0

	if in.dobSupplied {
		status, adjustment, matched := scoring.CompareDOB(in.dob, ids.datesFor(recordID))
		factors.DOB = &model.MatchFactor{
			Status:      status,
			Adjustment:  adjustment,
			RecordValue: matched.String(),
		}
		total += adjustment
	}

	if in.citizenshipSupplied {
		if in.citizenshipUnresolved() {
			factors.Citizenship = &model.MatchFactor{Status: scoring.StatusUnresolved}
		} else {
			status, adjustment, matched := scoring.CompareCitizenship(
				in.citizenship, ids.citizenshipFor(recordID))
			factors.Citizenship = &model.MatchFactor{
				Status:      status,
				Adjustment:  adjustment,
				RecordValue: matched,
			}
			total += adjustment
		}
	}

	return factors, total
}

// applySecondaryIdentifiers fetches the identifiers for the best name matches
// and folds them into each record's final score, in place.
//
// Only the top factorEvalLimit records by name score are evaluated. Anything
// below that could not reach the threshold even with a fully confirmed
// identifier, so fetching its dates would be work spent on records that cannot
// change the answer.
func (h *ScreenHandler) applySecondaryIdentifiers(scored []recordScore, in factorInputs, minScore int) error {
	evaluated := scored
	if len(evaluated) > factorEvalLimit {
		evaluated = evaluated[:factorEvalLimit]
	}
	if len(evaluated) == 0 {
		return nil
	}

	ids := make([]uint32, len(evaluated))
	for i, s := range evaluated {
		ids[i] = s.recordID
	}

	identifiers, err := h.fetchSecondaryIdentifiers(ids, in.dobSupplied, !in.citizenshipUnresolved() && in.citizenshipSupplied)
	if err != nil {
		return err
	}

	for i := range evaluated {
		factors, adjustment := evaluateFactors(in, identifiers, evaluated[i].recordID)
		evaluated[i].factors = factors
		evaluated[i].factorAdjustment = adjustment
		evaluated[i].finalScore = adjustedScore(evaluated[i].score, adjustment, minScore, h.factorPolicy)
		evaluated[i].shadowFinalScore = adjustedScore(evaluated[i].shadowScore, adjustment, minScore, h.factorPolicy)
	}
	return nil
}

// sortByScoreThenCorroboration orders results by score, and breaks ties by how
// well the secondary identifiers agree.
//
// The tiebreak is what makes the factors useful in the case they were built
// for. Several records routinely share one very common name and all score 100
// on it, and at that ceiling a confirmed date of birth cannot raise the score
// any further — leaving the one record whose identifiers match sitting behind
// records that have no identifiers at all. Ordering the equals by corroboration
// puts the likeliest person first.
func sortByScoreThenCorroboration(scored []recordScore) {
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].finalScore != scored[j].finalScore {
			return scored[i].finalScore > scored[j].finalScore
		}
		return scored[i].factorAdjustment > scored[j].factorAdjustment
	})
}

// adjustedScore applies the factor adjustment to a name score, holding a strong
// name match inside the alert set under the default policy.
func adjustedScore(nameScore, adjustment, minScore int, policy factorPolicy) int {
	score := nameScore + adjustment
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	if policy == factorPolicyDownweight &&
		adjustment < 0 &&
		nameScore >= strongNameMatchFloor &&
		minScore > 0 && score < minScore {
		return minScore
	}
	return score
}
