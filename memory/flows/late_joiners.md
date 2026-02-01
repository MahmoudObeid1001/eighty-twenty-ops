# Late Joiners Workflow

**Purpose**: Document how late-joining students are added to ongoing class rounds

**Status**: COMPLETED - Implemented 2026-02-01

**Evidence Sources**:
- `internal/models/repository.go` - GetStudentsInClassGroup, CompleteSession
- `migrations/017_create_attendance.sql`, `migrations/030_refine_attendance_and_followups.sql`
- `late_joiners_verified_proposal_v2.md` - Full proposal with evidence table
- `walkthrough.md` - Implementation walkthrough and verification steps
- `internal/handlers/api.go` - Late joiner API endpoints
- `internal/views/pre_enrolment_detail.html` - Late joiner UI and Undo buttons

**Stakeholder Confirmation**: All 7 open questions answered 2026-01-31

---

## KEY FINDING: Class Roster Mechanism

**CRITICAL**: This system does NOT have a dedicated enrollment or `class_students` table.

### How Students Are Linked to Classes

Students "belong" to a class via **implicit JOIN** when their records match class fields **AND** they have `status = 'in_classes'`:

```sql
-- Source: internal/models/repository.go:3797-3815 (UPDATED 2026-02-01)
SELECT l.id, l.full_name, l.phone, s.class_group_index
FROM leads l
INNER JOIN scheduling s ON s.lead_id = l.id
INNER JOIN placement_tests pt ON pt.lead_id = l.id
INNER JOIN class_groups cg ON (
    cg.level = pt.assigned_level                               -- Match 1: Level
    AND cg.class_days = s.class_days                          -- Match 2: Days
    AND cg.class_time = s.class_time::text                    -- Match 3: Time
    AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)  -- Match 4: Group number
)
WHERE cg.class_key = $1
  AND l.status = 'in_classes'  -- CRITICAL: Prevents roster leak
ORDER BY l.full_name
```

**Evidence**: `GetStudentsInClassGroup` function (line 3797)

### What This Means

- **To add a student to a class**: Update their `scheduling` record to match the target class's days/time/number, **AND** set `leads.status = 'in_classes'`, AND ensure `leads.sent_to_classes = true` to remove from pre-enrolment feed.
- **To remove a student**: Update their `scheduling` to different days/time/number, OR set `leads.sent_to_classes = false` / change status.
- **Roster Leak Fix (2026-02-01)**: Added `AND l.status = 'in_classes'` filter to prevent students from appearing in rosters before explicit enrollment.
- **Capacity Check Fix (2026-02-01)**: Late joiner capacity validation now also filters by `status = 'in_classes'` to count only enrolled students (line 3937).

---

## LOCKED Business Rules (SOURCE OF TRUTH)

### Late Joiner Eligibility (FINAL)

1. **Timing**: Can join only during **Session 1 or Session 2** (ABSOLUTE LIMIT)
   - If `current_session >= 3` → reject with error
   - Current session = COUNT(completed sessions) + 1
   
2. **Capacity**: Class must have **exactly 4 or 5 students** currently enrolled
   - After adding late joiner, total must not exceed 6
   - **NO overrides** allowed (strict enforcement)
   - **NOTE**: Capacity validation is NEW - current code has NO capacity checks

3. **Student Requirements**:
   - Must have `placement_tests` record (assigned_level)
   - Must have `scheduling` record
   - Status should be `ready_to_start` or `schedule_assigned` (flexible)
   - Must NOT already be `in_classes`
   - **NO payment gate** (can join without paid_full/deposit_paid)
   - **NO book delivery requirement**

4. **Class Requirements**:
   - `round_status` must be `'active'`
   - Class level must match student's `assigned_level`
   - Class days/time must be compatible with student needs

### Late Joiner Behavior (FINAL)

1. **Sessions Before Join**: Marked as **'N/A'** (uppercase, matches PRESENT/ABSENT casing)
2. **Missed Count**: N/A sessions excluded from missed session count (query only counts 'ABSENT')
3. **Badge**: Mentor/Mentor Head see "Late Join (SX)" badge next to student name
    - **Previous scheduling** (for undo capability)

### Awareness & Notifications (SSR & React)

1. **Recipients**: Mandatory unacknowledged notifications are created for:
    - The **assigned Mentor** for the specific class.
    - All **Mentor Heads**.
    - All **Student Success** users.
2. **Visibility Gate**: Notifications should **only appear** once the Mentor Head has started the round for the class
   (`class_groups.round_status = 'active'`) and the class is still `sent_to_mentor = true`.
3. **UI (Legacy SSR)**: High-visibility banner in `layout.html` for routes like `/student-success`.
4. **UI (Modern React)**: `LateJoinerBanner.tsx` component integrated into `AppLayout.tsx`.
    - **API**: Uses `/api/notifications/late-join` (GET and POST).
    - **Routing**: Uses role-based logic to navigate to the correct class page (`/mentor/class`, `/mentor-head/class`, or `/student-success/class`).
5. **List Filtering**: Late joiners are automatically marked `sent_to_classes = true` to ensure they are removed from the Pre-Enrolment list immediately.

### Undo Late Joiner (NEW FEATURE - LOCKED)

**Allowed only if**:
- Class current session still ≤ 2 (same join window)
- Lead has late_joiners record
- Performed by Ops Admin

**Actions**:
1. Delete late_joiners audit record
2. Remove N/A attendance rows for that class for that lead
3. Revert scheduling to previous snapshot (class_days, class_time, class_group_index)
4. Revert lead status to `ready_to_start` and set `sent_to_classes = false`
5. Delete unacknowledged `late_joiner_notifications` for that student/class.

**Use Case**: Admin added student to wrong class by mistake

---

## Current Session Number Calculation

**Definition**: Current session = COUNT(sessions WHERE status='completed') + 1

**Evidence**: `internal/models/repository.go:3780-3784`

```go
for _, s := range sessions {
    if s.Status == "completed" {
        completedCount++
    }
}
// Current session = completedCount + 1 (next upcoming session)
```

**NOT stored** anywhere - computed dynamically each time.

---

## Attendance Status Schema

**Current Schema** (as of migration 030):

```sql
-- Field: attendance.status (TEXT)
-- No CHECK constraint
-- Current values in use: 'PRESENT', 'ABSENT'
```

**Original Schema** (migration 017):
- Used Boolean `attended` field
- Migration 030 added TEXT `status` field

**For Late Joiners** (LOCKED):
- Use `'N/A'` status (uppercase to match PRESENT/ABSENT)
- **Must add CHECK constraint**:
  ```sql
  ALTER TABLE attendance ADD CONSTRAINT attendance_status_check
  CHECK (status IN ('PRESENT', 'ABSENT', 'N/A'));
  ```

**Missed Count Query** (`internal/models/repository.go:3733`):
```sql
WHERE cs.class_key = $1 AND a.status = 'ABSENT'
```
- Only counts 'ABSENT'
- N/A automatically excluded (no code change needed)

---

## Auto-Attendance Creation (CRITICAL FINDING - MUST FIX)

**Evidence**: `internal/models/repository.go:2649-2667`

When `CompleteSession` is called, it **automatically creates `status='PRESENT'` attendance for ALL students in the class who don't have an attendance record yet**.

### Implication for Late Joiners (LOCKED FIX REQUIRED)

If a late joiner is added **after a session is completed**, they will NOT get auto-PRESENT for that past session (good).

BUT: If sessions are completed **after** late joiner is added, the auto-create logic will conflict with N/A backfill.

**REQUIRED Solution** (LOCKED):
```sql
-- In CompleteSession, modify auto-create attendance query:
INSERT INTO attendance (id, session_id, lead_id, status, created_at, updated_at)
SELECT gen_random_uuid(), $1, s.lead_id, 'PRESENT', $2, $2
FROM scheduling s
INNER JOIN class_groups cg ON (...)
WHERE cg.class_key = $3
  AND NOT EXISTS (SELECT 1 FROM attendance WHERE session_id = $1 AND lead_id = s.lead_id)
  -- NEW: Exclude late joiners for sessions before they joined
  AND NOT EXISTS (
      SELECT 1 FROM late_joiners lj
      WHERE lj.lead_id = s.lead_id
        AND lj.class_key = cg.class_key
        AND $session_number < lj.joined_at_session_number
  );
```

**Alternative**: Check if attendance already exists with status='N/A' and skip those students.

---

## Capacity Enforcement (NEW BUSINESS RULE - LOCKED)

**Rule**: Strict enforcement - NO overrides

**Where to Apply**:
1. **Late Joiner Flow** (primary use case)
   - Check before allowing add
   - Must be 4 or 5 current students
   - After add, total ≤ 6

2. **Any Existing Add/Assign Student Workflows** (if they exist)
   - Audit current Ops workflows
   - Add capacity checks where students can be added to classes

**Implementation**:
```go
// Count students in class
students, _ := GetStudentsInClassGroup(classKey)
count := len(students)

// Validate
if count < 4 || count > 5 {
    return errors.New("Class must have 4-5 students for late joiner (current: " + strconv.Itoa(count) + ")")
}
```

---

## Class Number Sync Rule (LOCKED)

**Rule**: `scheduling.class_group_index` MUST be synced with `class_groups.class_number`

**Evidence**: Roster query uses:
```sql
COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
```

**For Late Joiners** (LOCKED):
1. When updating scheduling, use:
   ```sql
   class_group_index = COALESCE(:target_class_number, 1)
   ```
2. Always normalize NULL to 1
3. Write normalized value to ensure sync

### To Add Late Joiner

**Transaction Steps**:
1. **Validate** current session ≤ 2
2. **Validate** class capacity is 4 or 5
3. **Validate** lead not already in any class
4. **Create audit record** in `late_joiners` table
5. **Update `scheduling`** to match target class (days, time, class_group_index)
6. **Update `leads`** status to `'in_classes'` and `sent_to_classes = true`
7. **Backfill attendance** with `status='N/A'` for sessions 1 to (current-1)
8. **Insert notification records** for mentor, mentor heads, and student success

**Evidence**: See `late_joiners_verified_proposal_v2.md` section B.2

### To Display Late Joiner Badge

**Repository Change**:
- Extend `GetStudentsInClassGroup` to LEFT JOIN `late_joiners`
- Return `joined_at_session_number` in `ClassStudent` struct

**Frontend Logic**:
- If `joined_at_session_number` IS NOT NULL, show badge "Late Join (SX)"

---

## Database Schema (Proposed)

```sql
CREATE TABLE IF NOT EXISTS late_joiners (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    joined_at_session_number INT NOT NULL CHECK (joined_at_session_number IN (1, 2)),  -- Session 2 absolute limit
    reason TEXT NOT NULL,
    added_by_user_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    -- For Undo capability
    previous_class_days TEXT,
    previous_class_time TIME,
    previous_class_group_index INT,
    UNIQUE (lead_id)  -- Prevent duplicate late joins
);

CREATE INDEX idx_late_joiners_lead_id ON late_joiners(lead_id);
CREATE INDEX idx_late_joiners_class_key ON late_joiners(class_key);
```

---

## Open Questions (ALL ANSWERED - SEE LOCKED RULES)

~~1. **Session cutoff**: User said "Session 1 or Session 2" - is 2 the hard limit, or can it be extended to 3?~~  
**LOCKED**: Absolute limit = Session 2. Reject if current_session >= 3.

~~2. **Capacity override**: What if admin needs to add a 6th student to a class with 5 (emergency case)?~~  
**LOCKED**: NO override. Strict enforcement (4-5 current, max 6 after).

~~3. **Payment requirement**: Must late joiner have paid before joining? Or can join with pending payment?~~  
**LOCKED**: NO payment gate. Finance rules separate.

~~4. **Book delivery**: Must books be delivered first, or can they join without books?~~  
**LOCKED**: NO book delivery requirement.

~~5. **Remove late joiner**: If admin adds to wrong class, how to undo?~~  
**LOCKED**: Undo Late Join feature (see section above) - reverts scheduling, deletes audit, removes N/A attendance. Allowed only if current session ≤ 2.

~~6. **Capacity checks scope**: Add to other workflows?~~  
**LOCKED**: Yes - add to late joiner flow + any existing add/assign student workflows.

~~7. **Class number sync**: Is `class_number` always synced with `class_group_index`?~~  
**LOCKED**: Must enforce sync in code. Normalize NULL to 1. See Class Number Sync Rule above.

---

## Testing Checklist (UPDATED WITH LOCKED RULES)

- [ ] Add late joiner to class with 4 students (should succeed)
- [ ] Add late joiner to class with 5 students (should succeed, total = 6)
- [ ] Try to add to class with 3 students (should fail - capacity too low)
- [ ] Try to add to class with 6 students (should fail - capacity full)
- [ ] Try to add to class with 7students (should fail - over capacity)
- [ ] Verify N/A attendance uses uppercase 'N/A' not 'not_applicable'
- [ ] **Undo Late Join**: Add student, then undo within session 2 (should succeed)
- [ ] **Undo Late Join**: Try to undo after session 3 started (should fail)
- [ ] Verify undo reverts scheduling to previous snapshot
- [ ] Verify undo deletes N/A attendance rows
- [ ] **CompleteSession Fix**: Add late joiner, then complete session 1 - verify late joiner does NOT get auto-PRESENT for session 1

---

## Related Documentation

- **Full Proposal**: `late_joiners_verified_proposal_v2.md`
- **Class Lifecycle**: `class_lifecycle.md`
- **Attendance Rules**: `decisions/business_rules.md`
- **Database ERD**: `db/erd.md` (update after implementation)
