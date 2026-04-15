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
	if item.OfferFollowUpStep != 1 {
		t.Fatalf("expected offer_sent lead to use message 1 after 24h, got step %d", item.OfferFollowUpStep)
	}
	if item.NextAction != "Send Message 1" {
		t.Fatalf("expected offer_sent next action to be message 1, got %q", item.NextAction)
	}
}

func TestComputeLeadFlagsOfferSentWaitsFirst24Hours(t *testing.T) {
	now := time.Now()

	item := &LeadListItem{
		Lead: &Lead{
			Status:      "offer_sent",
			OfferSentAt: sql.NullTime{Time: now.Add(-12 * time.Hour), Valid: true},
			UpdatedAt:   now.Add(-12 * time.Hour),
			CreatedAt:   now.Add(-30 * 24 * time.Hour),
		},
		AmountPaid: sql.NullInt32{Valid: false},
		FinalPrice: sql.NullInt32{Valid: false},
	}

	ComputeLeadFlags(item)

	if item.FollowUpDue {
		t.Fatalf("expected no follow-up due before first 24 hours")
	}
	if item.OfferFollowUpStep != 0 {
		t.Fatalf("expected no message step before first 24 hours, got %d", item.OfferFollowUpStep)
	}
	if item.NextAction != "Await reply" {
		t.Fatalf("expected offer_sent next action to wait for reply, got %q", item.NextAction)
	}
}

func TestComputeLeadFlagsOfferSentEscalatesToMessage3AndColdReview(t *testing.T) {
	now := time.Now()

	message3 := &LeadListItem{
		Lead: &Lead{
			Status:      "offer_sent",
			OfferSentAt: sql.NullTime{Time: now.Add(-5 * 24 * time.Hour), Valid: true},
			UpdatedAt:   now.Add(-5 * 24 * time.Hour),
			CreatedAt:   now.Add(-30 * 24 * time.Hour),
		},
		AmountPaid: sql.NullInt32{Valid: false},
		FinalPrice: sql.NullInt32{Valid: false},
	}

	ComputeLeadFlags(message3)

	if message3.OfferFollowUpStep != 3 {
		t.Fatalf("expected message 3 at day 5, got step %d", message3.OfferFollowUpStep)
	}
	if message3.NextAction != "Send Message 3" {
		t.Fatalf("expected message 3 next action, got %q", message3.NextAction)
	}

	coldReview := &LeadListItem{
		Lead: &Lead{
			Status:      "offer_sent",
			OfferSentAt: sql.NullTime{Time: now.Add(-7 * 24 * time.Hour), Valid: true},
			UpdatedAt:   now.Add(-7 * 24 * time.Hour),
			CreatedAt:   now.Add(-30 * 24 * time.Hour),
		},
		AmountPaid: sql.NullInt32{Valid: false},
		FinalPrice: sql.NullInt32{Valid: false},
	}

	ComputeLeadFlags(coldReview)

	if !coldReview.FollowUpDue {
		t.Fatalf("expected cold review to remain actionable")
	}
	if coldReview.OfferFollowUpStep != 0 {
		t.Fatalf("expected no message step once cold review starts, got %d", coldReview.OfferFollowUpStep)
	}
	if coldReview.NextAction != "Review for cold lead" {
		t.Fatalf("expected cold-review next action, got %q", coldReview.NextAction)
	}
}

func TestComputeLeadFlagsOfferSentWaitsAfterLoggedMessage(t *testing.T) {
	now := time.Now()

	item := &LeadListItem{
		Lead: &Lead{
			Status:      "offer_sent",
			OfferSentAt: sql.NullTime{Time: now.Add(-3 * 24 * time.Hour), Valid: true},
			UpdatedAt:   now.Add(-3 * 24 * time.Hour),
			CreatedAt:   now.Add(-30 * 24 * time.Hour),
		},
		AmountPaid:            sql.NullInt32{Valid: false},
		FinalPrice:            sql.NullInt32{Valid: false},
		OfferFollowUpLastStep: 1,
		OfferFollowUpLastSent: sql.NullTime{Time: now.Add(-24 * time.Hour), Valid: true},
	}

	ComputeLeadFlags(item)

	if item.OfferFollowUpStep != 2 {
		t.Fatalf("expected next step to be message 2 after logging message 1, got %d", item.OfferFollowUpStep)
	}
	if item.OfferFollowUpDueNow {
		t.Fatalf("expected message 2 to stay dimmed until due")
	}
	if item.FollowUpDue {
		t.Fatalf("expected no actionable follow-up before next message becomes due")
	}
	if !item.OfferFollowUpDueAt.Valid {
		t.Fatalf("expected next offer follow-up due date to be set")
	}
}

func TestComputeLeadFlagsOfferSentReminderPausesSequence(t *testing.T) {
	now := time.Now()

	item := &LeadListItem{
		Lead: &Lead{
			Status:      "offer_sent",
			OfferSentAt: sql.NullTime{Time: now.Add(-5 * 24 * time.Hour), Valid: true},
			UpdatedAt:   now.Add(-5 * 24 * time.Hour),
			CreatedAt:   now.Add(-30 * 24 * time.Hour),
		},
		AmountPaid:        sql.NullInt32{Valid: false},
		FinalPrice:        sql.NullInt32{Valid: false},
		OfferReminderAt:   sql.NullTime{Time: now.Add(2 * 24 * time.Hour), Valid: true},
		OfferReminderNote: sql.NullString{String: "Will pay after salary", Valid: true},
	}

	ComputeLeadFlags(item)

	if item.OfferFollowUpStep != 0 {
		t.Fatalf("expected reminder to pause message sequence, got step %d", item.OfferFollowUpStep)
	}
	if item.FollowUpDue {
		t.Fatalf("expected future offer reminder to pause hot follow-up due state")
	}
	if item.NextAction != "Offer reminder set" {
		t.Fatalf("expected offer reminder next action, got %q", item.NextAction)
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
