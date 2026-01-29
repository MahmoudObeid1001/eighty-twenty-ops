-- Add status column to community_officer_feedback table to track feedback receipt
ALTER TABLE community_officer_feedback 
ADD COLUMN IF NOT EXISTS status TEXT DEFAULT 'sent' CHECK (status IN ('sent', 'received', 'removed'));

-- Create index for efficient filtering
CREATE INDEX IF NOT EXISTS idx_co_feedback_status ON community_officer_feedback(status);

-- Set existing records to 'sent' status
UPDATE community_officer_feedback SET status = 'sent' WHERE status IS NULL;
