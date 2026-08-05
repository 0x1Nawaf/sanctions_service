package scoring

import "testing"

type fpCase struct {
	query     string
	candidate string
	observed  int // score this pair produced before the tuning changes
	entity    bool
}

func (c fpCase) score() int {
	if c.entity {
		return ScoreEntityName(c.query, c.candidate)
	}
	return ScoreName(c.query, c.candidate)
}

// falsePositiveCorpus holds query/candidate pairs taken from real screening
// output where the pair scored high but is a different person or company. It
// measures whether a scoring change actually reduces false positives. Run with
// -v to see the scores.
//
// Every pair here is same-script. Screening output reports a record's best
// score across all of its name variants, so a cross-script pair — an Arabic
// query beside a record's Latin primary name — did not necessarily produce the
// score shown next to it, and cannot be used to judge a change in isolation.
var falsePositiveCorpus = []fpCase{
	{query: "سعود خليفه محمد ابراهيم", candidate: "ابراهيم محمد ابراهيم محمد", observed: 100},
	{query: "MUHAMMAD ASHRAF ABBAS", candidate: "Ashraf Muhammad Abas Muhammad", observed: 95},
	{query: "تركي محمد بن صالح الشتوي", candidate: "محمد بن صالح بن محمد الشرقي", observed: 94},
	{query: "معيض عبدالله محمد الشمراني", candidate: "محمد عبدالله محمد الزهراني", observed: 93},
	{query: "هدى محمد بن عبدالله الحميدي", candidate: "أحمد بن محمد بن أحمد الحميدي", observed: 91},
	{query: "ADAM MAOHAMED ALI MOHAMED", candidate: "Ali Mohamed Saeed Anam", observed: 90},
	{query: "فراس عبدالسلام بن عبدالله السليمان", candidate: "عبدالله بن سليمان السليمان", observed: 83},
	{query: "MOHAMMED ALI AHMED ALGHAMDI", candidate: "Ahmed Mohammed Ali Hareb Al-Shamri", observed: 83},
	{query: "عبدالله ذاكر علي المرحبي", candidate: "عبدالله علي المرّي", observed: 78},
	{query: "سالم بن عبدالله بن احمد الشهري", candidate: "محمد بن عبدالله بن أحمد الظاهري", observed: 77},
	{query: "عبدالرحمن ناصر بن عبدالرحمن آل قعود", candidate: "سعود بن عبدالله بن عبدالرحمن آل سعود", observed: 75},
	{query: "سليمان مهجع بن سليمان العنزي", candidate: "سليمان بن سيف بن سليمان الكندي", observed: 74},
	{query: "شركة نقدية المحدودة", candidate: "شركة أندا المحدودة", observed: 73, entity: true},
	{query: "شركة نكفيك العقارية", candidate: "شركة لافيكو العقارية", observed: 74, entity: true},
	{query: "شركة جوار للاستثمار", candidate: "شركة جوبيتر للاستثمار", observed: 74, entity: true},
	{query: "شركة نقدية المحدودة", candidate: "شركة سمت الهندسية المحدودة", observed: 56, entity: true},
}

func TestFalsePositiveCorpusScores(t *testing.T) {
	worst, aboveDefaultThreshold := 0, 0
	for _, c := range falsePositiveCorpus {
		got := c.score()
		if got > worst {
			worst = got
		}
		if got >= 50 {
			aboveDefaultThreshold++
		}
		t.Logf("was=%3d now=%3d  %q vs %q", c.observed, got, c.query, c.candidate)
	}
	t.Logf("highest remaining = %d; %d/%d still alert at the default min_score of 50",
		worst, aboveDefaultThreshold, len(falsePositiveCorpus))
}

// TestScoreBreakdown dumps each component of the blend so it is visible which
// stage inflates a false positive.
func TestScoreBreakdown(t *testing.T) {
	for _, c := range falsePositiveCorpus {
		if c.entity {
			continue
		}
		q, cand := normalize(c.query), normalize(c.candidate)
		qt := filterNameNoise(filterShortTokens(tokenize(q)))
		ct := filterNameNoise(filterShortTokens(tokenize(cand)))
		fwd := avgBestTokenSimilarity(qt, ct)
		rev := avgBestTokenSimilarity(ct, qt)
		tok := min(fwd, rev)
		full := levenshteinSimilarity(q, cand)
		ratio := min(len(qt), len(ct)) * 100 / max(len(qt), len(ct))
		blend := (tok*70 + full*20 + ratio*10) / 100
		boosted := subsetBoost(blend, fwd, len(qt), len(ct))
		t.Logf("%q vs %q\n  fwd=%d rev=%d token=%d full=%d ratio=%d blend=%d boosted=%d final=%d",
			c.query, c.candidate, fwd, rev, tok, full, ratio, blend, boosted,
			namePartGate(boosted, qt, ct))
	}
}

// TestTokenSimilarityPairs covers the surname pairs that drove the false
// positives: unrelated Gulf families whose names differ by one or two
// characters once the shared ال prefix is accounted for, alongside genuine
// transliteration variants that must keep matching.
func TestTokenSimilarityPairs(t *testing.T) {
	tests := []struct {
		a, b     string
		maxScore int // 0 means the pair must not match at all
		minScore int
	}{
		{a: "الشمراني", b: "الزهراني", maxScore: 0},
		{a: "الشتوي", b: "الشرقي", maxScore: 0},
		{a: "المرحبي", b: "المري", maxScore: 0},
		{a: "العجلان", b: "العيبان", maxScore: 0},
		{a: "الشهري", b: "الظاهري", maxScore: 0},
		{a: "العنزي", b: "الكندي", maxScore: 0},
		{a: "alghamdi", b: "alshamri", maxScore: 0},
		{a: "alkahtani", b: "alsowaidi", maxScore: 0},

		{a: "الحميدي", b: "الحميدي", minScore: 100, maxScore: 100},
		{a: "mohammed", b: "muhammad", minScore: 80, maxScore: 100},
		{a: "mohammed", b: "mohamed", minScore: 80, maxScore: 100},
		{a: "fahad", b: "fahd", minScore: 80, maxScore: 100},
		{a: "hassan", b: "hasan", minScore: 80, maxScore: 100},
		{a: "alshamrani", b: "shamrani", minScore: 100, maxScore: 100},
		{a: "العمر", b: "عمر", minScore: 100, maxScore: 100},
	}

	for _, tt := range tests {
		got := tokenSimilarity(tt.a, tt.b)
		if got > tt.maxScore {
			t.Errorf("tokenSimilarity(%q, %q) = %d, want <= %d", tt.a, tt.b, got, tt.maxScore)
		}
		if got < tt.minScore {
			t.Errorf("tokenSimilarity(%q, %q) = %d, want >= %d", tt.a, tt.b, got, tt.minScore)
		}
	}
}

// TestArabicNormalization covers the orthographic variation that used to cost
// an edit per character: hamza carriers, ta marbuta, diacritics, and compound
// given names recorded both joined and split.
func TestArabicNormalization(t *testing.T) {
	equal := [][2]string{
		{"أحمد", "احمد"},
		{"محمّد", "محمد"},
		{"هدى", "هدي"},
		{"فاطمة", "فاطمه"},
		{"آل سعود", "ال سعود"},
		{"عبد الله", "عبدالله"},
		{"عبد الرحمن", "عبدالرحمن"},
		{"abd al rahman", "abdalrahman"},
		{"مُحَمَّد", "محمد"},
	}
	for _, pair := range equal {
		if a, b := normalize(pair[0]), normalize(pair[1]); a != b {
			t.Errorf("normalize(%q) = %q, normalize(%q) = %q; want equal",
				pair[0], a, pair[1], b)
		}
	}
}
