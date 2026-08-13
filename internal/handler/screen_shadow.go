package handler

import "log"

// shadowComparison counts how the candidate scorer would have differed from the
// live one on a single query, so a production run can be measured without any
// change to what callers receive.
type shadowComparison struct {
	// agreed: both scorers alert on the record.
	agreed int
	// suppressed: the live scorer alerts, the candidate scorer does not.
	suppressed int
	// promoted: the candidate scorer alerts, the live scorer does not.
	promoted int
	// liveTotal is how many records the live scorer alerts on, which is what
	// the caller actually receives.
	liveTotal int
	// scoreDelta is the summed change in score over records both scorers
	// alert on, for a mean shift.
	scoreDelta int
}

func (c *shadowComparison) observe(s recordScore, minScore int) {
	liveAlerts := minScore <= 0 || s.score >= minScore
	shadowAlerts := minScore <= 0 || s.shadowScore >= minScore

	if liveAlerts {
		c.liveTotal++
	}
	switch {
	case liveAlerts && shadowAlerts:
		c.agreed++
		c.scoreDelta += s.shadowScore - s.score
	case liveAlerts && !shadowAlerts:
		c.suppressed++
	case !liveAlerts && shadowAlerts:
		c.promoted++
	}
}

func logShadowComparison(query, searchType string, minScore int, c shadowComparison) {
	meanDelta := 0
	if c.agreed > 0 {
		meanDelta = c.scoreDelta / c.agreed
	}
	log.Printf(
		"screen shadow query=%q type=%s min_score=%d live_alerts=%d agreed=%d suppressed=%d promoted=%d mean_delta=%d",
		query, searchType, minScore, c.liveTotal, c.agreed, c.suppressed, c.promoted, meanDelta,
	)
}
