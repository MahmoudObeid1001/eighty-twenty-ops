package models

import (
	"database/sql"
	"testing"
	"time"
)

func TestComputeLeadFlagsOfferSentUsesOfferSentAt(t *testing.T) {
	now := time.Now()

	item := &LeadListItem{
		Lead: &Lead{
			Status:      "offer_sent",
			OfferSentAt: sql.NullTime{Time: now.Add(-24 * time.Hour), Valid: true},
			UpdatedAt:   now.Add(-24 * time.Hour),
			CreatedAt:   now.Add(-30 * 24 * time.Hour),
		},
		TestDate:   sql.NullTime{Time: now.Add(-20 * 24 * time.Hour), Valid: true},
		AmountPaid: sql.NullInt32{Valid: false},
		FinalPrice: sql.NullInt32{Valid: false},
	}

	ComputeLeadFlags(item)

	if !item.FollowUpDue {
		t.Fatalf("expected follow-up due for unpaid offer_sent lead")
	}
	if item.HotLevel != "HOT" {
		t.Fatalf("expected HOT level using offer_sent_at clock, got %q", item.HotLevel)
	}
}

func TestComputeLeadFlagsTestedUsesTestDate(t *testing.T) {
	now := time.Now()

	item := &LeadListItem{
		Lead: &Lead{
			Status:    "tested",
			UpdatedAt: now.Add(-24 * time.Hour),
			CreatedAt: now.Add(-30 * 24 * time.Hour),
		},
		TestDate:   sql.NullTime{Time: now.Add(-20 * 24 * time.Hour), Valid: true},
		AmountPaid: sql.NullInt32{Valid: false},
		FinalPrice: sql.NullInt32{Valid: false},
	}

	ComputeLeadFlags(item)

	if item.HotLevel != "COOL" {
		t.Fatalf("expected COOL level for tested lead with old test date, got %q", item.HotLevel)
	}
}
