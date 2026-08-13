package scoring

import "sync/atomic"

// Token weights, in percent, applied to a token's contribution to the match.
//
// Matching on a name part that a quarter of the region shares is not the same
// evidence as matching on a rare family name. In the 7k-client sample that
// motivated this, "mohammed" appeared in 28% of client names and "abdullah" in
// 24%, yet each counted for exactly as much as a distinctive surname, which is
// how records assembled entirely from high-frequency names reached the top of
// the scale.
//
// The scale bottoms out at commonTokenWeight rather than zero: a common token
// still has to be accounted for, it just cannot carry a match by itself.
const (
	commonTokenWeight   = 20
	frequentTokenWeight = 35
	ordinaryTokenWeight = 55
	uncommonTokenWeight = 80
	rareTokenWeight     = 100
)

// Document-frequency band edges, in basis points of the corpus.
const (
	commonTokenShare   = 500 // 5%
	frequentTokenShare = 200 // 2%
	ordinaryTokenShare = 50  // 0.5%
	uncommonTokenShare = 10  // 0.1%
)

// TokenWeights reports how much evidence a match on a name token carries,
// derived from how many records in the feed contain that token.
//
// A document is one record, not one name row. A record can carry dozens of
// spelling variations of the same name, and counting rows would let a record
// inflate the apparent frequency of its own surname — pushing a genuinely rare
// and therefore highly discriminating token into the low-weight band, which is
// the opposite of what the weighting is for.
type TokenWeights struct {
	documents int
	df        map[string]int
}

// Weight returns the percent weight for an already-normalised token.
func (w *TokenWeights) Weight(token string) int {
	key := stripArticle(token)
	if w == nil || w.documents == 0 {
		return fallbackWeight(key)
	}
	// Integer comparison against basis-point band edges, to keep floating
	// point off a path that runs for every query token against every
	// candidate token.
	share := w.df[key] * 10000
	switch {
	case share >= w.documents*commonTokenShare:
		return commonTokenWeight
	case share >= w.documents*frequentTokenShare:
		return frequentTokenWeight
	case share >= w.documents*ordinaryTokenShare:
		return ordinaryTokenWeight
	case share >= w.documents*uncommonTokenShare:
		return uncommonTokenWeight
	default:
		return rareTokenWeight
	}
}

// Documents reports the size of the corpus the table was built from.
func (w *TokenWeights) Documents() int {
	if w == nil {
		return 0
	}
	return w.documents
}

// Distinct reports how many distinct tokens the table holds.
func (w *TokenWeights) Distinct() int {
	if w == nil {
		return 0
	}
	return len(w.df)
}

// TokenWeightsBuilder accumulates document frequencies one record at a time.
type TokenWeightsBuilder struct {
	documents int
	df        map[string]int
	seen      map[string]bool
}

func NewTokenWeightsBuilder() *TokenWeightsBuilder {
	return &TokenWeightsBuilder{
		df:   make(map[string]int),
		seen: make(map[string]bool),
	}
}

// AddRecord folds one record's name variants into the table. Every variant of
// the record must be supplied in the same call, so that a token repeated across
// variants counts once.
func (b *TokenWeightsBuilder) AddRecord(names []string) {
	for k := range b.seen {
		delete(b.seen, k)
	}
	counted := false
	for _, name := range names {
		for _, token := range contentTokens(normalize(name)) {
			key := stripArticle(token)
			if key == "" || b.seen[key] {
				continue
			}
			b.seen[key] = true
			b.df[key]++
			counted = true
		}
	}
	if counted {
		b.documents++
	}
}

func (b *TokenWeightsBuilder) Build() *TokenWeights {
	return &TokenWeights{documents: b.documents, df: b.df}
}

var activeWeights atomic.Pointer[TokenWeights]

// SetTokenWeights installs the frequency table used by ScoreNameV2. It is safe
// to call while screening is in flight, so the table can be rebuilt after a
// seeder run without restarting the service.
func SetTokenWeights(w *TokenWeights) {
	activeWeights.Store(w)
}

// TokenWeightsLoaded reports whether a table has been installed.
func TokenWeightsLoaded() bool {
	return activeWeights.Load() != nil
}

func tokenWeight(token string) int {
	return activeWeights.Load().Weight(token)
}

// fallbackCommonTokens are the name parts frequent enough across Gulf and wider
// Arabic naming that treating them as ordinary evidence visibly distorts
// scoring. They stand in until the real table is built from the database, so
// that a service which has not finished loading — or one pointed at a feed too
// small to measure — still refuses to be convinced by a chain of the commonest
// names. Entries are stored article-stripped, matching the lookup key.
var fallbackCommonTokens = fallbackSet(
	"mohammed", "mohamed", "muhammad", "mohammad", "mohamad", "muhammed",
	"ahmed", "ahmad", "ahmet",
	"abdullah", "abdallah", "abdulla",
	"ali", "hassan", "hasan", "hussain", "hussein", "husain",
	"saad", "saeed", "said", "salem", "salim", "saleh", "salah",
	"khalid", "khaled", "fahad", "fahd", "nasser", "naser",
	"abdulaziz", "abdulrahman", "abdelrahman", "abdulkarim", "abdullatif",
	"ibrahim", "ismail", "omar", "othman", "osman", "yousef", "yusuf",
	"mahmoud", "mahmud", "mustafa", "moustafa", "sultan", "turki", "faisal",
	"محمد", "احمد", "عبدالله", "علي", "حسن", "حسين", "سعد", "سعيد",
	"صالح", "سالم", "خالد", "فهد", "ناصر", "عبدالعزيز", "عبدالرحمن",
	"ابراهيم", "عمر", "عثمان", "يوسف", "محمود", "مصطفى", "سلطان", "فيصل",
)

func fallbackWeight(key string) int {
	if fallbackCommonTokens[key] {
		return commonTokenWeight
	}
	return rareTokenWeight
}

// fallbackSet normalises and article-strips its entries so they can be written
// in their natural spelling.
func fallbackSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if n := normalize(w); n != "" {
			set[stripArticle(n)] = true
		}
	}
	return set
}
