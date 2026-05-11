package tests

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

func TestReconcileUnidentifiedTransferToLeadReusesOriginalCashEntry(t *testing.T) {
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
	admin := mustCreateUser(t, "admin", fmt.Sprintf("reconcile_admin_%d@eightytwenty.test", nowSuffix))
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM users WHERE id = $1`, admin.ID)
	})

	leadID := createLeadWithStatus(t, "Unknown Transfer Match", uniquePhone(nowSuffix, 30), "offer_sent", admin.ID)
	t.Cleanup(func() { cleanupLead(t, leadID) })

	transferID := uuid.New()
	transferDate := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 5, 8, 13, 45, 0, 0, time.UTC)

	mustExec(t, `
		INSERT INTO transactions (
			id, transaction_date, transaction_type, category, amount, payment_method, notes, created_at, updated_at
		) VALUES ($1, $2::date, 'IN', 'unidentified_transfer', 1250, 'vodafone_cash', $3, $4, $4)
	`, transferID, transferDate.Format("2006-01-02"), "Unknown wallet transfer", createdAt)
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM transactions WHERE id = $1`, transferID)
	})

	payment, err := models.ReconcileUnidentifiedTransferToLead(transferID, leadID, "full_payment", "Lead sent receipt later", &admin.ID)
	if err != nil {
		t.Fatalf("ReconcileUnidentifiedTransferToLead failed: %v", err)
	}
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM lead_payments WHERE id = $1`, payment.ID)
	})

	if payment.Amount != 1250 {
		t.Fatalf("expected reconciled payment amount 1250, got %d", payment.Amount)
	}
	if payment.PaymentMethod != "vodafone_cash" {
		t.Fatalf("expected reconciled payment method vodafone_cash, got %s", payment.PaymentMethod)
	}
	if !payment.PaymentDate.Equal(transferDate) {
		t.Fatalf("expected payment date %s, got %s", transferDate.Format("2006-01-02"), payment.PaymentDate.Format("2006-01-02"))
	}
	if !payment.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected payment created_at %s, got %s", createdAt, payment.CreatedAt)
	}
	if !payment.Notes.Valid || payment.Notes.String != "Unknown wallet transfer\nReconciled note: Lead sent receipt later" {
		t.Fatalf("unexpected payment notes: %#v", payment.Notes)
	}

	var inTransactionCount int
	if err := mustQueryRow(t, `
		SELECT COUNT(*)
		FROM transactions
		WHERE transaction_type = 'IN'
		  AND lead_id = $1
	`, leadID).Scan(&inTransactionCount); err != nil {
		t.Fatalf("failed to count lead income transactions: %v", err)
	}
	if inTransactionCount != 1 {
		t.Fatalf("expected exactly one IN transaction for the lead after reconciliation, got %d", inTransactionCount)
	}

	var category string
	var amount int32
	var paymentMethod string
	var originalCategory sql.NullString
	var reconciledAt sql.NullTime
	var reconciledBy sql.NullString
	var refSubType sql.NullString
	var refKey sql.NullString
	var storedNotes sql.NullString
	if err := mustQueryRow(t, `
		SELECT category, amount, payment_method, original_category, reconciled_at, COALESCE(reconciled_by_user_id::text, ''), ref_sub_type, ref_key, notes
		FROM transactions
		WHERE id = $1
	`, transferID).Scan(&category, &amount, &paymentMethod, &originalCategory, &reconciledAt, &reconciledBy, &refSubType, &refKey, &storedNotes); err != nil {
		t.Fatalf("failed to reload reconciled transaction: %v", err)
	}
	if category != "course_payment" {
		t.Fatalf("expected transaction category course_payment after reconciliation, got %s", category)
	}
	if amount != 1250 {
		t.Fatalf("expected transaction amount to stay 1250, got %d", amount)
	}
	if paymentMethod != "vodafone_cash" {
		t.Fatalf("expected transaction payment method to stay vodafone_cash, got %s", paymentMethod)
	}
	if !originalCategory.Valid || originalCategory.String != "unidentified_transfer" {
		t.Fatalf("expected original_category unidentified_transfer, got %#v", originalCategory)
	}
	if !reconciledAt.Valid {
		t.Fatalf("expected reconciled_at to be populated")
	}
	if !reconciledBy.Valid || reconciledBy.String != admin.ID.String() {
		t.Fatalf("expected reconciled_by_user_id %s, got %#v", admin.ID.String(), reconciledBy)
	}
	if !refSubType.Valid || refSubType.String != "course_payment" {
		t.Fatalf("expected ref_sub_type course_payment, got %#v", refSubType)
	}
	expectedRefKey := fmt.Sprintf("lead:%s:course_payment:%s", leadID.String(), payment.ID.String())
	if !refKey.Valid || refKey.String != expectedRefKey {
		t.Fatalf("expected ref_key %s, got %#v", expectedRefKey, refKey)
	}
	if !storedNotes.Valid || storedNotes.String != payment.Notes.String {
		t.Fatalf("expected reconciled transaction notes to match payment notes, got %#v", storedNotes)
	}
}
