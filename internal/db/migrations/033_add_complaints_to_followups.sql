-- Migration 033: Add complaints support to followups table
-- Extends followups to support both absence escalations and complaint cases

-- Add type field to distinguish complaints from absence escalations
ALTER TABLE followups ADD COLUMN IF NOT EXISTS type TEXT DEFAULT 'absence_escalation';

-- Add complaint-specific fields
ALTER TABLE followups ADD COLUMN IF NOT EXISTS category TEXT;
ALTER TABLE followups ADD COLUMN IF NOT EXISTS urgency TEXT;
ALTER TABLE followups ADD COLUMN IF NOT EXISTS student_phone TEXT;
ALTER TABLE followups ADD COLUMN IF NOT EXISTS complaint_text TEXT;

-- Add soft delete fields (Manager-only)
ALTER TABLE followups ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE followups ADD COLUMN IF NOT EXISTS deleted_by_user_id UUID REFERENCES users(id);
ALTER TABLE followups ADD COLUMN IF NOT EXISTS delete_reason TEXT;

-- Add indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_followups_type_status ON followups(type, status);
CREATE INDEX IF NOT EXISTS idx_followups_deleted_at ON followups(deleted_at);
CREATE INDEX IF NOT EXISTS idx_followups_student_phone ON followups(student_phone);

-- Update existing records to have type='absence_escalation'
UPDATE followups SET type = 'absence_escalation' WHERE type IS NULL;

-- Make type non-nullable now
ALTER TABLE followups ALTER COLUMN type SET NOT NULL;
