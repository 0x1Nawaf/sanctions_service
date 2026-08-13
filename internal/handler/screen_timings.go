package handler

import (
	"log"
	"time"
)

type screenPhaseTimings struct {
	fetchCandidates time.Duration
	expandNames     time.Duration
	score           time.Duration
	likeRetry       time.Duration
	factors         time.Duration
	hydrate         time.Duration
	total           time.Duration
}

func logScreenTimings(query, searchType string, t screenPhaseTimings, initialCandidates, expandedRecords, results int, usedLike, usedBroad bool) {
	log.Printf(
		"screen timing query=%q type=%s fetch=%s expand=%s score=%s like_retry=%s factors=%s hydrate=%s total=%s initial_candidates=%d expanded_records=%d results=%d used_like=%t used_broad=%t",
		query,
		searchType,
		t.fetchCandidates,
		t.expandNames,
		t.score,
		t.likeRetry,
		t.factors,
		t.hydrate,
		t.total,
		initialCandidates,
		expandedRecords,
		results,
		usedLike,
		usedBroad,
	)
}
