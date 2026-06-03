package tests

import (
	"database/sql"
	"fmt"
	"strings"
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

func TestMarkNewLeadContactedWithLeadNoteAppendsArabicAuditLine(t *testing.T) {
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
	admin := mustCreateUser(t, "admin", fmt.Sprintf("lead_note_contact_admin_%d@eightytwenty.test", nowSuffix))
	leadID := createLeadWithStatus(t, "Lead Note Contact", uniquePhone(nowSuffix, 603), "lead_created", admin.ID)
	mustExec(t, `UPDATE leads SET notes = 'ملاحظة قديمة' WHERE id = $1`, leadID)
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM student_notes WHERE lead_id = $1`, leadID)
		cleanupLead(t, leadID)
		mustExec(t, `DELETE FROM users WHERE id = $1`, admin.ID)
	})

	noteText := "قام admin Test User بالتواصل مع العميل وارسل رسالة لتحديد ميعاد تحديد المستوي."
	if err := models.MarkNewLeadContactedWithLeadNote(leadID, admin.ID, noteText); err != nil {
		t.Fatalf("failed to mark contacted with note: %v", err)
	}

	var notes sql.NullString
	var contactedAt sql.NullTime
	if err := db.DB.QueryRow(`
		SELECT notes, new_lead_contacted_at
		FROM leads
		WHERE id = $1
	`, leadID).Scan(&notes, &contactedAt); err != nil {
		t.Fatalf("failed to read lead note state: %v", err)
	}
	if !contactedAt.Valid {
		t.Fatalf("expected contact marker to be set")
	}
	if !notes.Valid || !strings.Contains(notes.String, "ملاحظة قديمة") || !strings.Contains(notes.String, noteText) {
		t.Fatalf("expected notes to preserve existing content and append audit line, got %q", notes.String)
	}
}
