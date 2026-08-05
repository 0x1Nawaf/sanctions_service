package scoring

import (
	"strings"
	"sync"
	"unicode"
)

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

	// The connector-free token set is authoritative. Scoring both variants and
	// keeping the higher result, as this used to, meant a connector could only
	// ever raise a score — so adding بن or ال to the noise list would not have
	// removed the inflation they cause.
	return computeScore(
		filterNameNoise(sigSearchTokens),
		filterNameNoise(sigCandidateTokens),
		search, candidate)
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

	total = subsetBoost(total, forwardScore, len(searchTokens), len(candidateTokens))

	total = namePartGate(total, searchTokens, candidateTokens)

	if total > 100 {
		return 100
	}
	return total
}

// minSubsetCoverage is the smallest share of the candidate's tokens the query
// may cover and still be treated as a subset of it.
const minSubsetCoverage = 40

// subsetBoostCap limits how far the subset rule may lift a score above the
// weighted blend, so one rule can never turn a mediocre match into a
// near-certain one by itself.
//
// The allowance scales with the size of the query, because a fully matched quad
// name is much stronger evidence of the same person than a fully matched pair
// or triple: common given names co-occur by chance far more readily than four
// name parts in the same record do. Without that distinction, "Mohammed Hassan
// Ali" found inside a longer unrelated name is promoted just as confidently as
// a complete four-part name is.
func subsetBoostCap(searchLen int) int {
	if searchLen >= 4 {
		return 18
	}
	return 12
}

// subsetBoost raises the score when the query is a well-covered subset of a
// longer recorded name — "Osama Bin Laden" against "Usama Bin Muhammad Bin Awad
// Bin Laden". Gulf records routinely carry more patronymics than a client
// supplies, and without this the surplus middle names would suppress a genuine
// hit.
//
// Only that direction is considered. The mirror rule, boosting when the
// candidate was a subset of the query, was the largest single source of false
// positives. It keyed off how well the candidate's tokens were explained by the
// query, so a record assembled entirely from high-frequency names scored 100
// there whenever the query contained any of them, and returning that score
// discarded the fact that the query's own distinguishing tokens had matched
// nothing. It rated "سعود خليفه محمد ابراهيم" against "ابراهيم محمد ابراهيم
// محمد" a perfect 100, up from a blended 53.
func subsetBoost(baseScore, forwardScore, searchLen, candidateLen int) int {
	// The rule exists to excuse *surplus* candidate tokens, so it needs the
	// candidate to actually have more. At equal token counts neither side has
	// anything to excuse and the two-directional blend is already the right
	// answer.
	if searchLen < 2 || searchLen >= candidateLen {
		return baseScore
	}
	// Every query token must be accounted for before surplus candidate tokens
	// can be excused.
	if forwardScore < 90 {
		return baseScore
	}

	coverage := searchLen * 100 / candidateLen
	if coverage < minSubsetCoverage {
		return baseScore
	}

	boosted := forwardScore - (100-coverage)/2
	if cap := baseScore + subsetBoostCap(searchLen); boosted > cap {
		boosted = cap
	}
	if boosted > baseScore {
		return boosted
	}
	return baseScore
}

const (
	// How well a query's given and family name have to be represented among the
	// candidate's tokens to count as present. The family name is held to the
	// stricter bar: it is the more discriminating of the two, and it is the one
	// where a single substituted character separates real families — قعود from
	// سعود, الشتوي from الشرقي. Given names carry far more spelling variance
	// between systems, so an equally strict bar there would cost real matches.
	givenNameMatchFloor  = 72
	familyNameMatchFloor = 80
	// Multipliers applied when the corresponding name part is missing.
	givenNameShortfall  = 60
	familyNameShortfall = 55
)

// namePartGate penalises a match that fails to account for the ends of the
// query name.
//
// A Gulf name is given + father + grandfather + family. The middle two are what
// get dropped, reordered or abbreviated as a name moves between systems; the
// given name and the family name are the stable, discriminating pair. Scoring
// the name as an unordered bag of tokens loses that, which is how a record
// sharing only محمد and عبدالله with the query reaches the high eighties.
//
// Only the family name used to be checked, so nothing penalised a candidate
// whose given name was absent entirely.
func namePartGate(score int, searchTokens, candidateTokens []string) int {
	if len(searchTokens) < 2 {
		return score
	}

	present := func(token string, floor int) bool {
		if len([]rune(token)) < 3 {
			return true // too short to judge; do not penalise
		}
		for _, ct := range candidateTokens {
			if tokenSimilarity(token, ct) >= floor {
				return true
			}
		}
		return false
	}

	if !present(searchTokens[0], givenNameMatchFloor) {
		score = score * givenNameShortfall / 100
	}
	if !present(searchTokens[len(searchTokens)-1], familyNameMatchFloor) {
		score = score * familyNameShortfall / 100
	}
	return score
}

const (
	tokenMatchFloor        = 65
	skeletonMatchScore     = 85
	minSkeletonLenForMatch = 3
)

// editBudget is the largest edit distance still treated as a spelling variant
// of a token of the given length.
//
// A percentage floor alone is too permissive for the short tokens Arabic
// surnames reduce to once the article is removed: at 65%, a four-character
// token tolerates a whole substituted character, which is the entire difference
// between قعود and سعود. Capping the absolute number of edits scales the
// tolerance to how much name there is to be wrong about.
func editBudget(length int) int {
	switch {
	case length <= 2:
		return 0
	case length <= 7:
		return 1
	case length <= 11:
		return 2
	default:
		return 3
	}
}

// tokenSimilarity compares two name tokens on a 0-100 scale, returning 0 when
// they are too far apart to be plausibly the same name.
func tokenSimilarity(a, b string) int {
	if a == b {
		return 100
	}

	a, b = stripArticle(a), stripArticle(b)
	if a == b {
		return 100
	}

	ra, rb := []rune(a), []rune(b)
	maxLen := max(len(ra), len(rb))
	if maxLen == 0 {
		return 0
	}
	distance := levenshteinDistance(ra, rb)
	sim := ((maxLen - distance) * 100) / maxLen

	// Already a strong match; the skeleton check below cannot improve on it.
	if distance <= editBudget(maxLen) && sim >= skeletonMatchScore {
		return sim
	}

	// Two romanisations of one Arabic name differ mostly in the vowels the
	// transcriber supplied, which edit distance charges for in full. An
	// identical consonant skeleton means the difference is that transcription
	// noise: Mohammed/Muhammad, Fahad/Fahd, Shurf/Shurafa.
	if skeleton := consonantSkeleton(a); len(skeleton) >= minSkeletonLenForMatch &&
		skeleton == consonantSkeleton(b) {
		if sim < skeletonMatchScore {
			sim = skeletonMatchScore
		}
		return sim
	}

	if distance > editBudget(maxLen) || sim < tokenMatchFloor {
		return 0
	}
	return sim
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
			if sim := tokenSimilarity(st, tt); sim > best {
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
