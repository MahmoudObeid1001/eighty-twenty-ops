ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS new_lead_contacted_at TIMESTAMP WITH TIME ZONE;

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS new_lead_contacted_by_user_id TEXT;

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS new_lead_contacted_status TEXT;

CREATE INDEX IF NOT EXISTS idx_leads_new_lead_contacted_at
    ON leads (new_lead_contacted_at)
    WHERE new_lead_contacted_at IS NOT NULL;
