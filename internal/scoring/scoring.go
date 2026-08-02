package scoring

import (
	"strings"
	"sync"
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

	sigSearchTokens := filterShortTokens(searchTokens)
	sigCandidateTokens := filterShortTokens(candidateTokens)
	if len(sigSearchTokens) == 0 || len(sigCandidateTokens) == 0 {
		return computeScore(searchTokens, candidateTokens, search, candidate)
	}

	baseScore := computeScore(sigSearchTokens, sigCandidateTokens, search, candidate)

	filtSearch := filterNameNoise(sigSearchTokens)
	filtCandidate := filterNameNoise(sigCandidateTokens)
	if len(filtSearch) != len(sigSearchTokens) || len(filtCandidate) != len(sigCandidateTokens) {
		filteredScore := computeScore(filtSearch, filtCandidate, search, candidate)
		if filteredScore > baseScore {
			return filteredScore
		}
	}

	return baseScore
}

// nameNoiseWords are patronymic connectors (bin/bint/ibn) and honorific titles
// that appear in formal Arabic name records but carry no distinguishing value.
// Filtering them from token scoring prevents long patronymic chains from
// dragging down the bidirectional match score.
var nameNoiseWords = map[string]bool{
	"bin": true, "bint": true, "ibn": true, "ben": true,
	"princess": true, "prince": true,
	"sheikh": true, "shaikh": true, "shaykh": true,
	"king": true, "queen": true,
	"dr": true, "mr": true, "mrs": true, "ms": true,
}

// filterNameNoise removes patronymic connectors and titles from tokens.
// Returns the original slice if filtering would leave fewer than 2 tokens,
// to avoid degrading scoring for names like "Bin Laden".
func filterNameNoise(tokens []string) []string {
	var result []string
	for _, t := range tokens {
		if !nameNoiseWords[t] {
			result = append(result, t)
		}
	}
	if len(result) < 2 {
		return tokens
	}
	return result
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

	total := (tokenScore*70 + fullScore*20 + ratioScore*10) / 100

	total = subsetBoost(total, forwardScore, reverseScore,
		len(searchTokens), len(candidateTokens))

	total = surnamePenalty(total, searchTokens, candidateTokens)

	if total > 100 {
		return 100
	}
	return total
}

// subsetBoost raises the score when one name is clearly a subset of the other.
// For short subsets (2-3 tokens) a higher coverage threshold (>60%) is required
// to avoid boosting common-name overlaps like "Mohammed Hassan Ali" matching any
// longer name that happens to contain those three frequent tokens.
func subsetBoost(baseScore, forwardScore, reverseScore, searchLen, candidateLen int) int {
	best := baseScore

	boost := func(matchScore, subsetLen, supersetLen int) int {
		if matchScore < 90 || subsetLen < 2 {
			return 0
		}
		coverage := subsetLen * 100 / supersetLen
		if coverage > 100 {
			coverage = 100
		}
		minCoverage := 50
		if subsetLen < 4 {
			minCoverage = 61
		}
		if coverage < minCoverage {
			return 0
		}
		return matchScore - (100-coverage)/2
	}

	if b := boost(forwardScore, searchLen, candidateLen); b > best {
		best = b
	}
	if b := boost(reverseScore, candidateLen, searchLen); b > best {
		best = b
	}

	return best
}

// surnamePenalty reduces the score when the last token of the search (typically
// the family/surname in Arabic names) has no good match among the candidate
// tokens. This is the strongest signal for false positives: "Nouf Alkahtani"
// should not match "Nouf Al-Sowaidi" just because they share a first name.
func surnamePenalty(score int, searchTokens, candidateTokens []string) int {
	if len(searchTokens) < 2 {
		return score
	}
	lastToken := searchTokens[len(searchTokens)-1]
	if len([]rune(lastToken)) < 3 {
		return score
	}
	bestMatch := 0
	for _, ct := range candidateTokens {
		sim := levenshteinSimilarity(lastToken, ct)
		if sim > bestMatch {
			bestMatch = sim
		}
	}
	if bestMatch < 65 {
		return score * 65 / 100
	}
	return score
}

// avgBestTokenSimilarity scores how well each source token is represented
// among the target tokens. Only genuine matches (â¥65% similarity) contribute;
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

	need := lb + 1
	prev := levenshteinBufPool.Get().([]int)
	curr := levenshteinBufPool.Get().([]int)
	if len(prev) < need {
		prev = make([]int, need)
	}
	if len(curr) < need {
		curr = make([]int, need)
	}
	prev = prev[:need]
	curr = curr[:need]

	defer func() {
		levenshteinBufPool.Put(prev[:cap(prev)])
		levenshteinBufPool.Put(curr[:cap(curr)])
	}()

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

var levenshteinBufPool = sync.Pool{
	New: func() interface{} {
		buf := make([]int, 128)
		return buf
	},
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
	'\u0627': "a",  // Ø§ alef
	'\u0623': "a",  // Ø£ alef hamza above
	'\u0625': "i",  // Ø¥ alef hamza below
	'\u0622': "a",  // Ø¢ alef madda
	'\u0628': "b",  // Ø¨ ba
	'\u062A': "t",  // Øª ta
	'\u062B': "th", // Ø« tha
	'\u062C': "j",  // Ø¬ jeem
	'\u062D': "h",  // Ø­ ha
	'\u062E': "kh", // Ø® kha
	'\u062F': "d",  // Ø¯ dal
	'\u0630': "dh", // Ø° dhal
	'\u0631': "r",  // Ø± ra
	'\u0632': "z",  // Ø² zay
	'\u0633': "s",  // Ø³ seen
	'\u0634': "sh", // Ø´ sheen
	'\u0635': "s",  // Øµ sad
	'\u0636': "d",  // Ø¶ dad
	'\u0637': "t",  // Ø· ta
	'\u0638': "dh", // Ø¸ dha
	'\u0639': "a",  // Ø¹ ain
	'\u063A': "gh", // Øº ghain
	'\u0641': "f",  // Ù fa
	'\u0642': "q",  // Ù qaf
	'\u0643': "k",  // Ù kaf
	'\u0644': "l",  // Ù lam
	'\u0645': "m",  // Ù meem
	'\u0646': "n",  // Ù noon
	'\u0647': "h",  // Ù ha
	'\u0648': "u",  // Ù waw
	'\u064A': "i",  // Ù ya
	'\u0649': "a",  // Ù alef maksura
	'\u0629': "a",  // Ø© ta marbuta
	'\u0621': "",   // Ø¡ hamza
	'\u0626': "",   // Ø¦ ya hamza
	'\u0624': "u",  // Ø¤ waw hamza
	'\u0640': "",   // Ù tatweel
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
// common English spellings of Arabic names (e.g. ÙÙØ¯ â "fahd").
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
// e.g. "fhd" â "fahd", "rshad" â "rashad", "alhrbi" â "alharbi"
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
		// r is a consonant â decide whether to insert 'a' after it
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
	// Common ArabicâLatin digraphs where inserting a vowel would be wrong:
	// kh, sh, th, dh, gh
	switch first {
	case 'k', 's', 't', 'd', 'g':
		return second == 'h'
	}
	return false
}
