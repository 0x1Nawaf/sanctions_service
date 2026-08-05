package scoring

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Arabic names are written with several interchangeable orthographies. Hamza
// carriers (أ إ آ) are routinely typed as a bare alef, ta marbuta is written as
// ha, ya appears with and without its dots, and diacritics are omitted by most
// data entry. Folding these to one form is what lets "أحمد" compare equal to
// "احمد" and "محمّد" to "محمد" instead of costing an edit each.
var arabicFoldMap = map[rune]rune{
	'\u0622': '\u0627', // آ -> ا
	'\u0623': '\u0627', // أ -> ا
	'\u0625': '\u0627', // إ -> ا
	'\u0671': '\u0627', // ٱ -> ا
	'\u0649': '\u064A', // ى -> ي
	'\u0626': '\u064A', // ئ -> ي
	'\u0629': '\u0647', // ة -> ه
	'\u0624': '\u0648', // ؤ -> و
}

// isArabicDropRune reports marks that carry no lexical weight in a name: the
// tashkeel range, the superscript alef, the tatweel used only for justification,
// and the standalone hamza.
func isArabicDropRune(r rune) bool {
	switch {
	case r >= 0x064B && r <= 0x0655:
		return true
	case r == 0x0670, r == 0x0640, r == 0x0621:
		return true
	}
	return false
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if isArabicDropRune(r) {
			continue
		}
		if folded, ok := arabicFoldMap[r]; ok {
			r = folded
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevSpace = false
		case unicode.IsSpace(r) && !prevSpace:
			b.WriteRune(' ')
			prevSpace = true
		}
	}

	return strings.Join(joinCompoundNames(strings.Fields(b.String())), " ")
}

func tokenize(s string) []string {
	return strings.Fields(s)
}

// theophoricPrefixes are the leading half of Arabic compound given names
// ("servant of ..."). Sources record them both joined and split — عبدالله and
// عبد الله are the same name — so they are joined before tokenising. Left
// alone, the two spellings disagree on token count as well as content, which
// distorts the token-count ratio on top of the token comparison itself.
var theophoricPrefixes = map[string]bool{
	"عبد":   true,
	"abd":   true,
	"abdul": true,
	"abdel": true,
	"abdal": true,
}

// definiteArticles are the article as a standalone token, in both scripts.
var definiteArticles = map[string]bool{
	"ال": true,
	"al": true,
	"el": true,
}

func joinCompoundNames(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		if !theophoricPrefixes[tokens[i]] || i+1 >= len(tokens) {
			out = append(out, tokens[i])
			continue
		}
		// "abd al rahman" and "عبد ال رحمن" put the article between the two
		// halves; absorb it so the result matches the joined spelling.
		if definiteArticles[tokens[i+1]] && i+2 < len(tokens) {
			out = append(out, tokens[i]+tokens[i+1]+tokens[i+2])
			i += 2
			continue
		}
		out = append(out, tokens[i]+tokens[i+1])
		i++
	}
	return out
}

// stripArticle removes a leading definite article from a name token.
//
// Nearly every Arabic family name carries ال, and its Latin transliteration
// carries "al"/"el". Because edit distance is length-normalised, that shared
// two-character prefix hands every pair of unrelated surnames a similarity
// bonus: الشمراني and الزهراني score 75 against each other, and alghamdi and
// alshamri also score 75, purely on the article plus the shared name pattern.
// Removing it first compares the distinguishing part of the name.
//
// The length guard keeps short names that merely begin with those letters —
// ali, alam, علي — intact.
func stripArticle(token string) string {
	r := []rune(token)
	if len(r) < 5 {
		return token
	}
	if r[0] == '\u0627' && r[1] == '\u0644' { // ال
		return string(r[2:])
	}
	if r[0] == 'a' || r[0] == 'e' {
		if r[1] == 'l' {
			return string(r[2:])
		}
	}
	return token
}

// consonantSkeleton drops the vowels from a Latin token and collapses repeated
// letters.
//
// Arabic script does not write short vowels, so romanisation invents them and
// different sources invent different ones: Mohammed / Muhammad / Mohamed all
// transcribe the same name, as do Fahad / Fahd and Shurf / Shurafa. Those
// differences are noise from the transliteration, not evidence of different
// people, and edit distance alone charges up to three edits for them. Comparing
// skeletons recovers the match.
//
// It returns "" for non-Latin input: Arabic script spelling is stable, so the
// same reduction there would wrongly merge genuinely distinct names such as
// محمد and محمود.
func consonantSkeleton(token string) string {
	// Bail before allocating on anything outside ASCII. Screening is dominated
	// by Arabic-script comparisons, and this is on the hot path: every query
	// token is compared against every candidate token.
	for i := 0; i < len(token); i++ {
		if token[i] >= utf8.RuneSelf {
			return ""
		}
	}

	var b strings.Builder
	b.Grow(len(token))
	var prev byte
	for i := 0; i < len(token); i++ {
		c := token[i]
		if latinVowels[rune(c)] || c == 'y' || c == prev {
			continue
		}
		b.WriteByte(c)
		prev = c
	}
	return b.String()
}

// nameNoiseWords are patronymic connectors and honorific titles. They appear in
// a large share of formal Gulf names — in the current feed "al" is in 19% of
// name rows and "bin" in 9% — so matching on them is not evidence that two
// people are the same, and letting them count as full-value token matches
// inflates every score in the region.
var nameNoiseWords = normalizedSet(
	"bin", "bint", "ibn", "ben", "bn",
	"al", "el",
	"بن", "بنت", "ابن", "ال", "آل",
	"prince", "princess", "sheikh", "shaikh", "shaykh",
	"king", "queen",
	"dr", "mr", "mrs", "ms",
	"الشيخ", "الاميرة", "الامير", "السيد", "السيدة", "الدكتور",
)

// entityNoiseWords are legal forms and generic business terms that carry no
// distinguishing value. Without the Arabic half of this list, "شركة نقدية
// المحدودة" matches every Saudi limited company in the feed on شركة and
// المحدودة alone, which is exactly what the sample screening output shows.
var entityNoiseWords = normalizedSet(
	"co", "company", "corp", "corporation",
	"inc", "incorporated", "ltd", "limited",
	"llc", "llp", "plc", "pllc",
	"sa", "gmbh", "ag", "bv", "nv",
	"sarl", "srl", "ooo", "zao",
	"pjsc", "cjsc", "jsc",
	"est", "establishment",
	"trading", "shipping", "transport",
	"holdings", "holding", "group",
	"international", "intl",
	"enterprise", "enterprises",
	"organization", "organisation",
	"foundation", "association", "society",
	"institute", "services", "solutions",
	"industries", "industrial",
	"general", "national",
	"the", "of", "and", "for",

	"شركة", "شركه", "مؤسسة", "مؤسسه", "منشأة", "منشاة", "مكتب", "مصنع", "معمل",
	"المحدودة", "محدودة", "المسؤولية", "مسؤولية", "ذات", "ذمم", "شمل", "شمم",
	"القابضة", "قابضة", "المجموعة", "مجموعة",
	"العقارية", "عقارية", "التجارية", "تجارية", "الصناعية", "صناعية",
	"للاستثمار", "الاستثمار", "استثمار", "للاستثمارات", "الاستثمارات",
	"للتجارة", "التجارة", "للمقاولات", "المقاولات", "للخدمات", "الخدمات",
	"للتنمية", "التنمية", "للتطوير", "التطوير", "للصناعة", "الصناعة",
	"العالمية", "الدولية", "الوطنية", "العامة", "المتحدة",
	"واولاده", "وشركاه", "واخوانه",
)

// normalizedSet builds a lookup keyed by the same normalisation the scorer
// applies to input, so entries can be written in their natural spelling.
func normalizedSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if n := normalize(w); n != "" {
			set[n] = true
		}
	}
	return set
}

// IsNameConnector reports whether a raw search token is a patronymic connector,
// article or title. Retrieval uses it to avoid making such a token a required
// FULLTEXT term, which would drop every record that spells the name without it.
func IsNameConnector(token string) bool {
	return nameNoiseWords[normalize(token)]
}
