ALTER TABLE mentor_evaluations
    ADD COLUMN IF NOT EXISTS kpi_session_quality_by_session JSONB NOT NULL DEFAULT '[0,0,0,0,0,0,0,0]'::jsonb;

UPDATE mentor_evaluations
SET kpi_session_quality_by_session = CASE
    WHEN COALESCE(kpi_session_quality, 0) > 0
        THEN jsonb_build_array(kpi_session_quality, 0, 0, 0, 0, 0, 0, 0)
    ELSE '[0,0,0,0,0,0,0,0]'::jsonb
END
WHERE kpi_session_quality_by_session IS NULL
   OR kpi_session_quality_by_session = '[]'::jsonb;
