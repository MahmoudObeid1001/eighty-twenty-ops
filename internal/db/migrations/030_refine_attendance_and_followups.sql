-- Migration 030: Refine attendance and followups
-- 1. Update attendance table to support statuses (PRESENT, ABSENT, LATE)
ALTER TABLE attendance ADD COLUMN IF NOT EXISTS status TEXT DEFAULT 'PRESENT';
CREATE INDEX IF NOT EXISTS idx_attendance_status ON attendance(status);

-- 2. Refine followups table
ALTER TABLE followups ADD COLUMN IF NOT EXISTS resolved BOOLEAN DEFAULT false;
ALTER TABLE followups ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE followups ADD COLUMN IF NOT EXISTS resolved_by_user_id UUID REFERENCES users(id);

-- Update status constraint/comments
-- Existing statuses: NOT_CONTACTED, CONTACTED, RESOLVED
-- New desired statuses: none, contacted, replied, no_response
-- For now, we'll allow these via application logic or check constraints if needed.
-- Let's just update existing data to 'none' if it was 'NOT_CONTACTED' and handle the transition.
UPDATE followups SET status = 'none' WHERE status = 'NOT_CONTACTED';
UPDATE followups SET status = 'contacted' WHERE status = 'CONTACTED';
UPDATE followups SET status = 'contacted', resolved = true, resolved_at = updated_at WHERE status = 'RESOLVED';
