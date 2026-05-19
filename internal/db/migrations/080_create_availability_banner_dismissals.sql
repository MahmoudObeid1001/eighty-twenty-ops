CREATE TABLE IF NOT EXISTS availability_banner_dismissals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    banner_key TEXT NOT NULL,
    banner_month DATE NOT NULL,
    dismissed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, banner_key, banner_month)
);

CREATE INDEX IF NOT EXISTS idx_availability_banner_dismissals_user_month
    ON availability_banner_dismissals (user_id, banner_month);
