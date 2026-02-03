-- Rename community_officer_feedback table to student_success_feedback
ALTER TABLE community_officer_feedback RENAME TO student_success_feedback;

-- Update role name in users table if any legacy references remain
UPDATE users SET role = 'student_success' WHERE role = 'community_officer';
