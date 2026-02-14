CREATE TABLE IF NOT EXISTS mentor_testimonials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mentor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    testimonial_text TEXT NOT NULL,
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mentor_testimonials_mentor_id
    ON mentor_testimonials(mentor_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_mentor_testimonials_class_key
    ON mentor_testimonials(class_key);
