CREATE TABLE IF NOT EXISTS payment_cycles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    bundle_levels INTEGER NOT NULL CHECK (bundle_levels >= 1 AND bundle_levels <= 4),
    final_price INTEGER NOT NULL CHECK (final_price > 0),
    consumed_baseline INTEGER NOT NULL DEFAULT 0 CHECK (consumed_baseline >= 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
    closed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_cycles_one_active_per_lead
    ON payment_cycles (lead_id)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_payment_cycles_lead_started
    ON payment_cycles (lead_id, started_at DESC);
