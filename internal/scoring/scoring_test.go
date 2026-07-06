package scoring

import "testing"

func TestSubsetBoostFix(t *testing.T) {
	tests := []struct {
		name      string
		search    string
		candidate string
		minScore  int
		maxScore  int
	}{
		{
			name:      "common names scattered in longer name should not score high",
			search:    "Mohammed Hassan Ali",
			candidate: "Sana Mohammed Suhail Ali Hassan",
			minScore:  0,
			maxScore:  70,
		},
		{
			name:      "exact match must be 100",
			search:    "Mohammed Hassan Ali",
			candidate: "Mohammed Hassan Ali",
			minScore:  100,
			maxScore:  100,
		},
		{
			name:      "close spelling variant should score high",
			search:    "Mohammed Hassan Ali",
			candidate: "Mohamed Hasan Ali",
			minScore:  80,
			maxScore:  100,
		},
		{
			name:      "4-token legitimate subset should still score well",
			search:    "Mohammed Hassan Ali Ahmed",
			candidate: "Mohammed Hassan Ali Ahmed Al-Rashid",
			minScore:  70,
			maxScore:  100,
		},
		{
			name:      "same tokens different person with extra names",
			search:    "Ali Hassan Mohammed",
			candidate: "Fatima Ali Noor Hassan Youssef Mohammed",
			minScore:  0,
			maxScore:  70,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ScoreName(tt.search, tt.candidate)
			if score > tt.maxScore {
				t.Errorf("ScoreName(%q, %q) = %d, want <= %d",
					tt.search, tt.candidate, score, tt.maxScore)
			}
			if score < tt.minScore {
				t.Errorf("ScoreName(%q, %q) = %d, want >= %d",
					tt.search, tt.candidate, score, tt.minScore)
			}
			t.Logf("ScoreName(%q, %q) = %d", tt.search, tt.candidate, score)
		})
	}
}

func TestFalsePositiveReduction(t *testing.T) {
	tests := []struct {
		name      string
		search    string
		candidate string
		minScore  int
		maxScore  int
	}{
		{
			name:      "true positive - surname matches with extra middle names",
			search:    "Nouf Alkahtani",
			candidate: "Nouf Mohammed M. Alkahtani",
			minScore:  70,
			maxScore:  100,
		},
		{
			name:      "false positive - same first name different surname",
			search:    "Nouf Alkahtani",
			candidate: "Nouf Al-Sowaidi",
			minScore:  0,
			maxScore:  45,
		},
		{
			name:      "false positive - same first name unrelated surname",
			search:    "Nouf Alkahtani",
			candidate: "Nouf M. A. M. K. Al Shibani",
			minScore:  0,
			maxScore:  45,
		},
		{
			name:      "false positive - similar first name different surname",
			search:    "Nouf Alkahtani",
			candidate: "Noof Al Hammadi",
			minScore:  0,
			maxScore:  45,
		},
		{
			name:      "partial name match - truncated surname in longer patronymic",
			search:    "Muhammed Al Shurf",
			candidate: "Muhammed Ali Muhammed Al Shurafa Al Hammadi",
			minScore:  55,
			maxScore:  100,
		},
		{
			name:      "true positive - 2-token subset of 3-token name",
			search:    "Ahmed Alharbi",
			candidate: "Ahmed Mohammed Alharbi",
			minScore:  70,
			maxScore:  100,
		},
		{
			name:      "false positive - first name only overlap",
			search:    "Ahmed Alharbi",
			candidate: "Ahmed Al-Sowaidi",
			minScore:  0,
			maxScore:  45,
		},
		{
			name:      "true positive - spelling variant of surname",
			search:    "Fahad Al Otaibi",
			candidate: "Fahd Al Otaibi",
			minScore:  75,
			maxScore:  100,
		},
		{
			name:      "false positive - same tribe prefix different name",
			search:    "Fahad Al Otaibi",
			candidate: "Fahad Al Dosari",
			minScore:  0,
			maxScore:  50,
		},
		{
			name:      "true positive - long patronymic chain with Bin/Bint connectors",
			search:    "NOUF FAHAD KHALID AL SAUD",
			candidate: "Princess Nouf Bint Fahd Bin Khalid Bin Muhammad Bin Abdul Rahman Al Saud",
			minScore:  70,
			maxScore:  100,
		},
		{
			name:      "true positive - name with bin connectors should still match",
			search:    "Osama Bin Laden",
			candidate: "Usama Bin Muhammad Bin Awad Bin Laden",
			minScore:  55,
			maxScore:  100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ScoreName(tt.search, tt.candidate)
			if score > tt.maxScore {
				t.Errorf("ScoreName(%q, %q) = %d, want <= %d",
					tt.search, tt.candidate, score, tt.maxScore)
			}
			if score < tt.minScore {
				t.Errorf("ScoreName(%q, %q) = %d, want >= %d",
					tt.search, tt.candidate, score, tt.minScore)
			}
			t.Logf("ScoreName(%q, %q) = %d", tt.search, tt.candidate, score)
		})
	}
}

func TestScoreEntityName(t *testing.T) {
	tests := []struct {
		name      string
		search    string
		candidate string
		minScore  int
		maxScore  int
	}{
		{
			name:      "exact entity match",
			search:    "Al Rashid Trading Company",
			candidate: "Al Rashid Trading Company",
			minScore:  100,
			maxScore:  100,
		},
		{
			name:      "same entity different legal suffix",
			search:    "Al Rashid Trading Company",
			candidate: "Al Rashid Trading Ltd",
			minScore:  80,
			maxScore:  100,
		},
		{
			name:      "same core name different business type",
			search:    "Al Rashid Trading Company",
			candidate: "Al Rashid Shipping Ltd",
			minScore:  70,
			maxScore:  100,
		},
		{
			name:      "different entity same noise words",
			search:    "Al Rashid Trading Company",
			candidate: "Al Saud Trading Company",
			minScore:  0,
			maxScore:  75,
		},
		{
			name:      "completely different entities sharing only noise",
			search:    "Mahan Air International",
			candidate: "Petronas International Trading",
			minScore:  0,
			maxScore:  40,
		},
		{
			name:      "similar entity names should score high",
			search:    "Bank Melli Iran",
			candidate: "Bank Melli Iran",
			minScore:  100,
			maxScore:  100,
		},
		{
			name:      "different banks should not match well",
			search:    "Bank Melli Iran",
			candidate: "Bank Saderat Iran",
			minScore:  0,
			maxScore:  75,
		},
		{
			name:      "entity with slight spelling difference",
			search:    "Hezbollah",
			candidate: "Hizballah",
			minScore:  50,
			maxScore:  100,
		},
		{
			name:      "entity name vs unrelated entity sharing generic words",
			search:    "Islamic Revolutionary Guard Corps",
			candidate: "Islamic Relief Foundation",
			minScore:  0,
			maxScore:  55,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ScoreEntityName(tt.search, tt.candidate)
			if score > tt.maxScore {
				t.Errorf("ScoreEntityName(%q, %q) = %d, want <= %d",
					tt.search, tt.candidate, score, tt.maxScore)
			}
			if score < tt.minScore {
				t.Errorf("ScoreEntityName(%q, %q) = %d, want >= %d",
					tt.search, tt.candidate, score, tt.minScore)
			}
			t.Logf("ScoreEntityName(%q, %q) = %d", tt.search, tt.candidate, score)
		})
	}
}
