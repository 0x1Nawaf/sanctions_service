package handler

import "testing"

func TestMergeCandidateScoresShadowDisabledIsUnchanged(t *testing.T) {
	candidates := []nameCandidate{
		{recordID: 1, name: "Abdullah Bin Mohammed Alqahtani"},
		{recordID: 1, name: "Something Entirely Different"},
		{recordID: 2, name: "Ahmed Hassan Mohammed Ali"},
	}

	best := make(map[uint32]recordScore)
	mergeCandidateScores(best, candidates, "ABDULLAH MOHAMMED H ALQAHTANI", "individual", 75, false)

	got, ok := best[1]
	if !ok {
		t.Fatal("expected record 1 to be retained")
	}
	if got.name != "Abdullah Bin Mohammed Alqahtani" {
		t.Errorf("matched name = %q, want the best-scoring variant", got.name)
	}
	if got.shadowScore != 0 {
		t.Errorf("shadowScore = %d, want 0 when shadow scoring is off", got.shadowScore)
	}
	if _, ok := best[2]; ok {
		t.Error("expected record 2 to be filtered out by min_score")
	}
}

func TestMergeCandidateScoresShadowRetainsPromotions(t *testing.T) {
	// A generational swap: the live scorer alerts, the candidate scorer does
	// not. The record must still be retained so the difference is observable.
	candidates := []nameCandidate{
		{recordID: 1, name: "Ahmed Hassan Mohammed Ali"},
	}

	best := make(map[uint32]recordScore)
	mergeCandidateScores(best, candidates, "MOHAMMED HASSAN AHMED ALI", "individual", 75, true)

	got, ok := best[1]
	if !ok {
		t.Fatal("expected record 1 to be retained")
	}
	if got.score < 75 {
		t.Fatalf("live score = %d, expected this pair to alert under the live scorer", got.score)
	}
	if got.shadowScore >= 75 {
		t.Errorf("shadow score = %d, expected the candidate scorer to suppress a generational swap", got.shadowScore)
	}
}

func TestShadowComparisonCounts(t *testing.T) {
	// observe reads the post-adjustment scores, which equal the name scores
	// when no secondary identifier was supplied.
	var c shadowComparison
	c.observe(recordScore{finalScore: 90, shadowFinalScore: 95}, 75) // both alert
	c.observe(recordScore{finalScore: 88, shadowFinalScore: 40}, 75) // live only
	c.observe(recordScore{finalScore: 60, shadowFinalScore: 80}, 75) // shadow only
	c.observe(recordScore{finalScore: 30, shadowFinalScore: 20}, 75) // neither

	if c.liveTotal != 2 {
		t.Errorf("liveTotal = %d, want 2", c.liveTotal)
	}
	if c.agreed != 1 {
		t.Errorf("agreed = %d, want 1", c.agreed)
	}
	if c.suppressed != 1 {
		t.Errorf("suppressed = %d, want 1", c.suppressed)
	}
	if c.promoted != 1 {
		t.Errorf("promoted = %d, want 1", c.promoted)
	}
	if c.scoreDelta != 5 {
		t.Errorf("scoreDelta = %d, want 5", c.scoreDelta)
	}
}
