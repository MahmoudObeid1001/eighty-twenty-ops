ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS original_category TEXT,
    ADD COLUMN IF NOT EXISTS reconciled_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS reconciled_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_transactions_unidentified_transfer
    ON transactions (transaction_date DESC, created_at DESC)
    WHERE transaction_type = 'IN'
      AND category = 'unidentified_transfer'
      AND lead_id IS NULL;
