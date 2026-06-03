UPDATE leads AS l
SET
  new_lead_contacted_at = COALESCE(l.offer_sent_at, l.updated_at, l.created_at),
  new_lead_contacted_status = COALESCE(NULLIF(l.new_lead_contacted_status, ''), l.status)
WHERE l.new_lead_contacted_at IS NULL
  AND (l.status = 'offer_sent' OR l.offer_sent_at IS NOT NULL)
  AND l.status <> 'in_classes'
  AND NOT (
    (l.source IS NOT NULL AND LOWER(TRIM(l.source)) = 'landing page')
    OR (
      l.notes IS NOT NULL
      AND (
        LOWER(l.notes) LIKE '%landing page signup%'
        OR LOWER(l.notes) LIKE '%تم التواصل عن طريق السيستم%'
      )
    )
  );
