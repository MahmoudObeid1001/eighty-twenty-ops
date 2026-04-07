CREATE TABLE IF NOT EXISTS daily_report_reads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    report_date DATE NOT NULL,
    read_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, report_date)
);

CREATE INDEX IF NOT EXISTS idx_daily_report_reads_user_id
    ON daily_report_reads(user_id);

CREATE TABLE IF NOT EXISTS complaint_reads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    complaint_id UUID NOT NULL REFERENCES followups(id) ON DELETE CASCADE,
    read_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, complaint_id)
);

CREATE INDEX IF NOT EXISTS idx_complaint_reads_user_id
    ON complaint_reads(user_id);

CREATE INDEX IF NOT EXISTS idx_complaint_reads_complaint_id
    ON complaint_reads(complaint_id);
