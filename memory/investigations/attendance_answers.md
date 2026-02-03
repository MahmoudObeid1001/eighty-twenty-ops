# Attendance & Absences Investigations - ANSWERED

## Q: Automatic follow-up creation?

### Answer: **Manual creation - no automatic trigger**

**Evidence**: `internal/handlers/api.go:382-470` (MarkAttendance function)

The `MarkAttendance` handler does NOT create follow-ups automatically. It only:
1. Validates session and student IDs
2. Inserts/updates `attendance` table with status
3. Returns success

**Workflow**:
1. Mentor marks student as `absent_unexcused`
2. Attendance record created in DB
3. Student Success views **absence feed** (queries attendance table for absences)
4. Student Success **manually creates** follow-up via `POST /api/student-success/followups`

**Evidence for manual creation**: `cmd/server/main.go:317-326`
```go
mux.HandleFunc("/api/student-success/followups", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        middleware.RequireAnyRole([]string{"student_success", "mentor_head", "admin"}, cfg.SessionSecret)(apiHandler.CreateFollowUp)(w, r)
    }
}))
```

POST endpoint exists - follow-ups are created via API call, not database trigger.

---

## Q: Escalation thresholds/rules?

### Answer: **No automatic escalation rules found**

**Evidence**: 
- No database triggers on `attendance` or `followups` tables
- No CHECK constraints for escalation thresholds
- Handler code doesn't show escalation logic

**Inference**: Escalation is manual:
- Student Success views absence count per student
- Student Success decides when to escalate based on judgment
- No hardcoded "3 absences = escalate" rule visible in code

**TODO**: Could be UI-level guidance (e.g., frontend highlights students with 3+ absences) - would need to check frontend components

---

## Q: Attendance marking deadline?

### Answer: **No deadline enforced in code**

**Evidence**:
- `MarkAttendance` handler has no timestamp validation
- No CHECK constraint on `attendance.marked_at`
- No business logic blocking past session attendance

**Conclusion**: Mentors can mark attendance for any session at any time. No deadline enforced by system.

**Best practice** (not enforced): Likely expected to mark during/after session, but system doesn't prevent retroactive marking.

---

## Q: Grade tracking usage?

### Answer: **Grades table exists but usage unclear**

**Evidence**: `migrations/018_create_grades.sql`

**Table exists** with structure for storing grades, but:
- No API endpoints found for grades CRUD (checked `cmd/server/main.go`)
- No frontend components reference grades (checked React components)
- **TODO**: May be planned feature or legacy code

**Status**: Database schema ready, but feature not actively used in current implementation.

---

## Q: Can attendance for past sessions be modified?

### Answer: **Yes, no restrictions**

**Evidence**: `MarkAttendance` handler  

```go
if err := models.MarkAttendance(sessionID, leadID, req.Status, req.Notes, userID); err != nil {
    // ...
}
```

Calls `MarkAttendance` without date/time validation. The model likely uses `ON CONFLICT DO UPDATE` allowing updates.

**Conclusion**: Attendance can be marked/modified for any session (past or future) with no restrictions.
