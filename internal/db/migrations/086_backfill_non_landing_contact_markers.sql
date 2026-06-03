WITH contact_events AS (
    SELECT
        ch.lead_id,
        ch.created_at AS contacted_at,
        ch.created_by_user_id::TEXT AS contacted_by_user_id
    FROM pre_enrolment_contact_history ch
    WHERE ch.source <> 'landing_page_contacted'
      AND ch.event_type IN ('contact_confirmed', 'message_ready')

    UNION ALL

    SELECT
        osf.lead_id,
        osf.sent_at AS contacted_at,
        osf.sent_by_user_id::TEXT AS contacted_by_user_id
    FROM offer_sent_follow_ups osf

    UNION ALL

    SELECT
        l.id AS lead_id,
        COALESCE(l.offer_sent_at, l.updated_at, l.created_at) AS contacted_at,
        NULL::TEXT AS contacted_by_user_id
    FROM leads l
    WHERE l.status = 'offer_sent'
       OR l.offer_sent_at IS NOT NULL

    UNION ALL

    SELECT
        slf.lead_id,
        slf.sent_at AS contacted_at,
        slf.sent_by_user_id::TEXT AS contacted_by_user_id
    FROM sleeping_lead_follow_ups slf

    UNION ALL

    SELECT
        rrf.lead_id,
        rrf.sent_at AS contacted_at,
        rrf.sent_by_user_id::TEXT AS contacted_by_user_id
    FROM refused_renewal_follow_ups rrf
),
latest_contact AS (
    SELECT DISTINCT ON (lead_id)
        lead_id,
        contacted_at,
        contacted_by_user_id
    FROM contact_events
    WHERE contacted_at IS NOT NULL
    ORDER BY lead_id, contacted_at DESC
)
UPDATE leads l
SET new_lead_contacted_at = lc.contacted_at,
    new_lead_contacted_by_user_id = lc.contacted_by_user_id,
    new_lead_contacted_status = l.status
FROM latest_contact lc
WHERE l.id = lc.lead_id
  AND l.new_lead_contacted_at IS NULL
  AND l.status <> 'in_classes'
  AND NOT (
      (l.source IS NOT NULL AND LOWER(TRIM(l.source)) = 'landing page')
      OR (l.notes IS NOT NULL AND (
          LOWER(l.notes) LIKE '%landing page signup%'
          OR LOWER(l.notes) LIKE '%تم التواصل عن طريق السيستم%'
      ))
  );
