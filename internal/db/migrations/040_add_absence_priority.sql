-- Add high_priority flag and reason to leads table
-- For tracking 3+ missed sessions (at-risk students)

ALTER TABLE leads ADD COLUMN IF NOT EXISTS high_priority BOOLEAN DEFAULT FALSE;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS high_priority_reason TEXT;
