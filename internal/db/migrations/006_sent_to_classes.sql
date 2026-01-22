-- Add sent_to_classes field to leads table
-- This tracks whether a student has been manually sent to the classes board
ALTER TABLE leads ADD COLUMN IF NOT EXISTS sent_to_classes BOOLEAN DEFAULT false;

-- Index for efficient filtering
CREATE INDEX IF NOT EXISTS idx_leads_sent_to_classes ON leads(sent_to_classes) WHERE sent_to_classes = true;
