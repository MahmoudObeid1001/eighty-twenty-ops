ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS landing_learning_goal TEXT,
    ADD COLUMN IF NOT EXISTS landing_english_level TEXT;
