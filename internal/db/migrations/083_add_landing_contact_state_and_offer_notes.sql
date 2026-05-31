ALTER TABLE offers
    ADD COLUMN IF NOT EXISTS follow_up_notes TEXT;

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS landing_page_contacted_at TIMESTAMP WITH TIME ZONE;

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS landing_page_contacted_by_user_id TEXT;

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS landing_page_contacted_status TEXT;

CREATE INDEX IF NOT EXISTS idx_leads_landing_page_contacted_at
    ON leads (landing_page_contacted_at)
    WHERE landing_page_contacted_at IS NOT NULL;
