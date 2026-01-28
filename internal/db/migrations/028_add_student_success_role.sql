-- Add student_success role for Student Success (Community) workflow
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN (
    'admin', 'moderator', 'community_officer', 'mentor_head', 'mentor', 'hr', 'student_success'
));
