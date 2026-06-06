package handlers

import (
	"database/sql"
	"testing"

	"eighty-twenty-ops/internal/models"
)

func TestHasNonLandingLeadContactMarkerRequiresCurrentStatusMatch(t *testing.T) {
	baseLead := &models.Lead{
		Status:                 "offer_sent",
		NewLeadContactedAt:     sql.NullTime{Valid: true},
		NewLeadContactedStatus: sql.NullString{String: "offer_sent", Valid: true},
	}

	if !hasNonLandingLeadContactMarker(baseLead) {
		t.Fatalf("expected marker to be active when contacted status matches current status")
	}

	paidLead := &models.Lead{
		Status:                 "paid_full",
		NewLeadContactedAt:     sql.NullTime{Valid: true},
		NewLeadContactedStatus: sql.NullString{String: "offer_sent", Valid: true},
	}

	if hasNonLandingLeadContactMarker(paidLead) {
		t.Fatalf("expected marker to be hidden after status changes away from contacted status")
	}
}

func TestCanSendPlacementResultOnlyBeforePaymentStatuses(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{status: "tested", want: true},
		{status: "offer_sent", want: true},
		{status: "paid_full", want: false},
		{status: "deposit_paid", want: false},
		{status: "waiting_for_round", want: false},
		{status: "lead_created", want: false},
	}

	for _, tc := range cases {
		lead := &models.Lead{Status: tc.status}
		if got := canSendPlacementResult(lead); got != tc.want {
			t.Fatalf("status %s: expected %v, got %v", tc.status, tc.want, got)
		}
	}
}
