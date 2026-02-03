# Class Lifecycle Investigations - ANSWERED

##  Q: When are 8 sessions created?

### Answer: **At round start (StartRound function)**

**Evidence**: `internal/models/repository.go:1428-1448`

```go
// Create 8 sessions for each class
for classKey, schedule := range classGroups {
    for i := 1; i <= 8; i++ {
        sessionDate := schedule.StartDate.AddDate(0, 0, (i-1)*7) // Weekly sessions
        // ... time parsing ...
        _, err = tx.Exec(`
            INSERT INTO class_sessions (id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time, status, created_at, updated_at)
            VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'scheduled', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
            ON CONFLICT (class_key, session_number) DO NOTHING
        `, classKey, i, sessionDate, schedule.StartTime, endTime)
    }
}
```

**When**: During `StartRound()` function execution  
**Context**: Admin clicks "Start Round" → moves READY/LOCKED classes to IN_CLASSES status → creates 8 weekly sessions for each class  
**Status**: Sessions start as `status = 'scheduled'`  
**Schedule**: Weekly sessions (7 days apart) starting from `scheduling.start_date`

---

## Q: Can a closed round be reopened?

### Answer: **No database constraint prevents it, BUT no UI/API exists to reopen**

**Evidence**:

1. **Database**: No CHECK constraint prevents `round_status` reversal `migrations/027_class_groups_round_status.sql`
2. **Handler code**: No API endpoint to reopen closed rounds (checked `internal/handlers/api.go`)
3. **Frontend**: No UI button/action to reopen (checked React components)

**Conclusion**: Technically possible in DB, but not exposed as a feature. To reopen, would need manual DB update.

---

## Q: Return to ops vs unassign mentor - what's the difference?

### Answer: **Different preconditions and use cases**

**Evidence**: `internal/handlers/api.go`

### UnassignMentor (`/api/mentor-head/unassign`)
**Code**: Lines 1170-1239  
**Precondition**: **Blocks if sessions exist** (round already started)
```go
sessions, err := models.GetClassSessions(req.ClassKey)
if len(sessions) > 0 {
    jsonError(w, http.StatusBadRequest, "Cannot unassign: round already started (sessions exist).")
    return
}
```
**Action**: Deletes `mentor_assignments` record  
**Use case**: Remove mentor BEFORE round starts (e.g., mentor requested change)  
**Access**: mentor_head only

### ReturnToOps (`/api/mentor-head/return-to-ops`)
**Code**: Lines 1134-1168  
**Precondition**: No session check - can return at any time  
**Action**: Calls `models.ReturnClassGroupFromMentor(classKey)` - sets `sent_to_mentor = false`  
**Use case**: Return class to Operations (e.g., too many/few students, scheduling issue)  
**Access**: mentor_head + admin

**Key difference**:
- **UnassignMentor**: Removes mentor assignment (pre-round start only)
- **ReturnToOps**: Returns entire class to ops workflow (anytime)

---

## Q: Double-booking prevention - is there a trigger?

### Answer: **Yes, database trigger exists**

**Evidence**: `migrations/026_mentor_double_book_trigger.sql`

```sql
CREATE OR REPLACE FUNCTION prevent_mentor_double_booking()
RETURNS TRIGGER AS $$
-- Trigger function prevents assigning mentor to overlapping class times
```

**Protection**: Prevents same mentor from being assigned to classes with overlapping schedules  
**Enforcement**: Database-level trigger on `mentor_assignments` table
