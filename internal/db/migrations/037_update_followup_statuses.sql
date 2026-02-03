-- Add NOT_REPLIED status to followups table
-- This enables the timeline: contacted → not_replied (1 day) → no_response (4 days)

-- First, standardize existing data to avoid constraint violations
UPDATE followups SET status = 'NOT_CONTACTED' WHERE status = 'none' OR status = 'NOT_CONTACTED';
UPDATE followups SET status = 'RESOLVED' WHERE status = 'replied' OR status = 'resolved' OR status = 'RESOLVED';
UPDATE followups SET status = 'CONTACTED' WHERE status = 'contacted' OR status = 'CONTACTED';
UPDATE followups SET status = 'NO_RESPONSE' WHERE status = 'no_response' OR status = 'NO_RESPONSE';

ALTER TABLE followups DROP CONSTRAINT IF EXISTS followups_status_check;

ALTER TABLE followups ADD CONSTRAINT followups_status_check 
  CHECK (status IN ('NOT_CONTACTED', 'CONTACTED', 'NOT_REPLIED', 'NO_RESPONSE', 'RESOLVED'));
