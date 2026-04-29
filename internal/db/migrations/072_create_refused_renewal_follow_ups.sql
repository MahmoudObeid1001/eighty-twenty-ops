CREATE TABLE IF NOT EXISTS refused_renewal_message_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    refusal_reason TEXT NOT NULL CHECK (refusal_reason IN ('time_pressure', 'financial', 'not_satisfied', 'other')),
    sequence_step INTEGER NOT NULL DEFAULT 1 CHECK (sequence_step BETWEEN 1 AND 3),
    template_key TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (refusal_reason, sequence_step, template_key)
);

CREATE INDEX IF NOT EXISTS idx_refused_renewal_message_templates_reason
    ON refused_renewal_message_templates(refusal_reason, sequence_step);

CREATE TABLE IF NOT EXISTS refused_renewal_follow_ups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    message_number INTEGER NOT NULL CHECK (message_number BETWEEN 1 AND 3),
    template_id UUID REFERENCES refused_renewal_message_templates(id) ON DELETE SET NULL,
    message_text TEXT NOT NULL,
    sent_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_refused_renewal_follow_ups_lead_id
    ON refused_renewal_follow_ups(lead_id, sent_at DESC);

CREATE TABLE IF NOT EXISTS pre_enrolment_contact_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    channel TEXT NOT NULL DEFAULT 'whatsapp',
    event_type TEXT NOT NULL,
    source TEXT NOT NULL,
    template_key TEXT,
    message_text TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pre_enrolment_contact_history_lead_id
    ON pre_enrolment_contact_history(lead_id, created_at DESC);

CREATE TABLE IF NOT EXISTS global_banner_dismissals (
    banner_key TEXT NOT NULL,
    banner_date DATE NOT NULL,
    dismissed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    dismissed_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (banner_key, banner_date)
);

ALTER TABLE renewal_refusals
    ALTER COLUMN reason DROP DEFAULT;

INSERT INTO refused_renewal_message_templates (refusal_reason, sequence_step, template_key, title, body)
VALUES
    (
        'time_pressure',
        1,
        'message_c',
        'Message C — The Short Check-in',
        E'مساء الخير ي استاذ [الاسم]، ؟ 😊\nبس بتواصل أتأكد إن كل حاجة تمام\nلو الدنيا تمام وحابب تكمل، أنا هنا وهنرتب معاك في دقيقتين ✅'
    ),
    ('financial', 1, 'message_c', 'Message C — The Short Check-in', ''),
    ('not_satisfied', 1, 'message_c', 'Message C — The Short Check-in', ''),
    ('other', 1, 'message_c', 'Message C — The Short Check-in', '')
ON CONFLICT (refusal_reason, sequence_step, template_key) DO UPDATE
SET title = EXCLUDED.title,
    body = EXCLUDED.body,
    is_active = TRUE,
    updated_at = CURRENT_TIMESTAMP;
