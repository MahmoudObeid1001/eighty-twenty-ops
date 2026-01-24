-- Add payment date and method to placement_tests table
ALTER TABLE placement_tests ADD COLUMN IF NOT EXISTS placement_test_payment_date DATE;
ALTER TABLE placement_tests ADD COLUMN IF NOT EXISTS placement_test_payment_method TEXT CHECK (placement_test_payment_method IN ('vodafone_cash', 'bank_transfer', 'paypal', 'other'));

-- Create lead_payments table for course payments (supports multiple payments per lead)
CREATE TABLE IF NOT EXISTS lead_payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    kind TEXT NOT NULL DEFAULT 'course' CHECK (kind = 'course'),
    amount INTEGER NOT NULL CHECK (amount > 0),
    payment_method TEXT NOT NULL CHECK (payment_method IN ('vodafone_cash', 'bank_transfer', 'paypal', 'other')),
    payment_date DATE NOT NULL,
    notes TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for efficient queries
CREATE INDEX IF NOT EXISTS idx_lead_payments_lead_id ON lead_payments(lead_id);
CREATE INDEX IF NOT EXISTS idx_lead_payments_date ON lead_payments(payment_date);

-- Update transactions table to add reference fields
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS ref_type TEXT;
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS ref_id TEXT;
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS ref_sub_type TEXT;
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS ref_key TEXT;

-- Update unique constraint to use ref_key instead of source_key
DROP INDEX IF EXISTS idx_transactions_source_key_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_transactions_ref_key_unique ON transactions(ref_key) WHERE ref_key IS NOT NULL;

-- Index for reference lookups
CREATE INDEX IF NOT EXISTS idx_transactions_ref_id ON transactions(ref_id) WHERE ref_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_transactions_ref_sub_type ON transactions(ref_sub_type) WHERE ref_sub_type IS NOT NULL;
