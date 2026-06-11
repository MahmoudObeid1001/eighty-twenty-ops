package handlers

import (
	"strings"
	"testing"
)

func TestBuildRenewalPendingMessage(t *testing.T) {
	tests := []struct {
		name             string
		fullName         string
		level            int
		outcome          string
		attendedSessions int
		gender           string
		wantKey          string
		wantLabel        string
		wantContains     string
	}{
		{
			name:             "promoted uses promoted template",
			fullName:         "Ahmed Ali",
			level:            3,
			outcome:          "promoted",
			attendedSessions: 1,
			gender:           "male",
			wantKey:          "promoted",
			wantLabel:        "Promoted",
			wantContains:     "ابعت المبلغ",
		},
		{
			name:             "repeated with low attendance uses check-in template",
			fullName:         "Ahmed Ali",
			level:            2,
			outcome:          "repeated",
			attendedSessions: 2,
			gender:           "male",
			wantKey:          "repeated_low_attendance",
			wantLabel:        "Repeated - Low Attendance",
			wantContains:     "ابعتلنا صورة",
		},
		{
			name:             "repeated with partial attendance uses 40 percent template",
			fullName:         "Ahmed Ali",
			level:            4,
			outcome:          "repeated",
			attendedSessions: 3,
			gender:           "female",
			wantKey:          "repeated_partial_attendance",
			wantLabel:        "Repeated - Partial Attendance",
			wantContains:     "ابعتي المبلغ",
		},
		{
			name:             "female renewal copy adjusts broader agreement",
			fullName:         "Hager Ali",
			level:            2,
			outcome:          "promoted",
			attendedSessions: 1,
			gender:           "female",
			wantKey:          "promoted",
			wantLabel:        "Promoted",
			wantContains:     "إنتِ فعلاً بتتقدمي",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := buildRenewalPendingMessage(tc.fullName, tc.level, tc.outcome, tc.attendedSessions, tc.gender)
			if !ok {
				t.Fatalf("expected a message decision")
			}
			if got.Key != tc.wantKey {
				t.Fatalf("expected key %q, got %q", tc.wantKey, got.Key)
			}
			if got.Label != tc.wantLabel {
				t.Fatalf("expected label %q, got %q", tc.wantLabel, got.Label)
			}
			if got.Text == "" {
				t.Fatalf("expected non-empty message text")
			}
			if tc.wantContains != "" && !strings.Contains(got.Text, tc.wantContains) {
				t.Fatalf("expected message to contain %q, got %q", tc.wantContains, got.Text)
			}
			if tc.gender == "female" && tc.outcome == "promoted" {
				if !strings.Contains(got.Text, "وإحنا فخورين بيكي") {
					t.Fatalf("expected female renewal message to contain feminine pride phrase, got %q", got.Text)
				}
			}
		})
	}
}

func TestBuildSleepingLeadMessageAppliesFemaleCopy(t *testing.T) {
	got := buildSleepingLeadMessage("Sara Ali", 3, "female")
	if !strings.Contains(got, "مهتمة") {
		t.Fatalf("expected female sleeping message to contain feminine interest wording, got %q", got)
	}
	if !strings.Contains(got, "كلميني") {
		t.Fatalf("expected female sleeping message to contain feminine imperative, got %q", got)
	}
}

func TestBuildOfferSentFollowUpMessageAppliesFemaleCopy(t *testing.T) {
	got := buildOfferSentFollowUpMessage("Sara Ali", 3, "female")
	if !strings.Contains(got, "مناسبة ليكي") {
		t.Fatalf("expected female offer message to contain feminine phrase, got %q", got)
	}
	if !strings.Contains(got, "عايزة أجرب") {
		t.Fatalf("expected female offer message to contain feminine CTA, got %q", got)
	}
}

func TestPersonalizeStoredTemplateTextAppliesNameAndGender(t *testing.T) {
	got := personalizeStoredTemplateText("[الاسم]، [ابعت/ابعتي] صورة التحويل و[كلمني/كلميني]", "Mona Ahmed", "female")
	if !strings.Contains(got, "Mona، ابعتي صورة التحويل وكلميني") {
		t.Fatalf("expected personalized stored template, got %q", got)
	}
}
