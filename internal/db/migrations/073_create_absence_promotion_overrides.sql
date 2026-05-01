-- Mentor requests to promote a student despite the automatic absence repeat rule.
CREATE TABLE IF NOT EXISTS absence_promotion_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    requested_by_user_id UUID REFERENCES users(id),
    requested_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    reviewed_by_user_id UUID REFERENCES users(id),
    reviewed_at TIMESTAMP WITH TIME ZONE,
    review_note TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (lead_id, class_key)
);

CREATE INDEX IF NOT EXISTS idx_absence_promotion_overrides_class_key
    ON absence_promotion_overrides(class_key);

CREATE INDEX IF NOT EXISTS idx_absence_promotion_overrides_status
    ON absence_promotion_overrides(status);
