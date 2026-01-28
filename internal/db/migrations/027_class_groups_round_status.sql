-- Add round_status and round lifecycle columns to class_groups.
-- round_status: not_started | active | closed. Student Success sees only active.

ALTER TABLE class_groups
  ADD COLUMN IF NOT EXISTS round_status TEXT NOT NULL DEFAULT 'not_started'
    CHECK (round_status IN ('not_started', 'active', 'closed')),
  ADD COLUMN IF NOT EXISTS round_started_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN IF NOT EXISTS round_started_by UUID REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS round_closed_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN IF NOT EXISTS round_closed_by UUID REFERENCES users(id);

CREATE INDEX IF NOT EXISTS idx_class_groups_round_status ON class_groups(round_status);
