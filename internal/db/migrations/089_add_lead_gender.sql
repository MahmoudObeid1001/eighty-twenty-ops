ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS gender TEXT;

ALTER TABLE leads
    DROP CONSTRAINT IF EXISTS leads_gender_check;

ALTER TABLE leads
    ADD CONSTRAINT leads_gender_check
    CHECK (gender IN ('male', 'female') OR gender IS NULL);
