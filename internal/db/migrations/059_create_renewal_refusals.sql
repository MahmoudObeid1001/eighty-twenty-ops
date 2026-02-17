-- Track returning students who refuse to renew, for later retargeting/reporting.
CREATE TABLE IF NOT EXISTS renewal_refusals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    refused_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    refused_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    reason TEXT NOT NULL DEFAULT 'refused_renewal',
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_renewal_refusals_lead_id ON renewal_refusals(lead_id);
CREATE INDEX IF NOT EXISTS idx_renewal_refusals_refused_at ON renewal_refusals(refused_at);
