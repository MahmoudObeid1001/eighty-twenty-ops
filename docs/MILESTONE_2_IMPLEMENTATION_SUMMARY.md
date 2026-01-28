# Milestone 2 (Active Classes) — Implementation Summary

**Date:** 2026-01-25  
**Status:** Backend Implementation Complete — Templates Pending

---

## 1) FILES CHANGED

### Database Migrations (9 new files)
- `internal/db/migrations/014_add_mentor_roles.sql` — Adds `mentor_head` and `mentor` roles
- `internal/db/migrations/015_add_follow_up_flag.sql` — Adds `high_priority_follow_up` to leads
- `internal/db/migrations/016_create_class_sessions.sql` — Creates `class_sessions` table (8 sessions per class)
- `internal/db/migrations/017_create_attendance.sql` — Creates `attendance` table
- `internal/db/migrations/018_create_grades.sql` — Creates `grades` table (A/B/C/F at session 8)
- `internal/db/migrations/019_create_student_notes.sql` — Creates `student_notes` table
- `internal/db/migrations/020_create_community_officer_feedback.sql` — Creates feedback table (sessions 4, 8)
- `internal/db/migrations/021_create_absence_follow_up_logs.sql` — Creates absence follow-up logs table
- `internal/db/migrations/022_create_mentor_assignments.sql` — Creates mentor assignments table

### Models (2 files modified)
- `internal/models/models.go` — Added structs: `ClassSession`, `Attendance`, `Grade`, `StudentNote`, `CommunityOfficerFeedback`, `AbsenceFollowUpLog`, `MentorAssignment`; Added `HighPriorityFollowUp` to `Lead`
- `internal/models/repository.go` — Added 20+ repository functions:
  - `CreateClassSessions()` — Creates 8 sessions when round starts
  - `GetClassSessions()` — Fetches sessions for a class
  - `CompleteSession()` — Marks session completed, sets `completed_at`, handles credit consumption
  - `CancelAndRescheduleSession()` — Reschedules same session_number (no session 9)
  - `MarkAttendance()`, `GetAttendanceForSession()` — Attendance management
  - `EnterGrade()`, `GetGrade()` — Grade management (session 8)
  - `AddStudentNote()`, `GetStudentNotes()` — Student notes
  - `GetRefundableAmount()` — Session-based refund calculation (uses `completed_at` markers)
  - `AssignMentorToClass()`, `CheckMentorScheduleConflict()` — Mentor assignment with conflict detection
  - `GetMentorAssignment()`, `GetMentorClasses()` — Mentor queries
  - `CloseRound()` — Computes outcomes, sets `high_priority_follow_up`
  - `SubmitFeedback()`, `GetPendingFeedback()` — Community officer feedback
  - `LogAbsenceFollowUp()`, `GetAbsenceFollowUpLogs()` — Absence follow-up
  - `GetUsersByRole()`, `GetUserByID()` — User queries
  - `GetClassGroupsSentToMentor()`, `GetStudentsInClassGroup()` — Class group queries
  - `GetClassGroupByKey()`, `GetSessionByID()` — Lookup functions
  - Updated `GetAllLeads()` to support `follow_up` filter
  - Updated `GetLeadByID()` to include `high_priority_follow_up`
  - Updated `StartRound()` to create 8 sessions per class
  - Updated `CreateRefund()` and `CreateCancelRefundIdempotent()` to use `GetRefundableAmount()`

### Handlers (5 new files, 2 modified)
- `internal/handlers/mentor_head.go` — NEW: Mentor Head dashboard, assign mentor, cancel/reschedule session, close round
- `internal/handlers/mentor.go` — NEW: Mentor dashboard, class detail, mark attendance, enter grade, add note, complete session
- `internal/handlers/community_officer.go` — NEW: Community officer dashboard, submit feedback, log follow-up
- `internal/handlers/role.go` — Modified: Added `IsMentorHead()`, `IsMentor()`, `IsCommunityOfficer()`
- `internal/handlers/pre_enrolment.go` — Modified: Added `follow_up` filter support
- `internal/handlers/templates.go` — Modified: Registered new templates

### Routes (1 file modified)
- `cmd/server/main.go` — Added routes:
  - `/mentor-head` (GET, POST `/assign`, POST `/session/cancel`, POST `/close-round`, POST `/{classKey}/return`)
  - `/mentor` (GET, GET `/class/{classKey}`, POST `/attendance`, POST `/grade`, POST `/note`, POST `/session/complete`)
  - `/community-officer` (GET, POST `/feedback`, POST `/follow-up`)

### Templates (4 new files needed — NOT YET CREATED)
- `internal/views/mentor_head.html` — Mentor Head dashboard
- `internal/views/mentor.html` — Mentor dashboard
- `internal/views/mentor_class_detail.html` — Mentor class detail (8 sessions, attendance, grades)
- `internal/views/community_officer.html` — Community Officer dashboard

### Modified Templates (3 files need updates — NOT YET DONE)
- `internal/views/pre_enrolment_list.html` — Add "High Priority Follow-Up" filter checkbox
- `internal/views/pre_enrolment_detail.html` — Add "Active Class" tab (conditional, hidden for moderators)
- `internal/views/classes.html` — Add "Active Classes" tab (optional)

---

## 2) HOW TO RUN MIGRATIONS

Migrations run automatically on server startup via `db.RunMigrations()` in `cmd/server/main.go`.

**Manual verification:**
```bash
# Check migration status
psql $DATABASE_URL -c "SELECT version, applied_at FROM schema_migrations ORDER BY applied_at;"

# Verify new tables exist
psql $DATABASE_URL -c "\dt class_sessions attendance grades student_notes community_officer_feedback absence_follow_up_logs mentor_assignments"

# Verify roles updated
psql $DATABASE_URL -c "SELECT DISTINCT role FROM users;"
```

**Expected output:**
- 9 new migration records (014-022)
- 7 new tables created
- `users.role` enum includes: `admin`, `moderator`, `community_officer`, `mentor_head`, `mentor`
- `leads.high_priority_follow_up` column exists

---

## 3) QUICK MANUAL TEST CHECKLIST

### Phase 1: Database & Basic Setup
- [ ] Run server → migrations apply successfully (check logs)
- [ ] Verify all 9 migrations applied (check `schema_migrations` table)
- [ ] Test users are seeded automatically: `mentor_head@eightytwenty.test` / `mentor_head123`, `mentor@eightytwenty.test` / `mentor123`, `community_officer@eightytwenty.test` / `community_officer123` (or env vars)

### Phase 2: Round Start & Session Creation
- [ ] **Test:** Admin starts round → 8 sessions created per class
  - Steps: Go to `/classes`, click "Start Round"
  - Verify: Check `class_sessions` table — 8 sessions per class with `status='scheduled'`
  - Verify: Students' status changed to `in_classes`

### Phase 3: Credit Consumption
- [ ] **Test:** Mentor completes session 1 → credit consumed
  - Steps: Login as mentor, go to class detail, mark session 1 complete
  - Verify: `leads.levels_consumed` incremented by 1 for all students in class
  - Verify: `class_sessions.completed_at` is set for session 1

### Phase 4: Refund Rules (Session-Based)
- [ ] **Test:** Refund 50% after session 1 completed
  - Steps: Complete session 1, try to cancel lead with refund
  - Verify: Cancel modal shows "Maximum refund: 50% of course paid"
  - Verify: Refund amount capped at 50% (validation blocks higher amounts)

- [ ] **Test:** Refund blocked after session 2 completed
  - Steps: Complete session 2, try to cancel lead with refund
  - Verify: Cancel modal shows "No refund available. Session 2 has been completed."
  - Verify: Refund amount = 0

- [ ] **Test:** Refund uses completion markers, not wall-clock time
  - Steps: Session 2 scheduled time has passed, but session not marked completed
  - Verify: Refund still available (50% if session 1 completed, 100% if not)

### Phase 5: Mentor Assignment & Conflicts
- [ ] **Test:** Assign mentor to class
  - Steps: Login as mentor_head, go to `/mentor-head`, assign mentor to class
  - Verify: `mentor_assignments` record created
  - Verify: Mentor sees class in `/mentor` dashboard

- [ ] **Test:** Mentor conflict blocked
  - Steps: Try to assign same mentor to overlapping sessions (same date/time)
  - Verify: Assignment rejected with error message
  - Verify: Different mentors can be assigned to same time slot

### Phase 6: Attendance & Grades
- [ ] **Test:** Mark attendance
  - Steps: Mentor marks attendance for students in a session
  - Verify: `attendance` records created/updated
  - Verify: Attendance visible in class detail view

- [ ] **Test:** Enter grade at session 8
  - Steps: Complete session 8, enter grade A/B/C/F for students
  - Verify: `grades` record created (session_number=8)
  - Verify: Grade visible in class detail

### Phase 7: Session Cancellation/Reschedule
- [ ] **Test:** Cancel and reschedule session
  - Steps: Mentor Head cancels session 3, reschedules to new date/time
  - Verify: Same `session_number=3` updated (not session 9 created)
  - Verify: Class still has exactly 8 sessions

### Phase 8: Round Close & Follow-Up
- [ ] **Test:** Close round computes outcomes
  - Steps: Mentor Head closes round
  - Verify: Outcomes computed (repeat if absences > 2 OR grade F, else promote)
  - Verify: Students with no remaining credits have `high_priority_follow_up = true`
  - Verify: Class returned to Operations (`sent_to_mentor = false`)

- [ ] **Test:** Follow-up filter
  - Steps: Go to `/pre-enrolment?follow_up=high_priority`
  - Verify: Only students with `high_priority_follow_up = true` shown

### Phase 9: Community Officer Workflow
- [ ] **Test:** Submit feedback at session 4
  - Steps: Complete session 4, login as community_officer, submit feedback
  - Verify: `community_officer_feedback` record created (session_number=4)

- [ ] **Test:** Submit feedback at session 8
  - Steps: Complete session 8, submit feedback
  - Verify: `community_officer_feedback` record created (session_number=8)

- [ ] **Test:** Log absence follow-up
  - Steps: Student absent, community officer logs follow-up action
  - Verify: `absence_follow_up_logs` record created with session link

### Phase 10: Access Control
- [ ] **Test:** Moderator cannot access mentor sections
  - Steps: Login as moderator, try `/mentor-head`, `/mentor`, `/community-officer`
  - Verify: 403 access-restricted page shown (same pattern as `/classes` and `/finance`)

- [ ] **Test:** Role-based access
  - Steps: Verify mentor_head can access `/mentor-head`, mentor can access `/mentor`, etc.
  - Verify: Admin can access all sections

### Phase 11: Student Notes Carry-Over
- [ ] **Test:** Notes persist across sessions
  - Steps: Mentor adds note in session 2, different mentor views class in session 3
  - Verify: Previous note visible in chronological order

### Phase 12: Repeat/Promote Logic
- [ ] **Test:** Missed 3 sessions forces repeat even with grade A
  - Steps: Student has 3 absences, grade A, close round
  - Verify: Outcome = REPEAT (absences > 2 rule takes precedence)

- [ ] **Test:** Grade F forces repeat regardless of attendance
  - Steps: Student has perfect attendance, grade F, close round
  - Verify: Outcome = REPEAT (grade F rule)

---

## 4) KNOWN LIMITATIONS / TODO

### Templates Not Yet Created
The following templates need to be created (backend is ready):
1. `mentor_head.html` — Dashboard with class list, assign mentor modal, cancel/reschedule modal
2. `mentor.html` — Dashboard with assigned classes list
3. `mentor_class_detail.html` — 8 sessions timeline, attendance entry, grade entry, notes
4. `community_officer.html` — Pending feedback list, absence follow-up list

### Template Updates Needed
1. `pre_enrolment_list.html` — Add follow-up filter checkbox
2. `pre_enrolment_detail.html` — Add "Active Class" tab (conditional on `status='in_classes'`)
3. `classes.html` — Optional: Add "Active Classes" tab

### Minor Issues
- `GetPendingFeedback()` returns a struct literal — may need refactoring for better type safety
- Some error handling could be more user-friendly (currently logs errors)
- Session date/time calculation uses `scheduling.start_date` and `scheduling.start_time` — if not set, defaults to today/07:30

---

## 5) NEXT STEPS

1. **Create Templates:** Implement the 4 new templates and update the 3 existing ones
2. **Test End-to-End:** Run through all acceptance scenarios
3. **UI Polish:** Ensure consistent styling with existing pages
4. **Error Handling:** Add user-friendly error messages
5. **Documentation:** Update README with new roles and features

---

**Implementation Status:** Backend complete, templates pending.
