# Class Roster Mechanism - ANSWERED

**Date**: 2026-01-31  
**Status**: Verified with codebase

---

## Q: How are students linked to classes?

### Answer: **Implicit JOIN on scheduling + placement_tests - NO enrollment table**

**Evidence**: `internal/models/repository.go::GetStudentsInClassGroup` (lines 3789-3827)

### The Roster Query

```sql
SELECT l.id, l.full_name, l.phone, s.class_group_index
FROM leads l
INNER JOIN scheduling s ON s.lead_id = l.id
INNER JOIN placement_tests pt ON pt.lead_id = l.id
INNER JOIN class_groups cg ON (
    cg.level = pt.assigned_level
    AND cg.class_days = s.class_days
    AND cg.class_time = s.class_time::text
    AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
)
WHERE cg.class_key = $1
ORDER BY l.full_name
```

### What This Means

A student "belongs" to a class when **ALL** of these are true:

1. **Level Match**: `placement_tests.assigned_level` = `class_groups.level`
2. **Days Match**: `scheduling.class_days` = `class_groups.class_days`
3. **Time Match**: `scheduling.class_time` = `class_groups.class_time`
4. **Group Number Match**: `scheduling.class_group_index` = `class_groups.class_number` (with COALESCE fallback to 1)

**NO dedicated enrollment table exists** (`class_students`, `class_enrollments`, etc.)

---

## Q: How to add a student to a class?

### Answer: **Update their scheduling record to match the target class**

**Steps**:
1. Get target class details (level, class_days, class_time, class_number)
2. Ensure student's `placement_tests.assigned_level` matches the class level
3. **Update `scheduling` record**:
   ```sql
   UPDATE scheduling
   SET class_days = :target_class_days,
       class_time = :target_class_time::time,
       class_group_index = :target_class_number
   WHERE lead_id = :student_lead_id;
   ```
4. Update `leads.status` to `'in_classes'`

**Evidence**: This is how the late joiners workflow must work.

---

## Q: How to count students in a class?

### Answer: **Call GetStudentsInClassGroup and count the results (Mentor Head uses expanded roster pre-round)**

**Implementation**: `internal/models/repository.go:3721`

```go
students, _ := GetStudentsInClassGroup(r.ClassKey)
r.StudentCount = len(students)
```

**Mentor Head note**: For classes sent to mentor head (`sent_to_mentor = true`) while `round_status = not_started`, Mentor Head/Admin views use `GetStudentsForMentorHeadClass`, which includes `ready_to_start` students in addition to `in_classes`. Other roles remain `in_classes` only.

**NO database field** stores student count - it's computed dynamically each time.

---

## Q: Is there capacity validation?

### Answer: **NO - capacity validation does NOT exist in current code**

**Evidence**: Searched entire codebase for:
- "capacity", "max students", "class full"
- No handlers check student count before allowing scheduling changes
- No database constraints on max students per class

**Conclusion**: Late joiners workflow will be **FIRST** feature to implement capacity validation.

---

## Q: What if scheduling matches multiple classes?

### Answer: **Student would appear in multiple class rosters (edge case)**

**Evidence**: The JOIN logic has no UNIQUE constraint.

**Scenario**:
- Two classes: `L3-Sun/Wed-7:30PM-1` and `L3-Sun/Wed-7:30PM-2`
- Student has: `assigned_level=3`, `class_days=Sun/Wed`, `class_time=7:30PM`, `class_group_index=1`
- Student appears in **first class only** (because class_group_index matches class_number=1)

**Edge Case**:
- If `class_group_index` is NULL, `COALESCE(..., 1)` defaults to 1
- If `class_number` is NULL, also defaults to 1
- Both NULL → match → possible duplicate appearance

**Mitigation**: Ensure `class_group_index` is always set correctly when student is scheduled.

---

## Q: How does CompleteSession affect roster?

### Answer: **Auto-creates PRESENT attendance for all students in roster**

**Evidence**: `internal/models/repository.go:2649-2667`

When a session is completed, the function:
1. Uses the **same JOIN logic** as `GetStudentsInClassGroup`
2. Creates `status='PRESENT'` attendance for any student **who doesn't have an attendance record** for that session

```sql
INSERT INTO attendance (id, session_id, lead_id, status, created_at, updated_at)
SELECT gen_random_uuid(), $1, s.lead_id, 'PRESENT', $2, $2
FROM scheduling s
INNER JOIN class_groups cg ON (
    cg.level = (SELECT pt.assigned_level FROM placement_tests pt WHERE pt.lead_id = s.lead_id)
    AND cg.class_days = s.class_days
    AND cg.class_time = s.class_time::text
    AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
)
WHERE cg.class_key = $3
AND NOT EXISTS (
    SELECT 1 FROM attendance WHERE session_id = $1 AND lead_id = s.lead_id
);
```

**Implication**: If a student is added to a class (via scheduling update) **after** a session is completed, they will NOT get attendance for that past session.

---

## Related Findings

- **Session completion** also increments `leads.levels_consumed` for session 1 (lines 2627-2647)
- **Attendance backfill** uses `ON CONFLICT DO NOTHING` protection (implied by NOT EXISTS check)
- **Roster consistency** depends entirely on keeping `scheduling` and `placement_tests` records accurate

---

## Implications for Late Joiners

1. **To add late joiner**: Update `scheduling` to match target class
2. **N/A attendance**: Must create BEFORE session completion, or modify `CompleteSession` to skip late joiners for past sessions
3. **Capacity check**: Must query `GetStudentsInClassGroup`, count results, validate 4-5
4. **Badge display**: Must LEFT JOIN `late_joiners` in `GetStudentsInClassGroup` to fetch `joined_at_session_number`

---

## Testing Notes

To verify this mechanism works as documented:
1. Create a test class with known class_key
2. Create a test lead with scheduling that matches
3. Call `GetStudentsInClassGroup(class_key)` - student should appear
4. Change lead's scheduling to NOT match
5. Call `GetStudentsInClassGroup(class_key)` - student should disappear
6. Complete a session - verify auto-PRESENT created for student

**Status**: Can be tested via integration tests or manual DB queries.
