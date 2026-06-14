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

	return computeScore(searchTokens, candidateTokens, search, candidate)
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

	if total > 100 {
		return 100
	}
	return total
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
	'Ø§': "a", 'Ø£': "a", 'Ø¥': "i", 'Ø¢': "a",
	'Ø¨': "b", 'Øª': "t", 'Ø«': "th",
	'Ø¬': "j", 'Ø­': "h", 'Ø®': "kh",
	'Ø¯': "d", 'Ø°': "dh", 'Ø±': "r", 'Ø²': "z",
	'Ø³': "s", 'Ø´': "sh",
	'Øµ': "s", 'Ø¶': "d",
	'Ø·': "t", 'Ø¸': "dh",
	'Ø¹': "a", 'Øº': "gh",
	'Ù': "f", 'Ù': "q", 'Ù': "k",
	'Ù': "l", 'Ù': "m", 'Ù': "n",
	'Ù': "h", 'Ù': "u", 'Ù': "i",
	'Ù': "a", 'Ø©': "a",
	'Ø¡': "", 'Ø¦': "", 'Ø¤': "u",
	'\u0640': "", // tatweel
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
