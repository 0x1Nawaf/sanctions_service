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

	searchTokens := tokenize(search)
	candidateTokens := tokenize(candidate)

	if len(searchTokens) == 0 || len(candidateTokens) == 0 {
		return 0
	}

	// Bidirectional token similarity (60% weight)
	forwardScore := avgBestTokenSimilarity(searchTokens, candidateTokens)
	reverseScore := avgBestTokenSimilarity(candidateTokens, searchTokens)
	tokenScore := min(forwardScore, reverseScore)

	// Full string similarity (20% weight)
	fullScore := levenshteinSimilarity(search, candidate)

	// Token count ratio (20% weight) — penalizes mismatched name lengths
	shorter := min(len(searchTokens), len(candidateTokens))
	longer := max(len(searchTokens), len(candidateTokens))
	ratioScore := (shorter * 100) / longer

	total := (tokenScore*60 + fullScore*20 + ratioScore*20) / 100

	if total > 100 {
		return 100
	}
	return total
}

// avgBestTokenSimilarity scores how well each source token is represented
// among the target tokens, using actual Levenshtein similarity values.
func avgBestTokenSimilarity(sourceTokens, targetTokens []string) int {
	if len(sourceTokens) == 0 {
		return 0
	}
	total := 0
	for _, st := range sourceTokens {
		best := 0
		for _, tt := range targetTokens {
			sim := levenshteinSimilarity(st, tt)
			if sim > best {
				best = sim
			}
		}
		total += best
	}
	return total / len(sourceTokens)
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
