package scoring

import (
	"strings"
	"unicode"
)

// entityNoiseWords are generic legal suffixes and organizational terms that
// carry no distinguishing value when comparing entity names. They are removed
// from token-level scoring so that "Al Rashid Trading Co" doesn't falsely
// match "Al Saud Trading Co" just because of the shared noise words.
var entityNoiseWords = map[string]bool{
	"co": true, "company": true, "corp": true, "corporation": true,
	"inc": true, "incorporated": true, "ltd": true, "limited": true,
	"llc": true, "llp": true, "plc": true, "pllc": true,
	"sa": true, "gmbh": true, "ag": true, "bv": true, "nv": true,
	"sarl": true, "srl": true, "ooo": true, "zao": true,
	"pjsc": true, "cjsc": true, "jsc": true,
	"est": true, "establishment": true,
	"trading": true, "shipping": true, "transport": true,
	"holdings": true, "holding": true, "group": true,
	"international": true, "intl": true,
	"enterprise": true, "enterprises": true,
	"organization": true, "organisation": true,
	"foundation": true, "association": true, "society": true,
	"institute": true, "services": true, "solutions": true,
	"industries": true, "industrial": true,
	"general": true, "national": true,
	"the": true, "of": true, "and": true, "for": true,
}

// ScoreEntityName compares entity names by separating "significant" tokens
// (the distinctive part of the name) from noise tokens (legal suffixes,
// generic organizational terms). Significant tokens drive the match; noise
// tokens that match on both sides provide only a small bonus.
func ScoreEntityName(searchName, candidateName string) int {
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

	sigSearch, noiseSearch := splitEntityTokens(searchTokens)
	sigCandidate, noiseCandidate := splitEntityTokens(candidateTokens)

	// If all tokens are noise (unlikely but possible), fall back to full comparison
	if len(sigSearch) == 0 {
		sigSearch = searchTokens
		noiseSearch = nil
	}
	if len(sigCandidate) == 0 {
		sigCandidate = candidateTokens
		noiseCandidate = nil
	}

	// Primary score: bidirectional match on significant tokens only
	forwardSig := avgBestTokenSimilarity(sigSearch, sigCandidate)
	reverseSig := avgBestTokenSimilarity(sigCandidate, sigSearch)
	sigScore := min(forwardSig, reverseSig)

	// Noise bonus: if both sides share noise words, add a small credit
	noiseBonus := 0
	if len(noiseSearch) > 0 && len(noiseCandidate) > 0 {
		noiseMatch := avgBestTokenSimilarity(noiseSearch, noiseCandidate)
		noiseBonus = noiseMatch / 10 // max +10 points
	}

	// Full-string similarity on normalized strings
	fullScore := levenshteinSimilarity(search, candidate)

	// Token count ratio based on significant tokens
	shorter := min(len(sigSearch), len(sigCandidate))
	longer := max(len(sigSearch), len(sigCandidate))
	ratioScore := (shorter * 100) / longer

	// Weighted blend: significant tokens 65%, full-string 20%, ratio 15%
	total := (sigScore*65 + fullScore*20 + ratioScore*15) / 100
	total += noiseBonus

	if total > 100 {
		return 100
	}
	return total
}

// splitEntityTokens separates tokens into significant (distinctive) and
// noise (generic legal/organizational) groups.
func splitEntityTokens(tokens []string) (significant, noise []string) {
	for _, t := range tokens {
		if entityNoiseWords[strings.ToLower(t)] {
			noise = append(noise, t)
		} else {
			significant = append(significant, t)
		}
	}
	return
}

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

	// Score with all tokens first (full-string similarity still uses everything).
	// For token-level comparison, filter out single-character tokens (initials
	// like "M", "A", "K") Ã¢ÂÂ they can never cross the 65% similarity threshold
	// against real tokens and would drag down the bidirectional average.
	sigSearchTokens := filterShortTokens(searchTokens)
	sigCandidateTokens := filterShortTokens(candidateTokens)
	if len(sigSearchTokens) == 0 || len(sigCandidateTokens) == 0 {
		// Fall back to all tokens if filtering leaves nothing
		return computeScore(searchTokens, candidateTokens, search, candidate)
	}

	return computeScore(sigSearchTokens, sigCandidateTokens, search, candidate)
}

// filterShortTokens removes tokens with fewer than 2 runes (single-letter
// initials) that add noise to token-level similarity scoring.
func filterShortTokens(tokens []string) []string {
	var result []string
	for _, t := range tokens {
		if len([]rune(t)) >= 2 {
			result = append(result, t)
		}
	}
	return result
}

func computeScore(searchTokens, candidateTokens []string, searchFull, candidateFull string) int {
	forwardScore := avgBestTokenSimilarity(searchTokens, candidateTokens)
	reverseScore := avgBestTokenSimilarity(candidateTokens, searchTokens)
	tokenScore := min(forwardScore, reverseScore)

	fullScore := levenshteinSimilarity(searchFull, candidateFull)

	shorter := min(len(searchTokens), len(candidateTokens))
	longer := max(len(searchTokens), len(candidateTokens))
	ratioScore := (shorter * 100) / longer

	total := (tokenScore*60 + fullScore*20 + ratioScore*20) / 100

	// Subset boost: when one name is a strong subset of the other (e.g. a
	// shorter patronymic chain vs the full chain), the bidirectional min +
	// length penalties are too harsh. If the shorter side's tokens all match
	// well, lift the score based on how much of the longer side is covered.
	total = subsetBoost(total, forwardScore, reverseScore,
		len(searchTokens), len(candidateTokens))

	if total > 100 {
		return 100
	}
	return total
}

// subsetBoost raises the score when one name is clearly a subset of the other.
// Forward (searchÃ¢ÂÂcandidate): the user searched with fewer tokens and all were
// found Ã¢ÂÂ strong signal, requires Ã¢ÂÂ¥3 search tokens.
// Reverse (candidateÃ¢ÂÂsearch): the DB record is shorter than the search Ã¢ÂÂ weaker
// signal, requires Ã¢ÂÂ¥4 candidate tokens to avoid false positives from very short
// names like "Nouf Al Saud" matching any longer name containing those tokens.
func subsetBoost(baseScore, forwardScore, reverseScore, searchLen, candidateLen int) int {
	best := baseScore

	boost := func(matchScore, subsetLen, supersetLen, minSubsetLen int) int {
		if matchScore < 90 || subsetLen < minSubsetLen {
			return 0
		}
		coverage := subsetLen * 100 / supersetLen
		if coverage > 100 {
			coverage = 100
		}
		if coverage < 50 {
			return 0
		}
		return matchScore - (100-coverage)/2
	}

	if b := boost(forwardScore, searchLen, candidateLen, 4); b > best {
		best = b
	}
	if b := boost(reverseScore, candidateLen, searchLen, 4); b > best {
		best = b
	}

	return best
}

// avgBestTokenSimilarity scores how well each source token is represented
// among the target tokens. Only genuine matches (Ã¢ÂÂ¥65% similarity) contribute;
// below that threshold, coincidental character overlap is treated as no match.
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
		if best >= 65 {
			total += best
		}
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

// --- Arabic-to-Latin transliteration ---

var arabicMap = map[rune]string{
	'\u0627': "a",  // ÃÂ§ alef
	'\u0623': "a",  // ÃÂ£ alef hamza above
	'\u0625': "i",  // ÃÂ¥ alef hamza below
	'\u0622': "a",  // ÃÂ¢ alef madda
	'\u0628': "b",  // ÃÂ¨ ba
	'\u062A': "t",  // ÃÂª ta
	'\u062B': "th", // ÃÂ« tha
	'\u062C': "j",  // ÃÂ¬ jeem
	'\u062D': "h",  // ÃÂ­ ha
	'\u062E': "kh", // ÃÂ® kha
	'\u062F': "d",  // ÃÂ¯ dal
	'\u0630': "dh", // ÃÂ° dhal
	'\u0631': "r",  // ÃÂ± ra
	'\u0632': "z",  // ÃÂ² zay
	'\u0633': "s",  // ÃÂ³ seen
	'\u0634': "sh", // ÃÂ´ sheen
	'\u0635': "s",  // ÃÂµ sad
	'\u0636': "d",  // ÃÂ¶ dad
	'\u0637': "t",  // ÃÂ· ta
	'\u0638': "dh", // ÃÂ¸ dha
	'\u0639': "a",  // ÃÂ¹ ain
	'\u063A': "gh", // ÃÂº ghain
	'\u0641': "f",  // ÃÂ fa
	'\u0642': "q",  // ÃÂ qaf
	'\u0643': "k",  // ÃÂ kaf
	'\u0644': "l",  // ÃÂ lam
	'\u0645': "m",  // ÃÂ meem
	'\u0646': "n",  // ÃÂ noon
	'\u0647': "h",  // ÃÂ ha
	'\u0648': "u",  // ÃÂ waw
	'\u064A': "i",  // ÃÂ ya
	'\u0649': "a",  // ÃÂ alef maksura
	'\u0629': "a",  // ÃÂ© ta marbuta
	'\u0621': "",   // ÃÂ¡ hamza
	'\u0626': "",   // ÃÂ¦ ya hamza
	'\u0624': "u",  // ÃÂ¤ waw hamza
	'\u0640': "",   // ÃÂ tatweel
}

var latinVowels = map[rune]bool{
	'a': true, 'e': true, 'i': true, 'o': true, 'u': true,
}

// IsArabic reports whether the string contains Arabic script characters.
func IsArabic(s string) bool {
	for _, r := range s {
		if r >= 0x0600 && r <= 0x06FF {
			return true
		}
	}
	return false
}

// TransliterateArabic converts Arabic script to a Latin approximation.
// The result is lowercase with spaces preserved. Diacritics are stripped and
// a short 'a' vowel is inserted between consecutive consonants to approximate
// common English spellings of Arabic names (e.g. ÃÂÃÂÃÂ¯ Ã¢ÂÂ "fahd").
func TransliterateArabic(s string) string {
	if !IsArabic(s) {
		return s
	}
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		// Skip Arabic diacritics (tashkeel)
		if r >= 0x064B && r <= 0x065F {
			continue
		}

		if unicode.IsSpace(r) {
			b.WriteRune(' ')
			continue
		}

		lat, ok := arabicMap[r]
		if !ok {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
			continue
		}
		b.WriteString(lat)
	}

	raw := strings.TrimSpace(b.String())
	return insertVowels(raw)
}

// insertVowels adds a short 'a' between consonant pairs where no vowel
// precedes the first consonant, approximating common Arabic name romanization.
// e.g. "fhd" Ã¢ÂÂ "fahd", "rshad" Ã¢ÂÂ "rashad", "alhrbi" Ã¢ÂÂ "alharbi"
func insertVowels(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}

	var result []rune
	for i, r := range runes {
		result = append(result, r)

		if unicode.IsSpace(r) || latinVowels[r] || !unicode.IsLetter(r) {
			continue
		}
		// r is a consonant Ã¢ÂÂ decide whether to insert 'a' after it
		if i+1 >= len(runes) {
			continue
		}
		next := runes[i+1]
		if unicode.IsSpace(next) || latinVowels[next] || !unicode.IsLetter(next) {
			continue
		}
		if isDigraphPair(r, next) {
			continue
		}
		// Skip if the character just before r in the output is already a vowel
		// (either original or previously inserted). This prevents over-insertion
		// in sequences like "alh" where 'l' already follows vowel 'a'.
		if len(result) >= 2 && latinVowels[result[len(result)-2]] {
			continue
		}
		result = append(result, 'a')
	}
	return string(result)
}

func isDigraphPair(first, second rune) bool {
	// Common ArabicÃ¢ÂÂLatin digraphs where inserting a vowel would be wrong:
	// kh, sh, th, dh, gh
	switch first {
	case 'k', 's', 't', 'd', 'g':
		return second == 'h'
	}
	return false
}
