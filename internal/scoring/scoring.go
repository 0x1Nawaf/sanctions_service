package scoring

import (
	"strings"
	"unicode"
)

func ScoreName(searchName, candidateName string) int {
	search := normalize(searchName)
	candidate := normalize(candidateName)

	if search == "" || candidate == "" {
		return 0
	}

	if search == candidate {
		return 100
	}

	// Check prefix match
	if strings.HasPrefix(candidate, search) || strings.HasPrefix(search, candidate) {
		searchRunes := []rune(search)
		candidateRunes := []rune(candidate)
		shorter := len(searchRunes)
		if len(candidateRunes) < shorter {
			shorter = len(candidateRunes)
		}
		longer := len(searchRunes)
		if len(candidateRunes) > longer {
			longer = len(candidateRunes)
		}
		return 60 + (40 * shorter / longer)
	}

	searchTokens := tokenize(search)
	candidateTokens := tokenize(candidate)

	if len(searchTokens) == 0 || len(candidateTokens) == 0 {
		return 0
	}

	// Token matching (50% weight)
	tokenScore := tokenMatchScore(searchTokens, candidateTokens)

	// Full string similarity (30% weight)
	fullScore := levenshteinSimilarity(search, candidate)

	// Coverage score (20% weight) — prevents "John" matching "John Alexander Muhammad Al-Rashidi" at 95%
	coverageScore := coverageMatchScore(searchTokens, candidateTokens)

	total := (tokenScore*50 + fullScore*30 + coverageScore*20) / 100

	if total > 100 {
		return 100
	}
	return total
}

func tokenMatchScore(searchTokens, candidateTokens []string) int {
	if len(searchTokens) == 0 {
		return 0
	}

	totalScore := 0
	for _, st := range searchTokens {
		bestMatch := 0
		for _, ct := range candidateTokens {
			sim := levenshteinSimilarity(st, ct)
			if sim > bestMatch {
				bestMatch = sim
			}
		}
		if bestMatch >= 85 {
			totalScore += 100
		} else if bestMatch >= 60 {
			totalScore += 50
		}
	}

	return totalScore / len(searchTokens)
}

func coverageMatchScore(searchTokens, candidateTokens []string) int {
	if len(candidateTokens) == 0 {
		return 0
	}

	covered := 0
	for _, ct := range candidateTokens {
		for _, st := range searchTokens {
			if levenshteinSimilarity(st, ct) >= 70 {
				covered++
				break
			}
		}
	}

	return (covered * 100) / len(candidateTokens)
}

func levenshteinSimilarity(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	d := levenshteinDistance(ra, rb)
	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	if maxLen == 0 {
		return 100
	}
	return ((maxLen - d) * 100) / maxLen
}

func levenshteinDistance(a, b []rune) int {
	la := len(a)
	lb := len(b)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost

			curr[j] = ins
			if del < curr[j] {
				curr[j] = del
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if unicode.IsSpace(r) && !prevSpace {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func tokenize(s string) []string {
	tokens := strings.Fields(s)
	result := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if len(t) > 0 {
			result = append(result, t)
		}
	}
	return result
}
