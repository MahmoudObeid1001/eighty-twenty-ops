-- Add soft-deactivation support for users who leave the organization.
ALTER TABLE users
ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

UPDATE users
SET is_active = true
WHERE is_active IS NULL;
