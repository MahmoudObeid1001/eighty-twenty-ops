-- Add ops archive flag for class_groups
ALTER TABLE class_groups
  ADD COLUMN IF NOT EXISTS hidden_in_ops BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS hidden_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN IF NOT EXISTS hidden_by UUID REFERENCES users(id);

CREATE INDEX IF NOT EXISTS idx_class_groups_hidden_in_ops ON class_groups(hidden_in_ops);
