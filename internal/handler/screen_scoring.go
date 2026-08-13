package handler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nnn/sanctions-service/internal/model"
	"github.com/nnn/sanctions-service/internal/scoring"
)

const (
	aliasExpandMargin     = 20
	maxAliasExpandRecords = 100
)

type recordScore struct {
	recordID uint32
	score    int
	name     string
	// shadowScore is the candidate scorer's verdict on the same record, best
	// across all of its name variants. Zero unless shadow scoring is enabled.
	shadowScore int
	shadowName  string

	// finalScore is score after secondary identifiers have been applied. It
	// equals score when the caller supplied none, and it is what the response
	// reports and orders by.
	finalScore       int
	shadowFinalScore int
	factors          *model.MatchFactors
	// factorAdjustment is the total the identifiers contributed, kept separate
	// because it breaks ties that finalScore cannot. A perfect name match is
	// already at 100, so confirming its date of birth cannot raise it any
	// further — but it should still be shown ahead of an equally-scoring record
	// whose identifiers are absent or contradictory.
	factorAdjustment int
}

func scoreCandidateName(searchName, searchType, candidateName string) int {
	if searchType == "entity" {
		return scoring.ScoreEntityName(searchName, candidateName)
	}
	return scoring.ScoreName(searchName, candidateName)
}

// scoreCandidateNameV2 is the candidate replacement scorer. Only individual
// scoring is being reworked, so entity searches score identically under both
// and will show no shadow difference.
func scoreCandidateNameV2(searchName, searchType, candidateName string) int {
	if searchType == "entity" {
		return scoring.ScoreEntityName(searchName, candidateName)
	}
	return scoring.ScoreNameV2(searchName, candidateName)
}

// mergeCandidateScores keeps the best-scoring name variant per record.
//
// When shadow is set, a record is also retained if only the candidate scorer
// would have alerted on it. Those records are excluded from the response by
// screenWithScore; carrying them this far is what makes it possible to observe
// what the candidate scorer would have promoted, rather than only what it would
// have suppressed.
func mergeCandidateScores(best map[uint32]recordScore, candidates []nameCandidate, searchName, searchType string, minScore int, shadow bool) {
	for _, c := range candidates {
		s := scoreCandidateName(searchName, searchType, c.name)
		shadowScore := 0
		if shadow {
			shadowScore = scoreCandidateNameV2(searchName, searchType, c.name)
		}
		if minScore > 0 && s < minScore && shadowScore < minScore {
			continue
		}

		existing, ok := best[c.recordID]
		if !ok {
			existing = recordScore{recordID: c.recordID, score: -1, shadowScore: -1}
		}
		if s > existing.score {
			existing.score = s
			existing.name = c.name
		}
		if shadowScore > existing.shadowScore {
			existing.shadowScore = shadowScore
			existing.shadowName = c.name
		}
		existing.recordID = c.recordID
		best[c.recordID] = existing
	}
}

func preliminaryScores(candidates []nameCandidate, searchName, searchType string) map[uint32]recordScore {
	best := make(map[uint32]recordScore, len(candidates))
	for _, c := range candidates {
		s := scoreCandidateName(searchName, searchType, c.name)
		if existing, ok := best[c.recordID]; !ok || s > existing.score {
			best[c.recordID] = recordScore{recordID: c.recordID, score: s, name: c.name}
		}
	}
	return best
}

func candidateKey(c nameCandidate) string {
	return fmt.Sprintf("%d:%s", c.recordID, strings.ToLower(strings.TrimSpace(c.name)))
}

func dedupeExpandedCandidates(seen map[string]bool, expanded []nameCandidate) []nameCandidate {
	if len(expanded) == 0 {
		return nil
	}
	out := make([]nameCandidate, 0, len(expanded))
	for _, c := range expanded {
		key := candidateKey(c)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func markCandidatesSeen(seen map[string]bool, candidates []nameCandidate) {
	for _, c := range candidates {
		seen[candidateKey(c)] = true
	}
}

func selectRecordIDsForAliasExpansion(preliminary map[uint32]recordScore, minScore int) map[uint32]bool {
	threshold := minScore - aliasExpandMargin
	if threshold < 0 {
		threshold = 0
	}

	ranked := make([]recordScore, 0, len(preliminary))
	for _, s := range preliminary {
		if s.score >= threshold {
			ranked = append(ranked, s)
		}
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	if len(ranked) > maxAliasExpandRecords {
		ranked = ranked[:maxAliasExpandRecords]
	}

	ids := make(map[uint32]bool, len(ranked))
	for _, s := range ranked {
		ids[s.recordID] = true
	}
	return ids
}
