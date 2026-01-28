-- Add high_priority_follow_up flag to leads table
-- This flag is set explicitly by mentor_head when closing a round for students with no remaining credits
ALTER TABLE leads ADD COLUMN IF NOT EXISTS high_priority_follow_up BOOLEAN DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_leads_high_priority_follow_up ON leads(high_priority_follow_up) WHERE high_priority_follow_up = true;
