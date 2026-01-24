-- Finance tracking: transactions table
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_date DATE NOT NULL DEFAULT CURRENT_DATE,
    transaction_type TEXT NOT NULL CHECK (transaction_type IN ('IN', 'OUT')),
    category TEXT NOT NULL,
    amount INTEGER NOT NULL CHECK (amount > 0),
    payment_method TEXT CHECK (payment_method IN ('vodafone_cash', 'bank_transfer', 'paypal', 'other')),
    lead_id UUID REFERENCES leads(id) ON DELETE SET NULL,
    notes TEXT,
    source_key TEXT, -- Unique key to prevent duplicates (e.g., "placement_test_{lead_id}" or "course_{lead_id}")
    bundle_levels INTEGER, -- For course payments: 1, 2, 3, or 4
    levels_purchased INTEGER, -- For course payments: how many levels this payment represents
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Unique constraint on source_key to prevent duplicates
CREATE UNIQUE INDEX IF NOT EXISTS idx_transactions_source_key_unique ON transactions(source_key) WHERE source_key IS NOT NULL;

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_transactions_date ON transactions(transaction_date);
CREATE INDEX IF NOT EXISTS idx_transactions_type ON transactions(transaction_type);
CREATE INDEX IF NOT EXISTS idx_transactions_category ON transactions(category);
CREATE INDEX IF NOT EXISTS idx_transactions_lead_id ON transactions(lead_id);

-- Add credit tracking fields to leads table
ALTER TABLE leads ADD COLUMN IF NOT EXISTS levels_purchased_total INTEGER DEFAULT 0;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS levels_consumed INTEGER DEFAULT 0;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS bundle_type TEXT CHECK (bundle_type IN ('none', 'single', 'bundle2', 'bundle3', 'bundle4'));

-- Index for credits queries
CREATE INDEX IF NOT EXISTS idx_leads_levels_remaining ON leads(levels_purchased_total, levels_consumed) WHERE levels_purchased_total > 0;
