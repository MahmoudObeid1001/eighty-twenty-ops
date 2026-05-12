ALTER TABLE class_groups
    ADD COLUMN IF NOT EXISTS suggested_start_date DATE;

CREATE INDEX IF NOT EXISTS idx_class_groups_suggested_start_date
    ON class_groups (suggested_start_date)
    WHERE suggested_start_date IS NOT NULL;
