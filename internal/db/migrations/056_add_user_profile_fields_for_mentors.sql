-- Add profile fields on users and enforce required mentor identity.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS full_name TEXT,
    ADD COLUMN IF NOT EXISTS phone TEXT;

-- Backfill mentor names from email local part when missing.
UPDATE users
SET full_name = INITCAP(REPLACE(REPLACE(SPLIT_PART(email, '@', 1), '.', ' '), '_', ' '))
WHERE role = 'mentor'
  AND (full_name IS NULL OR BTRIM(full_name) = '');

-- Backfill mentor phones with generated values when missing.
WITH numbered AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY created_at, id) AS rn
    FROM users
    WHERE role = 'mentor'
      AND (phone IS NULL OR BTRIM(phone) = '')
)
UPDATE users u
SET phone = '010' || LPAD((10000000 + numbered.rn)::text, 8, '0')
FROM numbered
WHERE u.id = numbered.id;

-- Enforce required profile values for mentor rows only.
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_mentor_profile_required;

ALTER TABLE users
    ADD CONSTRAINT users_mentor_profile_required
    CHECK (
        role <> 'mentor'
        OR (
            full_name IS NOT NULL
            AND BTRIM(full_name) <> ''
            AND phone IS NOT NULL
            AND BTRIM(phone) <> ''
        )
    );
