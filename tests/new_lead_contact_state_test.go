package tests

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/models"
)

func TestNewLeadContactStatePersistsAcrossStatusChanges(t *testing.T) {
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	nowSuffix := time.Now().UnixNano()
	admin := mustCreateUser(t, "admin", fmt.Sprintf("new_lead_contact_admin_%d@eightytwenty.test", nowSuffix))
	leadID := createLeadWithStatus(t, "Fresh Contact Lead", uniquePhone(nowSuffix, 601), "lead_created", admin.ID)
	t.Cleanup(func() {
		cleanupLead(t, leadID)
		mustExec(t, `DELETE FROM users WHERE id = $1`, admin.ID)
	})

	if err := models.MarkNewLeadContacted(leadID, admin.ID); err != nil {
		t.Fatalf("failed to mark new lead contacted: %v", err)
	}

	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		t.Fatalf("failed to reload lead detail: %v", err)
	}
	if !detail.Lead.NewLeadContactedAt.Valid {
		t.Fatalf("expected new lead contacted timestamp to be set")
	}
	if !detail.Lead.NewLeadContactedStatus.Valid || detail.Lead.NewLeadContactedStatus.String != "lead_created" {
		t.Fatalf("expected new lead contacted status snapshot to be lead_created, got %+v", detail.Lead.NewLeadContactedStatus)
	}

	if err := models.UpdateLeadStatus(leadID, "test_booked"); err != nil {
		t.Fatalf("failed to move lead to test_booked: %v", err)
	}

	var contactedAt sql.NullTime
	var contactedBy sql.NullString
	var contactedStatus sql.NullString
	if err := db.DB.QueryRow(`
		SELECT new_lead_contacted_at, new_lead_contacted_by_user_id, new_lead_contacted_status
		FROM leads
		WHERE id = $1
	`, leadID).Scan(&contactedAt, &contactedBy, &contactedStatus); err != nil {
		t.Fatalf("failed to read raw lead contact state: %v", err)
	}
	if !contactedAt.Valid || !contactedBy.Valid || !contactedStatus.Valid {
		t.Fatalf("expected new lead contact state to persist after status change, got at=%v by=%v status=%v", contactedAt, contactedBy, contactedStatus)
	}
}

func TestOfferFollowUpMarksLeadAsContacted(t *testing.T) {
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	nowSuffix := time.Now().UnixNano()
	admin := mustCreateUser(t, "admin", fmt.Sprintf("offer_follow_contact_admin_%d@eightytwenty.test", nowSuffix))
	leadID := createLeadWithStatus(t, "Offer Contact Lead", uniquePhone(nowSuffix, 602), "offer_sent", admin.ID)
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM offer_sent_follow_ups WHERE lead_id = $1`, leadID)
		cleanupLead(t, leadID)
		mustExec(t, `DELETE FROM users WHERE id = $1`, admin.ID)
	})

	if err := models.RecordOfferSentFollowUp(leadID, 1, admin.ID); err != nil {
		t.Fatalf("failed to record offer follow-up: %v", err)
	}

	var contactedAt sql.NullTime
	var contactedBy sql.NullString
	var contactedStatus sql.NullString
	if err := db.DB.QueryRow(`
		SELECT new_lead_contacted_at, new_lead_contacted_by_user_id, new_lead_contacted_status
		FROM leads
		WHERE id = $1
	`, leadID).Scan(&contactedAt, &contactedBy, &contactedStatus); err != nil {
		t.Fatalf("failed to read raw lead contact state after offer follow-up: %v", err)
	}
	if !contactedAt.Valid || !contactedBy.Valid || !contactedStatus.Valid || contactedStatus.String != "offer_sent" {
		t.Fatalf("expected offer follow-up to mark contacted with offer_sent snapshot, got at=%v by=%v status=%v", contactedAt, contactedBy, contactedStatus)
	}
}
