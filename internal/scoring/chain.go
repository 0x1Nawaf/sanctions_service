package scoring

// ScoreNameV2 is the candidate replacement for ScoreName. It is run in shadow
// mode alongside the current scorer and does not yet affect any result.
//
// It differs from ScoreName in three ways, each aimed at a failure mode
// measured over a 7,000-client screening run:
//
//  1. Tokens are weighted by how rare the name is, so a chain of the commonest
//     given names cannot reach the top of the scale on its own.
//  2. Position along the name chain is scored, not just membership in it. In
//     that run 51% of alerts matched the client's given name at a *different*
//     place in the candidate's chain — the signature of a relative a
//     generation removed, which a bag of tokens cannot distinguish from the
//     person themselves.
//  3. A match built only from very common name parts is capped, because it
//     does not identify anybody without a second identifier to confirm it.
func ScoreNameV2(searchName, candidateName string) int {
	search := normalize(searchName)
	candidate := normalize(candidateName)

	if search == "" || candidate == "" {
		return 0
	}
	if search == candidate {
		return 100
	}

	searchTokens := contentTokens(search)
	candidateTokens := contentTokens(candidate)
	if len(searchTokens) == 0 || len(candidateTokens) == 0 {
		return 0
	}

	forward, matchedMass := weightedBestTokenSimilarity(searchTokens, candidateTokens)
	reverse, _ := weightedBestTokenSimilarity(candidateTokens, searchTokens)
	core := min(forward, reverse)

	chain := chainAlignment(searchTokens, candidateTokens)
	full := levenshteinSimilarity(search, candidate)

	// The token-count ratio the current blend carries is dropped: it rewarded
	// two names for having the same number of parts regardless of which parts
	// those were, and the chain term's order component covers the same ground
	// with the identity of the tokens taken into account.
	total := (core*coreWeight + chain*chainWeight + full*fullWeight) / 100

	total = subsetBoost(total, forward, len(searchTokens), len(candidateTokens))

	if generationalSwap(searchTokens, candidateTokens) {
		total = total * generationalSwapPenalty / 100
	}

	total = weakEvidenceCap(total, chain, matchedMass)

	if total > 100 {
		return 100
	}
	if total < 0 {
		return 0
	}
	return total
}

const (
	coreWeight  = 60
	chainWeight = 25
	fullWeight  = 15
)

// contentTokens reduces a normalised name to the parts that carry identity:
// initials and patronymic connectors removed, with the same guard ScoreName
// applies so that filtering never empties the name.
func contentTokens(normalized string) []string {
	tokens := tokenize(normalized)
	significant := filterShortTokens(tokens)
	if len(significant) == 0 {
		return tokens
	}
	return filterNameNoise(significant)
}

// chainMatchFloor is the similarity at which two tokens are treated as the same
// name part for the purpose of locating it in the chain.
const chainMatchFloor = 72

// Weights within the chain term. The father slot earns more than its share of
// the name because it is what separates brothers and cousins, who already share
// both the given-name pool and the family name and so agree everywhere else.
const (
	chainHeadWeight   = 35
	chainFatherWeight = 20
	chainFamilyWeight = 30
	chainOrderWeight  = 15

	chainHeadWeightNoFather   = 45
	chainFamilyWeightNoFather = 35
	chainOrderWeightNoFather  = 20
)

// chainAlignment scores how well two names line up as chains rather than as
// bags of tokens.
//
// An Arabic name is given + father + grandfather + ... + family. Each
// generation drops the oldest link and prepends a new one, so a father and his
// son share almost their entire token set in a shifted order. Membership alone
// therefore cannot tell them apart; position can.
func chainAlignment(searchTokens, candidateTokens []string) int {
	head := tokenSimilarity(searchTokens[0], candidateTokens[0])
	family := tokenSimilarity(
		searchTokens[len(searchTokens)-1],
		candidateTokens[len(candidateTokens)-1],
	)
	order := orderAlignment(searchTokens, candidateTokens)

	if len(searchTokens) >= 3 && len(candidateTokens) >= 3 {
		father := tokenSimilarity(searchTokens[1], candidateTokens[1])
		return (head*chainHeadWeight +
			father*chainFatherWeight +
			family*chainFamilyWeight +
			order*chainOrderWeight) / 100
	}

	return (head*chainHeadWeightNoFather +
		family*chainFamilyWeightNoFather +
		order*chainOrderWeightNoFather) / 100
}

// orderAlignment reports what share of the longer name is covered by matched
// parts that appear in the same relative order on both sides.
//
// Pairing each query token with its best unused counterpart and then taking the
// longest increasing run of those positions means a pair of names that share
// every token but reverse its order scores far below one that shares the same
// tokens in sequence.
func orderAlignment(searchTokens, candidateTokens []string) int {
	used := make([]bool, len(candidateTokens))
	positions := make([]int, 0, len(searchTokens))

	for _, st := range searchTokens {
		bestIndex, bestScore := -1, 0
		for j, ct := range candidateTokens {
			if used[j] {
				continue
			}
			if sim := tokenSimilarity(st, ct); sim > bestScore {
				bestIndex, bestScore = j, sim
			}
		}
		if bestScore >= chainMatchFloor {
			used[bestIndex] = true
			positions = append(positions, bestIndex)
		}
	}

	if len(positions) == 0 {
		return 0
	}
	return longestIncreasingRun(positions) * 100 / max(len(searchTokens), len(candidateTokens))
}

// longestIncreasingRun returns the length of the longest strictly increasing
// subsequence of positions, via the usual patience-sorting tails array.
func longestIncreasingRun(positions []int) int {
	tails := make([]int, 0, len(positions))
	for _, p := range positions {
		lo, hi := 0, len(tails)
		for lo < hi {
			mid := (lo + hi) / 2
			if tails[mid] < p {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo == len(tails) {
			tails = append(tails, p)
		} else {
			tails[lo] = p
		}
	}
	return len(tails)
}

// generationalSwapPenalty is applied when two chains hold the same names in a
// shifted order. It is a multiplier rather than a rejection because the pattern
// is strong evidence of a relative, not proof of one.
const generationalSwapPenalty = 72

// generationalSwap reports the father/son signature: the query's given name
// sits deeper in the candidate's chain, and the candidate's given name sits
// deeper in the query's. Both halves are required, so an ordinary spelling
// variant of the given name — which fails the first half — is untouched.
//
// Chains shorter than three parts are exempt. Two-part names are routinely
// stored surname-first by Western systems, and penalising that would cost real
// matches for no gain.
func generationalSwap(searchTokens, candidateTokens []string) bool {
	if len(searchTokens) < 3 || len(candidateTokens) < 3 {
		return false
	}
	if tokenSimilarity(searchTokens[0], candidateTokens[0]) >= chainMatchFloor {
		return false
	}
	return appearsIn(searchTokens[0], candidateTokens[1:]) &&
		appearsIn(candidateTokens[0], searchTokens[1:])
}

func appearsIn(token string, tokens []string) bool {
	for _, t := range tokens {
		if tokenSimilarity(token, t) >= chainMatchFloor {
			return true
		}
	}
	return false
}

const (
	// minEvidenceMass is the summed token weight, in percent, that a match has
	// to account for before it is treated as identifying a person rather than
	// describing a common name. Roughly one and a half rare tokens, or four
	// of the commonest.
	minEvidenceMass = 150
	// weakEvidenceCeiling is where such a match is held instead.
	weakEvidenceCeiling = 74
	// cleanChainAlignment is the chain score above which the cap is lifted.
	cleanChainAlignment = 95
)

// weakEvidenceCap holds down a match assembled entirely from name parts too
// common to single anybody out.
//
// The cap is lifted when the chain lines up cleanly, because there the name
// really is the same name: the residual doubt is "a great many people are
// called this", which only a second identifier — date of birth, citizenship,
// gender — can settle. Suppressing the alert would hide it from exactly that
// check, so it is left at full strength for a reviewer to resolve.
func weakEvidenceCap(total, chain, matchedMass int) int {
	if matchedMass >= minEvidenceMass || chain >= cleanChainAlignment {
		return total
	}
	if total > weakEvidenceCeiling {
		return weakEvidenceCeiling
	}
	return total
}

// weightedBestTokenSimilarity scores how well each source token is represented
// among the target tokens, weighting each by how rare the source token is. It
// also reports the summed weight of the source tokens that actually found a
// match, which is the evidence the match rests on.
func weightedBestTokenSimilarity(sourceTokens, targetTokens []string) (score, matchedMass int) {
	if len(sourceTokens) == 0 {
		return 0, 0
	}
	weightedTotal, weightSum := 0, 0
	for _, st := range sourceTokens {
		w := tokenWeight(st)
		best := 0
		for _, tt := range targetTokens {
			if sim := tokenSimilarity(st, tt); sim > best {
				best = sim
			}
		}
		weightedTotal += w * best
		weightSum += w
		if best >= chainMatchFloor {
			matchedMass += w
		}
	}
	if weightSum == 0 {
		return 0, 0
	}
	return weightedTotal / weightSum, matchedMass
}
