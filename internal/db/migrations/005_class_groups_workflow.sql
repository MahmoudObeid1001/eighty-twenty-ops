-- Class groups workflow table for tracking mentor head workflow
CREATE TABLE IF NOT EXISTS class_groups (
    class_key TEXT PRIMARY KEY,
    level INTEGER NOT NULL,
    class_days TEXT NOT NULL,
    class_time TEXT NOT NULL,
    class_number INTEGER NOT NULL,
    sent_to_mentor BOOLEAN DEFAULT false,
    sent_at TIMESTAMP WITH TIME ZONE,
    returned_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index for efficient lookups
CREATE INDEX IF NOT EXISTS idx_class_groups_key ON class_groups(class_key);
