# Business Rules

**Purpose**: Document rules that are **actually implemented** in the code (evidence-based).

**Evidence Sources**: Migrations, handlers, middleware

---

## Authentication & Authorization

### Rule: Role-based access control
**Implementation**: Middleware checks user role against allowed roles per route  
**Mechanism**: `RequireAuth` + `RequireAnyRole([]string{...})`  
**Evidence**: `internal/middleware/auth.go`, `cmd/server/main.go` (route definitions)

### Rule: Session-based authentication
**Implementation**: Cookie-based sessions with HMAC signing  
**Cookie name**: `session`  
**Evidence**: `internal/middleware/auth.go` - session validation logic

### Rule: 7 distinct roles
**Roles**: admin, moderator, mentor_head, mentor, community_officer, hr, student_success  
**Constraint**: CHECK constraint on `users.role` field  
**Evidence**: `migrations/001_init.sql`, `migrations/014_add_mentor_roles.sql`, `migrations/023_add_hr_role.sql`, `migrations/028_add_student_success_role.sql`

---

## Class Lifecycle

### Rule: One mentor per class
**Constraint**: `UNIQUE (class_key)` on `mentor_assignments` table  
**Effect**: Cannot assign multiple mentors to same class  
**Evidence**: `migrations/022_create_mentor_assignments.sql:9`

### Rule: Round status progression
**States**: not_started → active → closed  
**Constraint**: CHECK constraint on `class_groups.round_status`  
**Evidence**: `migrations/027_class_groups_round_status.sql`

### Rule: Reopen closed round only if fewer than 8 sessions completed
**Who**: Mentor Head/Admin  
**Rule**: Closed classes can be reopened only when completed sessions < 8.  
**Evidence**: `internal/handlers/api.go` (`ReopenRound`), `internal/models/repository.go` (`ReopenClosedRound`), `cmd/server/main.go` (`/api/mentor-head/reopen-round`)

### Rule: 8 sessions per class
**Implementation**: Each class has 8 `class_sessions` records (session_number 1-8)  
**Evidence**: `migrations/016_create_class_sessions.sql`  
**Created**: When `StartClassRound` is called (`internal/models/repository.go:3829`)

### Rule: Class roster computed via implicit JOIN with status filter
**Implementation**: NO dedicated enrollment table. Students belong to a class when their `scheduling` + `placement_tests` match the class fields (level, days, time, number) **AND** `leads.status = 'in_classes'`  
**Evidence**: `internal/models/repository.go:3797-3815` (`GetStudentsInClassGroup`)  
**Critical**: The `status = 'in_classes'` filter prevents leads from appearing in rosters before explicit enrollment  
**See**: `memory/investigations/roster_mechanism_answers.md`

### Rule: Mentor Head pre-round roster visibility (sent classes)
**Implementation**: Mentor Head/Admin can see `ready_to_start` students for classes that are `sent_to_mentor` and still `round_status = not_started`. Other roles remain `in_classes` only.  
**Evidence**: `internal/models/repository.go` (`GetStudentsForMentorHeadClass`), `internal/handlers/mentor_head.go`, `internal/handlers/api.go` (Mentor Head dashboard + class workspace).

### Rule: sent_to_mentor flag
**Purpose**: Tracks if class has been sent from ops to mentor head  
**Values**: true (sent to mentor head), false (with ops)  
**Evidence**: `migrations/005_class_groups_workflow.sql:8`

### Rule: Class metadata stored in class_key
**Format**: `{level}-{days}-{time}-{number}` (e.g., "3-TuTh-5:00PM-1")  
**Evidence**: Database uses class_key as TEXT primary key in `class_groups`

---

## Pre-Enrolment Pipeline

### Rule: Lead status progression
**Pipeline**: lead_created → test_booked → tested → offer_sent → booking_confirmed → paid_full/deposit_paid → waiting_for_round → schedule_assigned → ready_to_start → in_classes  
**Constraint**: CHECK constraint on `leads.status`  
**Evidence**: `migrations/001_init.sql:19-22`, `migrations/004_classes_board.sql:3-6`

### Rule: Waiting list returns lead to pre-enrolment feed
**Scope**: Any action that sets `status = waiting_for_round`  
**Behavior**: Clears `sent_to_classes` so the lead appears in Pre-Enrolment and is removed from Classes board.  
**Evidence**: `internal/models/repository.go` (`UpdateLeadStatus`).

### Rule: Send-to-Classes does not join active rounds
**Scope**: Pre-Enrolment "Send to Classes" action  
**Behavior**: Assigns the lead to a non-started class group for the same level/days/time (or opens a new group if all are full).  
**Active rounds**: Joining a running class requires the Late Joiner flow.  
**Evidence**: `internal/models/repository.go` (`SendLeadToClasses`, `AssignClassGroup`, `MoveStudentBetweenGroups`), `internal/views/pre_enrolment_detail.html` note.

### Rule: Cancel lead requires refund modal when payments exist
**Scope**: Pre-Enrolment cancellations  
**Behavior**: Direct delete is disabled; cancellation must go through the cancel flow and show refund modal if course payments exist.  
**Evidence**: `internal/handlers/pre_enrolment.go` (cancel action), `internal/views/pre_enrolment_detail.html` cancel modal; list action routes to `?action=cancel`.

### Rule: One-to-one relationships for lead data
**Tables**: placement_tests, offers, bookings, payments, scheduling, shipping all have UNIQUE `lead_id`  
**Effect**: One placement test, one offer, one booking, etc. per lead  
**Evidence**: `migrations/001_init.sql` - UNIQUE constraints on lead_id

### Rule: Assigned level range  
**Initial**: 1-4  
**Updated**: 1-8  
**Constraint**: CHECK on `placement_tests.assigned_level`  
**Evidence**: `migrations/003_assigned_level_1_to_8.sql`

---

## Attendance & Absence

### Rule: Three attendance statuses
**Values**: `present`, `absent_excused`, `absent_unexcused`  
**Constraint**: CHECK constraint on `attendance.status`  
**Evidence**: `migrations/017_create_attendance.sql`

### Rule: Unique absence follow-up per session
**Constraint**: `UNIQUE (class_key, lead_id, session_number)` on `followups` table  
**Effect**: Cannot create duplicate follow-ups for same absence  
**Evidence**: `migrations/029_create_followups_table.sql:12`

### Rule: Session number nullable for complaints
**Purpose**: Absence escalations have session_number, complaints don't  
**Migration**: Made session_number nullable  
**Evidence**: `migrations/036_make_followups_nullable_for_complaints.sql`

---

## Follow-Ups & Complaints

### Rule: Dual-purpose followups table
**Types**: 'absence_escalation' (student absences), 'complaint' (student complaints)  
**Implementation**: `followups.type` field distinguishes them  
**Shared**: Both use same status tracking (NOT_CONTACTED, CONTACTED, RESOLVED)  
**Evidence**: `migrations/033_add_complaints_to_followups.sql:5`

### Rule: Follow-up status lifecycle
**States**: NOT_CONTACTED → CONTACTED → RESOLVED  
**Constraint**: CHECK constraint on `followups.status`  
**Evidence**: `migrations/029_create_followups_table.sql:8`

### Rule: Case notes for audit trail
**Purpose**: Track all updates/resolutions for follow-ups and complaints  
**Types**: `status_update` (intermediate notes), `resolution` (final closure note)  
**Evidence**: `migrations/034_create_followup_case_notes.sql`

### Rule: Soft delete support (unused)
**Fields**: `deleted_at`, `deleted_by_user_id`, `delete_reason` on `followups` table  
**Status**: Schema exists but feature not active (manager role removed)  
**Evidence**: `migrations/033_add_complaints_to_followups.sql:14-16`

### Rule: Late joiner capacity limits (NEW - pending implementation)
**Business Rule**: Late joiners allowed only if class has 4-5 students, max 6 after adding  
**Implementation**: NOT yet implemented - no capacity validation exists in current code  
**Evidence**: User requirement, verified missing from codebase  
**See**: `memory/flows/late_joiners.md`

---

## Permissions & Access

### Rule: Admin has broad access
**Implementation**: Admin role included in most RequireAnyRole checks  
**Exceptions**: Cannot unassign mentors (mentor_head only), cannot edit evaluations (mentor_head only)  
**Evidence**: `cmd/server/main.go` - route definitions

### Rule: Moderator has restricted access
**Behavior**: Allowed by middleware but gets 403 in handler logic  
**Affected routes**: `/classes`, `/finance`, pre-enrolment updates  
**Evidence**: Route middleware allows moderator, but handlers reject  
**TODO**: Confirm exact handler-level role checks

### Rule: Shared access (student_success + mentor_head)
**Routes**: Absence feed, follow-ups, resolve actions  
**Purpose**: Mentor head needs visibility into student issues  
**Evidence**: `cmd/server/main.go:312-389` - RequireAnyRole includes both

### Rule: Class workspace multi-role access
**Allowed**: mentor, mentor_head, admin, student_success  
**Purpose**: Different roles need class data for different reasons  
**Evidence**: `cmd/server/main.go:217`

---

## Database Constraints

### Rule: Cascade deletes for leads
**Behavior**: Deleting a lead deletes all related records  
**Tables affected**: placement_tests, offers, bookings, payments, scheduling, shipping, followups  
**Constraint**: `ON DELETE CASCADE` on foreign keys  
**Evidence**: All lead-related tables in `migrations/001_init.sql`

### Rule: Unique phone numbers
**Constraint**: `UNIQUE (phone)` on `leads` table  
**Effect**: Cannot create duplicate leads with same phone  
**Evidence**: `migrations/001_init.sql:16`

### Rule: Unique email for users
**Constraint**: `UNIQUE (email)` on `users` table  
**Effect**: Cannot create duplicate user accounts  
**Evidence**: `migrations/001_init.sql:6`

---

## System Configuration

### Rule: Current round tracking
**Storage**: `settings` table with key='current_round'  
**Purpose**: Track which enrollment round system is currently in  
**Evidence**: `migrations/004_classes_board.sql:12-20`

### Rule: Class group indexing
**Field**: `scheduling.class_group_index`  
**Purpose**: Track which class group within same level/days/time combination  
**Evidence**: `migrations/004_classes_board.sql:9`

---

## Audit & Tracking

### Rule: created_by tracking
**Implementation**: Most tables have `created_by_user_id` FK  
**Purpose**: Track which user created each record  
**Evidence**: Throughout migrations - users reference on creation fields

### Rule: Timestamps on all core tables
**Fields**: `created_at`, `updated_at`  
**Default**: CURRENT_TIMESTAMP  
**Evidence**: All table definitions include timestamps

### Rule: Closed round metadata
**Fields**: `closed_at`, `closed_by_mentor_user_id` on `class_groups`  
**Purpose**: Track when round closed and who closed it  
**Evidence**: `migrations/027_class_groups_round_status.sql:5-7`

---

## Notes

All rules listed above are **evidence-based** - they exist in migrations, database constraints, or handler code. Rules that are suspected but not confirmed are marked with **TODO** and listed in `open_questions.md`.
