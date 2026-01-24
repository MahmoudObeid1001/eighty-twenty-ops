-- Ensure placement test payment fields exist (idempotent)
-- Source of truth: 008_finance_ledger_sync.sql (placement_tests table)

ALTER TABLE placement_tests
    ADD COLUMN IF NOT EXISTS placement_test_payment_date DATE;

ALTER TABLE placement_tests
    ADD COLUMN IF NOT EXISTS placement_test_payment_method TEXT;

