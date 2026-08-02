package handler

import (
	"fmt"
	"sort"
	"strings"

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
}

func scoreCandidateName(searchName, searchType, candidateName string) int {
	if searchType == "entity" {
		return scoring.ScoreEntityName(searchName, candidateName)
	}
	return scoring.ScoreName(searchName, candidateName)
}

func mergeCandidateScores(best map[uint32]recordScore, candidates []nameCandidate, searchName, searchType string, minScore int) {
	for _, c := range candidates {
		s := scoreCandidateName(searchName, searchType, c.name)
		if minScore > 0 && s < minScore {
			continue
		}
		if existing, ok := best[c.recordID]; !ok || s > existing.score {
			best[c.recordID] = recordScore{recordID: c.recordID, score: s, name: c.name}
		}
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
