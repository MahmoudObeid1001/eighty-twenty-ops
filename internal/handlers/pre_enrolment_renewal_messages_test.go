package handlers

import "testing"

func TestBuildRenewalPendingMessage(t *testing.T) {
	tests := []struct {
		name             string
		fullName         string
		level            int
		outcome          string
		attendedSessions int
		wantKey          string
		wantLabel        string
	}{
		{
			name:             "promoted uses promoted template",
			fullName:         "Ahmed Ali",
			level:            3,
			outcome:          "promoted",
			attendedSessions: 1,
			wantKey:          "promoted",
			wantLabel:        "Promoted",
		},
		{
			name:             "repeated with low attendance uses check-in template",
			fullName:         "Ahmed Ali",
			level:            2,
			outcome:          "repeated",
			attendedSessions: 2,
			wantKey:          "repeated_low_attendance",
			wantLabel:        "Repeated - Low Attendance",
		},
		{
			name:             "repeated with partial attendance uses 40 percent template",
			fullName:         "Ahmed Ali",
			level:            4,
			outcome:          "repeated",
			attendedSessions: 3,
			wantKey:          "repeated_partial_attendance",
			wantLabel:        "Repeated - Partial Attendance",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := buildRenewalPendingMessage(tc.fullName, tc.level, tc.outcome, tc.attendedSessions)
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
		})
	}
}
