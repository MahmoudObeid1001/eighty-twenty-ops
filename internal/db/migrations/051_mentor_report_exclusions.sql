-- Soft-hide mentor rows from reports dashboard (without deleting business data).
CREATE TABLE IF NOT EXISTS mentor_report_exclusions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mentor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    round_status TEXT NOT NULL CHECK (round_status IN ('active', 'closed')),
    excluded_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (mentor_user_id, round_status)
);

CREATE INDEX IF NOT EXISTS idx_mentor_report_exclusions_mentor
    ON mentor_report_exclusions(mentor_user_id);

CREATE INDEX IF NOT EXISTS idx_mentor_report_exclusions_round_status
    ON mentor_report_exclusions(round_status);
