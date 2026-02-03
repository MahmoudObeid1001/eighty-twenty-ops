-- Add closed_mentor_user_id to class_groups to track which mentor was assigned when the round was closed.
ALTER TABLE class_groups
  ADD COLUMN IF NOT EXISTS closed_mentor_user_id UUID REFERENCES users(id);

-- Add index for efficient filtering by closed_mentor_user_id
CREATE INDEX IF NOT EXISTS idx_class_groups_closed_mentor_user_id ON class_groups(closed_mentor_user_id);
