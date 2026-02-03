-- Migration 036: Make lead_id and session_number nullable for complaints
-- Since complaints use student_phone instead of lead_id and don't have sessions

-- Make lead_id nullable (needed for complaints which use student_phone)
ALTER TABLE followups ALTER COLUMN lead_id DROP NOT NULL;

-- Make session_number nullable (needed for complaints which don't have sessions)
ALTER TABLE followups ALTER COLUMN session_number DROP NOT NULL;

-- Drop constraint if exists (idempotent)
ALTER TABLE followups DROP CONSTRAINT IF EXISTS followups_type_fields_check;

-- Add check constraint: either it's an absence_escalation with lead_id+session_number, OR a complaint with student_phone
-- This ensures data integrity
ALTER TABLE followups ADD CONSTRAINT followups_type_fields_check CHECK (
    (type = 'absence_escalation' AND lead_id IS NOT NULL AND session_number IS NOT NULL) OR
    (type = 'complaint' AND student_phone IS NOT NULL)
);
