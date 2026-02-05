-- Create feedback_collected_uploads table for storing feedback files from students
CREATE TABLE IF NOT EXISTS feedback_collected_uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL,
    session_number INTEGER,
    file_name TEXT NOT NULL,
    file_url TEXT NOT NULL,
    mime_type TEXT,
    size_bytes INTEGER,
    note TEXT,
    uploaded_by_user_id UUID REFERENCES users(id),
    uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_feedback_collected_class_key ON feedback_collected_uploads(class_key);
CREATE INDEX IF NOT EXISTS idx_feedback_collected_lead_id ON feedback_collected_uploads(lead_id);
CREATE INDEX IF NOT EXISTS idx_feedback_collected_class_lead ON feedback_collected_uploads(class_key, lead_id);
