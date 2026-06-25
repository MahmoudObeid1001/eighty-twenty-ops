ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS landing_learning_goal TEXT,
    ADD COLUMN IF NOT EXISTS landing_english_level TEXT,
    ADD COLUMN IF NOT EXISTS landing_source TEXT,
    ADD COLUMN IF NOT EXISTS current_job TEXT,
    ADD COLUMN IF NOT EXISTS current_level TEXT,
    ADD COLUMN IF NOT EXISTS english_need TEXT,
    ADD COLUMN IF NOT EXISTS selected_package TEXT;
