-- Prevent mentor double-booking: same mentor cannot be assigned to two classes
-- with the same class_days + class_time. Enforced via trigger.

CREATE OR REPLACE FUNCTION check_mentor_double_book()
RETURNS TRIGGER AS $$
DECLARE
  v_days TEXT;
  v_time TEXT;
  v_exists BOOLEAN;
BEGIN
  SELECT cg.class_days, cg.class_time::TEXT INTO v_days, v_time
  FROM class_groups cg WHERE cg.class_key = NEW.class_key;
  IF v_days IS NULL OR v_time IS NULL THEN
    RETURN NEW;
  END IF;

  SELECT EXISTS(
    SELECT 1
    FROM mentor_assignments ma
    INNER JOIN class_groups cg ON cg.class_key = ma.class_key
    WHERE ma.mentor_user_id = NEW.mentor_user_id
      AND ma.class_key != NEW.class_key
      AND cg.class_days = v_days
      AND cg.class_time::TEXT = v_time
  ) INTO v_exists;

  IF v_exists THEN
    RAISE EXCEPTION 'Mentor already assigned to another class at % %.', v_days, v_time;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_mentor_assignments_double_book ON mentor_assignments;
CREATE TRIGGER trg_mentor_assignments_double_book
  BEFORE INSERT OR UPDATE OF mentor_user_id, class_key ON mentor_assignments
  FOR EACH ROW EXECUTE FUNCTION check_mentor_double_book();
