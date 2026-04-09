-- Migration 063: explicit class memberships, transfer audit, and ops queue reasons

CREATE TABLE IF NOT EXISTS class_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    joined_at_session_number INT NOT NULL CHECK (joined_at_session_number BETWEEN 1 AND 8),
    left_after_session_number INT CHECK (left_after_session_number BETWEEN 0 AND 8),
    join_reason TEXT NOT NULL DEFAULT 'round_start',
    leave_reason TEXT,
    added_by_user_id UUID REFERENCES users(id),
    removed_by_user_id UUID REFERENCES users(id),
    removed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CHECK (
        left_after_session_number IS NULL
        OR left_after_session_number >= joined_at_session_number - 1
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_class_memberships_unique_window
    ON class_memberships (lead_id, class_key, joined_at_session_number);

CREATE UNIQUE INDEX IF NOT EXISTS idx_class_memberships_one_active_per_lead
    ON class_memberships (lead_id)
    WHERE left_after_session_number IS NULL AND removed_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_class_memberships_class_key
    ON class_memberships (class_key);

CREATE INDEX IF NOT EXISTS idx_class_memberships_lead_id
    ON class_memberships (lead_id);

CREATE TABLE IF NOT EXISTS class_transfers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    source_class_key TEXT REFERENCES class_groups(class_key) ON DELETE SET NULL,
    target_class_key TEXT REFERENCES class_groups(class_key) ON DELETE SET NULL,
    source_membership_id UUID REFERENCES class_memberships(id) ON DELETE SET NULL,
    target_membership_id UUID REFERENCES class_memberships(id) ON DELETE SET NULL,
    source_exit_after_session_number INT CHECK (source_exit_after_session_number BETWEEN 0 AND 8),
    target_joined_at_session_number INT CHECK (target_joined_at_session_number BETWEEN 1 AND 8),
    reason TEXT NOT NULL CHECK (
        reason IN (
            'schedule_change',
            'promotion',
            'demotion',
            'refund_to_admin',
            'private_track_to_admin',
            'late_join',
            'other'
        )
    ),
    notes TEXT,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CHECK (source_class_key IS NOT NULL OR target_class_key IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_class_transfers_lead_id
    ON class_transfers (lead_id);

CREATE INDEX IF NOT EXISTS idx_class_transfers_source_class_key
    ON class_transfers (source_class_key);

CREATE INDEX IF NOT EXISTS idx_class_transfers_target_class_key
    ON class_transfers (target_class_key);

ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS ops_queue_reason TEXT;

ALTER TABLE leads
    DROP CONSTRAINT IF EXISTS leads_ops_queue_reason_check;

ALTER TABLE leads
    ADD CONSTRAINT leads_ops_queue_reason_check CHECK (
        ops_queue_reason IS NULL
        OR ops_queue_reason IN ('refund_review', 'private_track')
    );

CREATE INDEX IF NOT EXISTS idx_leads_ops_queue_reason
    ON leads (ops_queue_reason);
