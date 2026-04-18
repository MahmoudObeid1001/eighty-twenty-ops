ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS mentor_head_return_reason TEXT;

ALTER TABLE leads
    DROP CONSTRAINT IF EXISTS leads_mentor_head_return_reason_check;

ALTER TABLE leads
    ADD CONSTRAINT leads_mentor_head_return_reason_check CHECK (
        mentor_head_return_reason IS NULL
        OR mentor_head_return_reason IN ('class_return', 'early_repeat_absence')
    );

CREATE INDEX IF NOT EXISTS idx_leads_mentor_head_return_reason
    ON leads (mentor_head_return_reason)
    WHERE mentor_head_return_reason IS NOT NULL;

