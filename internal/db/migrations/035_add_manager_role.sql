-- Migration 035: Add manager role for complaint removal management
-- Manager has exclusive permission to soft-delete complaint cases

-- Add manager role to users table constraint
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN (
    'admin', 'moderator', 'community_officer', 'mentor_head', 'mentor', 'hr', 'student_success', 'manager'
));

-- Create a seed manager user for testing (password: 'manager123')
-- Password hash generated with bcrypt cost 10
INSERT INTO users (email, password_hash, role)
VALUES ('manager@eightytwenty.test', '$2a$10$K239hQNDhAKzKyHlY/hLnutOe6IstcXbk53wdZyfo8UhDIq/a8nXa', 'manager')
ON CONFLICT (email) DO NOTHING;
