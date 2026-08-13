package scoring

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// generationalCorpus holds pairs taken from a 7,000-client screening run where
// the current scorer rated a *relative* as the client. Each shares most of its
// name chain with the query but in a shifted order, which is what a name one
// generation up or down looks like.
var generationalCorpus = []struct {
	query     string
	candidate string
	observed  int // score the live scorer produced on that run
}{
	{"عبدالعزيز بن خالد بن عبدالله آل سعود", "خالد بن عبدالله بن عبدالعزيز آل سعود", 91},
	{"محمد ناصر محمد الهاجري", "ناصر محمد ناصر الهاجري", 90},
	{"MOHAMMED HASSAN AHMED ALI", "Ahmed Hassan Mohammed Ali", 93},
	{"ABDULLAH SALEH MOHAMMED ALI", "Saleh Ali Mohammed Abdullah Al Bourai", 89},
	{"AHMED ALI MOHAMED HASSAN", "Mohamed Ahmed Ali Hassan", 90},
	{"عبدالمحسن عبدالعزيز عبدالمحسن الراشد", "راشد عبدالعزيز عبدالمحسن الراشد", 95},
	{"عبدالله محمد بن سعد الحقباني", "محمد بن عبدالله بن سعد آل الحقباني", 90},
	{"احمد محمود أحمد عبدالله", "محمود أحمد عبدالله أحمد", 91},
	{"MOHAMMED ABDULLAH MOHAMMED ALKHALID", "Khalid Abdullah Mohammed Abdullah Al Mass", 90},
	{"محمد عبدالرحمن خلف الراشد", "عبدالرحمن بن محمد الراشد", 70},
}

// samePersonCorpus holds pairs from the same run that are the same individual:
// the chain agrees end to end, and what differs is transliteration, an omitted
// patronymic, or a dropped article. These must keep alerting.
var samePersonCorpus = []struct {
	query     string
	candidate string
}{
	{"MUFLIH RUBAYAN SHUFLUT ALQAHTANI", "Muflih Rabian Shaflout Al-Qahtani"},
	{"ABDULLAH MOHAMMED H ALQAHTANI", "Abdullah Bin Mohammed Alqahtani"},
	{"MOHAMMED ABDULLAH A ALOMARI", "Mohammed Bin Abdullah Al Omari"},
	{"FAHAD ABDULLAH S ALDOSSARI", "Fahad Bin Abdullah Aldossari"},
	{"ABDULLAH MOHAMMED ABDULLAH ALSHEHRI", "Abdullah Bin Mohammed Bin Abdullah Al Shehri"},
	{"ALI ABDULLAH S ALQARNI", "Ali Abdullah D. Alqarni"},
	{"IBRAHIM MOHAMMED I ALKHENIZAN", "Ibrahim Mohammed Al-Khenizan"},
	{"MOHAMMED ALI MOHAMMED ALMALKI", "Mohammad Bin Ali Mohammed Al Malki"},
	{"SAMI AL ASKARI", "Sami Askari"},
	{"ABRAR HUSSAIN", "Abrar Hussain"},
}

const alertThreshold = 75

func TestGenerationalCorpusDropsBelowThreshold(t *testing.T) {
	worst := 0
	for _, c := range generationalCorpus {
		got := ScoreNameV2(c.query, c.candidate)
		if got > worst {
			worst = got
		}
		t.Logf("was=%3d now=%3d  %q vs %q", c.observed, got, c.query, c.candidate)
		if got >= alertThreshold {
			t.Errorf("ScoreNameV2(%q, %q) = %d, want < %d",
				c.query, c.candidate, got, alertThreshold)
		}
	}
	t.Logf("highest remaining = %d (threshold %d)", worst, alertThreshold)
}

func TestSamePersonCorpusKeepsAlerting(t *testing.T) {
	lowest := 100
	for _, c := range samePersonCorpus {
		got := ScoreNameV2(c.query, c.candidate)
		if got < lowest {
			lowest = got
		}
		t.Logf("score=%3d  %q vs %q", got, c.query, c.candidate)
		if got < alertThreshold {
			t.Errorf("ScoreNameV2(%q, %q) = %d, want >= %d",
				c.query, c.candidate, got, alertThreshold)
		}
	}
	t.Logf("lowest true positive = %d (threshold %d)", lowest, alertThreshold)
}

// TestScoreNameV2HoldsFalsePositiveCorpus reuses the corpus the current scorer
// is already regression-tested against, so the replacement cannot reintroduce
// anything that one fixed.
func TestScoreNameV2HoldsFalsePositiveCorpus(t *testing.T) {
	worst := 0
	for _, c := range falsePositiveCorpus {
		if c.entity {
			continue
		}
		got := ScoreNameV2(c.query, c.candidate)
		if got > worst {
			worst = got
		}
		if got >= alertThreshold {
			t.Errorf("ScoreNameV2(%q, %q) = %d, want < %d",
				c.query, c.candidate, got, alertThreshold)
		}
	}
	t.Logf("highest remaining across the existing corpus = %d", worst)
}

func TestGenerationalSwapDetection(t *testing.T) {
	tests := []struct {
		name  string
		query string
		cand  string
		want  bool
	}{
		{"chain shifted one generation", "محمد ناصر محمد الهاجري", "ناصر محمد ناصر الهاجري", true},
		{"given names swapped", "MOHAMMED HASSAN AHMED ALI", "Ahmed Hassan Mohammed Ali", true},
		{"same person, aligned chain", "MOHAMMED ABDULLAH A ALOMARI", "Mohammed Bin Abdullah Al Omari", false},
		{"two-part western flip is exempt", "John Smith", "Smith John", false},
		{"unrelated names", "MOHAMMED MUTAB E ALHARBI", "Sara Osman Ahmed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := contentTokens(normalize(tt.query))
			c := contentTokens(normalize(tt.cand))
			if got := generationalSwap(q, c); got != tt.want {
				t.Errorf("generationalSwap(%v, %v) = %t, want %t", q, c, got, tt.want)
			}
		})
	}
}

func TestOrderAlignmentPenalisesReversal(t *testing.T) {
	forward := contentTokens(normalize("ahmed hassan mohammed ali"))
	reversed := contentTokens(normalize("ali mohammed hassan ahmed"))

	aligned := orderAlignment(forward, forward)
	scrambled := orderAlignment(forward, reversed)

	if aligned != 100 {
		t.Errorf("orderAlignment(x, x) = %d, want 100", aligned)
	}
	if scrambled >= aligned {
		t.Errorf("orderAlignment on a reversed chain = %d, want < %d", scrambled, aligned)
	}
}

func TestChainAlignmentWeightsTheFatherSlot(t *testing.T) {
	query := contentTokens(normalize("مسلم زيد بن محمد السبيعي"))
	sameFather := contentTokens(normalize("مسلم زيد محمد السبيعي"))
	otherFather := contentTokens(normalize("مسلم محمد مسلم السبيعي"))

	with := chainAlignment(query, sameFather)
	without := chainAlignment(query, otherFather)
	if with <= without {
		t.Errorf("chainAlignment with matching father = %d, with differing father = %d; want the first to be higher",
			with, without)
	}
}

func TestTokenWeightsBands(t *testing.T) {
	// 10,000 records, so that a token appearing once really is rare by share
	// and not merely rare by count.
	const records = 10000
	builder := NewTokenWeightsBuilder()
	for i := 0; i < records; i++ {
		switch {
		case i == 0:
			builder.AddRecord([]string{"Mohammed Ahmed Alkhenizan"}) // 0.01%
		case i < 30:
			builder.AddRecord([]string{"Mohammed Ahmed Alharbi"}) // 0.3%
		case i < 300:
			builder.AddRecord([]string{"Mohammed Ahmed Alqahtani"}) // 2.7%
		default:
			builder.AddRecord([]string{"Mohammed Ahmed Alshehri"})
		}
	}
	w := builder.Build()

	if got := w.Documents(); got != records {
		t.Fatalf("Documents() = %d, want %d", got, records)
	}

	tests := []struct {
		token string
		want  int
	}{
		{"mohammed", commonTokenWeight},    // in every record
		{"alshehri", commonTokenWeight},    // 97%
		{"alqahtani", frequentTokenWeight}, // 2.7%
		{"alharbi", uncommonTokenWeight},   // 0.3%
		{"alkhenizan", rareTokenWeight},    // 0.01%
	}
	for _, tt := range tests {
		if got := w.Weight(tt.token); got != tt.want {
			t.Errorf("Weight(%q) = %d, want %d", tt.token, got, tt.want)
		}
	}

	// The definite article must not split one token into two.
	if w.Weight("alharbi") != w.Weight("harbi") {
		t.Errorf("Weight(alharbi) = %d, Weight(harbi) = %d; want equal",
			w.Weight("alharbi"), w.Weight("harbi"))
	}
}

// TestTokenWeightsCountRecordsNotRows guards the rule that makes the weighting
// work. A record carrying many spelling variations of one name must count once,
// or it inflates the apparent frequency of its own surname and pushes a
// discriminating token into a low-weight band.
func TestTokenWeightsCountRecordsNotRows(t *testing.T) {
	variations := []string{
		"Ibrahim Mohammed Al-Khenizan",
		"Ibrahim Mohammed Alkhenizan",
		"Ibrahim Mohammed Al Khenizan",
		"ابراهيم محمد الخنيزان",
	}

	// A corpus of 2,000 straddles a band edge for this token: counted once it
	// is 0.05% and rare, counted four times it is 0.2% and merely uncommon.
	const corpus = 2000
	filler := func(b *TokenWeightsBuilder, n int) {
		for i := 0; i < n; i++ {
			b.AddRecord([]string{"Mohammed Ahmed Alshehri"})
		}
	}

	// One record holding every variation.
	groupedBuilder := NewTokenWeightsBuilder()
	groupedBuilder.AddRecord(variations)
	filler(groupedBuilder, corpus-1)
	grouped := groupedBuilder.Build()

	// The same variations spread over separate records, which is what counting
	// name rows instead of records would amount to.
	splitBuilder := NewTokenWeightsBuilder()
	for _, v := range variations {
		splitBuilder.AddRecord([]string{v})
	}
	filler(splitBuilder, corpus-len(variations))
	split := splitBuilder.Build()

	if got := grouped.Documents(); got != corpus {
		t.Fatalf("grouped Documents() = %d, want %d", got, corpus)
	}
	if got := grouped.Weight("alkhenizan"); got != rareTokenWeight {
		t.Errorf("grouped Weight(alkhenizan) = %d, want %d", got, rareTokenWeight)
	}
	if grouped.Weight("alkhenizan") <= split.Weight("alkhenizan") {
		t.Errorf("grouping a record's variations gave weight %d, splitting them gave %d; want grouping to rate the token rarer",
			grouped.Weight("alkhenizan"), split.Weight("alkhenizan"))
	}
}

func TestFallbackWeightsApplyWithoutATable(t *testing.T) {
	if TokenWeightsLoaded() {
		t.Skip("a weights table is installed; fallback path not exercised")
	}
	if got := tokenWeight("mohammed"); got != commonTokenWeight {
		t.Errorf("fallback weight for mohammed = %d, want %d", got, commonTokenWeight)
	}
	if got := tokenWeight("alkhenizan"); got != rareTokenWeight {
		t.Errorf("fallback weight for alkhenizan = %d, want %d", got, rareTokenWeight)
	}
}

// TestLabelledSampleMetrics reports precision and recall of both scorers over
// the hand-labelled sample in analysis/labels.tsv. It asserts only that the
// replacement is not worse than what it replaces; the absolute numbers are
// logged so a tuning change can be judged rather than guessed at.
func TestLabelledSampleMetrics(t *testing.T) {
	const path = "../../analysis/labels.tsv"
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("labelled sample not available: %v", err)
	}
	defer f.Close()

	type pair struct {
		positive bool
		query    string
		cand     string
	}
	var pairs []pair

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 4 || (parts[0] != "0" && parts[0] != "1") {
			continue // ambiguous rows are excluded from the metrics
		}
		pairs = append(pairs, pair{positive: parts[0] == "1", query: parts[2], cand: parts[3]})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if len(pairs) == 0 {
		t.Skip("labelled sample held no usable rows")
	}

	measure := func(score func(string, string) int, threshold int) (tp, fp, fn int) {
		for _, p := range pairs {
			alert := score(p.query, p.cand) >= threshold
			switch {
			case alert && p.positive:
				tp++
			case alert && !p.positive:
				fp++
			case !alert && p.positive:
				fn++
			}
		}
		return
	}
	pct := func(a, b int) int {
		if a+b == 0 {
			return 0
		}
		return a * 100 / (a + b)
	}

	t.Logf("labelled sample: %d pairs", len(pairs))
	for _, threshold := range []int{70, 75, 80, 85} {
		liveTP, liveFP, liveFN := measure(ScoreName, threshold)
		newTP, newFP, newFN := measure(ScoreNameV2, threshold)

		livePrec, liveRec := pct(liveTP, liveFP), pct(liveTP, liveFN)
		newPrec, newRec := pct(newTP, newFP), pct(newTP, newFN)

		t.Logf("threshold=%d  live: prec=%3d%% recall=%3d%% (tp=%d fp=%d fn=%d)  v2: prec=%3d%% recall=%3d%% (tp=%d fp=%d fn=%d)",
			threshold, livePrec, liveRec, liveTP, liveFP, liveFN,
			newPrec, newRec, newTP, newFP, newFN)

		if newPrec < livePrec {
			t.Errorf("threshold %d: v2 precision %d%% is below live precision %d%%",
				threshold, newPrec, livePrec)
		}
		if newRec < liveRec {
			t.Errorf("threshold %d: v2 recall %d%% is below live recall %d%%",
				threshold, newRec, liveRec)
		}
	}
}
