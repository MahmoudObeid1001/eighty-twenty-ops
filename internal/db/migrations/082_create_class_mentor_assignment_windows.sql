-- Track mentor ownership by class session window so mid-round shifts preserve attribution.
CREATE TABLE IF NOT EXISTS class_mentor_assignment_windows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    mentor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    effective_from_session INTEGER NOT NULL CHECK (effective_from_session BETWEEN 1 AND 8),
    effective_to_session INTEGER CHECK (effective_to_session BETWEEN 0 AND 8),
    assigned_by_user_id UUID REFERENCES users(id),
    ended_by_user_id UUID REFERENCES users(id),
    reason TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CHECK (effective_to_session IS NULL OR effective_to_session >= effective_from_session)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_class_mentor_assignment_windows_one_open
    ON class_mentor_assignment_windows (class_key)
    WHERE effective_to_session IS NULL;

CREATE INDEX IF NOT EXISTS idx_class_mentor_assignment_windows_class_key
    ON class_mentor_assignment_windows (class_key);

CREATE INDEX IF NOT EXISTS idx_class_mentor_assignment_windows_mentor_user_id
    ON class_mentor_assignment_windows (mentor_user_id);

CREATE INDEX IF NOT EXISTS idx_class_mentor_assignment_windows_session_lookup
    ON class_mentor_assignment_windows (class_key, effective_from_session, effective_to_session);

INSERT INTO class_mentor_assignment_windows (
    class_key,
    mentor_user_id,
    effective_from_session,
    effective_to_session,
    assigned_by_user_id,
    reason,
    created_at,
    updated_at
)
SELECT
    ma.class_key,
    ma.mentor_user_id,
    1,
    NULL,
    ma.created_by_user_id,
    'Initial assignment history backfill',
    ma.assigned_at,
    NOW()
FROM mentor_assignments ma
WHERE NOT EXISTS (
    SELECT 1
    FROM class_mentor_assignment_windows w
    WHERE w.class_key = ma.class_key
);
