-- Migration 060: Reactivate manager role and enforce first-login password change flag

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN (
  'admin', 'moderator', 'mentor_head', 'mentor', 'hr', 'student_success', 'manager'
));
