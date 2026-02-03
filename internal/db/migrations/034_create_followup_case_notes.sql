-- Migration 034: Create audit trail table for follow-up case actions
-- Tracks all notes, status changes, and resolutions for complaints and follow-ups

CREATE TABLE IF NOT EXISTS followup_case_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id UUID NOT NULL REFERENCES followups(id) ON DELETE CASCADE,
    note_text TEXT NOT NULL,
    note_type TEXT NOT NULL, -- 'comment', 'status_change', 'resolution', 'system'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by_user_id UUID REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_followup_case_notes_case_id ON followup_case_notes(case_id);
CREATE INDEX IF NOT EXISTS idx_followup_case_notes_created_at ON followup_case_notes(created_at);
