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

### Rule: Credit consumption on session 1 completion
**Trigger**: When session 1 is marked as completed  
**Effect**: All students in the class have `leads.levels_consumed` incremented by 1  
**Purpose**: Tracks how many levels a student has consumed (affects refund eligibility and renewal)  
**Evidence**: `internal/models/repository.go` (`CompleteSession` function lines 3040-3060)

### Rule: Close round outcome logic
**Trigger**: Mentor Head closes a round  
**Decision Tree**:  
- If absences > 2 OR grade = 'F': Outcome = REPEAT (student must repeat level)  
- Otherwise: Outcome = PROMOTE (student advances to next level)  
**Side Effects**:  
- Students with no remaining credits get `high_priority_follow_up = true`  
- Class returned to Operations (`sent_to_mentor = false`)  
**Guards**: Cannot close round if any student is missing a session 8 grade.  
**Evidence**: `internal/models/repository.go` (`CloseRound` function), `docs/MILESTONE_2_IMPLEMENTATION_SUMMARY.md:159-163`

### Rule: Close-round grade validation uses active class roster only
**Behavior**: The close-round guard checks grades only for students who are actually in the running class roster (`leads.status = 'in_classes'`) for that `class_key`.  
**Reason**: Prevents false "missing final grade" errors from non-class leads that share schedule/level metadata but are not active class members.  
**Evidence**: `internal/models/repository.go` (`CloseRound` query), `internal/models/repository.go` (`GetStudentsInClassGroup`)

### Rule: Remaining credits are computed
**Rule**: Remaining credits = `levels_purchased_total - levels_consumed` (min 0)  
**Storage**: `leads.remaining_credits` is updated on round close for returning flow  
**Evidence**: `internal/models/repository.go` (`PromoteStudent`)

### Rule: Post‑class status uses pre‑deduction credits
**Rule**: `waiting_for_round` vs `renewal_pending` is decided using credits **before** the promotion deduction.  
**Example**: Student finishes with 1 credit → status `waiting_for_round`, remaining_credits becomes 0 after promotion.  
**Reason**: Students who still had credit at class end should appear in the Ops “waiting for round” flow even if the final credit is consumed by promotion.  
**Evidence**: `internal/models/repository.go` (`PromoteStudent`)

### Rule: Returning flow resets offer/payment snapshots
**Rule**: When a round is closed and a student becomes `is_returning = true`, the **current** Offer and Payment snapshot records are cleared so the next cycle starts clean.  
**Reason**: Prevents old “paid in full” data from leaking into the next pre‑enrolment cycle and auto‑staging the lead incorrectly.  
**History**: Course payment history remains in `lead_payments` (multi‑payment ledger) and `transactions`.  
**Evidence**: `internal/models/repository.go` (`PromoteStudent`), `internal/models/repository.go` (`GetLeadPayments`)

### Rule: Current-cycle payments only
**Rule**: Remaining balance and "paid in full" for returning students are computed from **current‑cycle** payments only.  
**Definition**: Current cycle starts at the latest class close time (`class_enrollments.completed_at`) for that lead.  
**Reason**: Prevents previous‑level payments from being deducted from a new offer.  
**Implementation**: All payment validation points check `is_returning` flag and use `GetTotalCoursePaidCurrentCycle` instead of `GetTotalCoursePaid`:
- Pre-enrolment detail page summary (remaining balance display)
- Add payment validation (prevents exceeding new offer price)
- Cancel flow refund validation (validates against current cycle only)
- Finance refund handler (validates against current cycle only)  
**Evidence**: `internal/models/repository.go` (`GetTotalCoursePaidCurrentCycle`), `internal/handlers/pre_enrolment.go` (lines 446-450, 954-965, 1963-1974), `internal/handlers/finance.go` (lines 331-348)

### Rule: Returning status is not auto‑staged
**Rule**: For `renewal_pending` or `waiting_for_round` leads, auto‑stage logic must **not** downgrade or re‑classify to NEW_LEAD/PAID_FULL based on old form data.  
**Reason**: Returning leads are a separate cycle; status changes should be explicit actions (offer/payment steps).  
**Evidence**: `internal/handlers/pre_enrolment.go` (auto‑stage guard for returning leads)

### Rule: Renewal offer auto-transition
**Rule**: When an offer is saved for a `renewal_pending` student, status auto-transitions to `offer_sent`.  
**Reason**: Provides clear visual feedback that Ops Admin has completed their work; allows filtering students waiting for offers vs waiting for payment.  
**Behavior**: Mirrors new student workflow (tested → offer_sent → paid); `is_returning` flag is preserved.  
**Evidence**: `internal/models/repository.go` (`ComputeStageFromFormCompletion` line 129)

### Rule: Repeat Level filter and badge
**UI**: Pre‑enrolment list has a **Repeat Level** filter and shows a **REPEAT** badge for latest outcome = `repeated`  
**Tooltip**: Shows repeat reason (grade F vs missed 3+ sessions) based on latest enrollment grade  
**Evidence**: `internal/views/pre_enrolment_list.html`, `internal/models/repository.go` (GetAllLeads with last outcome/grade)

### Rule: Returning student visibility (Ops)
**UI**: Pre‑enrolment list/detail shows remaining credits and last outcome (promoted vs repeated) for returning students.  
**Purpose**: Ops can immediately see how many levels are left and whether the student should repeat.  
**Evidence**: `internal/views/pre_enrolment_list.html`, `internal/views/pre_enrolment_detail.html`, `internal/models/repository.go` (`GetAllLeads`, `GetLeadByID`)

### Rule: Returning schedule defaults
**Rule**: Returning students (waiting_for_round / renewal_pending) default their class days/time to the **previous class schedule** (latest class_enrollments), editable by Ops.  
**Reason**: Ensures continuity and avoids empty schedule when returning.  
**Evidence**: `internal/handlers/pre_enrolment.go` (detail view defaults), `internal/models/repository.go` (latest class_enrollments schedule)

### Rule: Scheduling allowed with remaining credits
**Rule**: Returning students with `remaining_credits > 0` can set class days/time even if the current cycle is not fully paid.  
**Reason**: Prepaid levels from the previous bundle should allow immediate scheduling for the next round.  
**Evidence**: `internal/handlers/pre_enrolment.go` (schedule gate)

### Rule: Locked schedule does not block Save
**Rule**: If schedule fields are locked (not fully paid and no remaining credits), Save should not fail. Defaults may be displayed but are not applied.  
**Reason**: Ops must be able to save offer changes without being forced to schedule.  
**Evidence**: `internal/handlers/pre_enrolment.go` (schedule guard), `internal/views/pre_enrolment_detail.html` (locked inputs)

### Rule: Move dropdown options
**Rule**: “Move to” includes (1) Create new class with same days/time, (2) any available class for the level that is not sent/active/closed and not locked (6 students).  
**Reason**: Ops can move students across times/days or open a new group on the same schedule.  
**Evidence**: `internal/models/repository.go` (`GetMoveOptionsForLead`, `MoveStudentToClassKey`), `internal/views/classes.html`

### Rule: Move auto-creates missing class group
**Rule**: If a move target exists in scheduling but not in `class_groups`, the system creates a `class_groups` record on demand before moving.  
**Reason**: Avoids “target class not found” when classes exist only via scheduling.  
**Evidence**: `internal/models/repository.go` (`MoveStudentToClassKey`)

### Rule: 'At Risk' status for classes
**Trigger**: A class is marked "AT RISK" on the Student Success dashboard if any student in the class has `leads.high_priority = true`.
**Automation**: High priority is set automatically when a student misses 3+ sessions in their current level.
**Storage**: `leads.high_priority` (boolean) and `leads.high_priority_reason` (text).
**Evidence**: `internal/models/repository.go` (`UpdateAbsencePriority`), `frontend/src/pages/StudentSuccessDashboard.tsx`.

### Rule: Waiting for Round payment bypass
**Behavior**: Students with `waiting_for_round` status bypass the "paid in full" validation when being marked `ready_to_start`.  
**Reason**: These students finish their previous level with credits (e.g., 1 level bundle), so they've already "paid" for the next level upfront.  
**Evidence**: `internal/handlers/pre_enrolment.go:MarkReady` (bypass logic)

### Rule: Waiting-for-round appears as PAID_FULL in Ops list
**Behavior**: Returning students with remaining credits in waiting flow (`waiting_for_round`, `schedule_assigned`, `ready_to_start`) are shown as `PAID_FULL` in pre-enrolment payment column/filter even if current-cycle payment snapshot is empty.  
**Reason**: Their entitlement is prepaid credit, not a new cycle payment yet.  
**Evidence**: `internal/models/repository.go` (`GetAllLeads`)

### Rule: Waiting list action is credit-gated
**Behavior**: `Move to Waiting List` is only allowed when lead is returning and has prepaid entitlement for next round.  
**Blocked cases**:
- Non-returning leads.
- Returning leads with zero remaining credits unless they are already in waiting flow (`waiting_for_round` / `schedule_assigned` / `ready_to_start`).  
**Reason**: Prevents status-hopping from `renewal_pending` into `waiting_for_round` and bypassing payment checks.  
**Evidence**: `internal/handlers/pre_enrolment.go` (`move_waiting` action, `MarkWaiting`)

### Rule: Waiting for Round UI logic
**Behavior**: For `waiting_for_round` students:
- **Round / Schedule**: Locked warning is hidden and fields are unlocked (interactive).
- **Offer & Pricing**: Blocked (dimmed) with an "Already Paid" banner; no new offer needed.
- **Course Payment**: Blocked (dimmed); no new payment needed for this cycle.  
**Reason**: Since they already have credits, the pre-enrolment workflow only requires setting their new schedule. No financial actions are needed.  
**Evidence**: `internal/views/pre_enrolment_detail.html` (IsWaitingForRound conditions)

### Rule: Mentor compliance checks per session
**Scope**: Student Success only compliance auditing for each class session.  
**Storage**: `mentor_session_checks` (one row per `class_session_id`, enforced by UNIQUE).  
**Fields**: reminder_1d, reminder_1h, reminder_tasks, delay_minutes, is_absent, checked_by_user_id.  
**Behavior**: API upserts record per session; report engine aggregates by mentor.  
**Permissions**: Compliance modal/actions are available only to role `student_success`.  
**Evidence**:
- `internal/db/migrations/050_mentor_compliance.sql`
- `internal/models/compliance.go`
- `internal/handlers/api.go`

### Rule: Compliance completion banner at Session 8
**Trigger**: Student Success dashboard shows a banner when a class has reached 8 completed sessions but compliance checklist is incomplete.  
**Condition**: `completed_sessions >= 8` and `compliance_done < compliance_total` (session checks).  
**Action**: Banner links directly to class page with compliance modal auto-open (`open_compliance=1`).  
**Evidence**:
- `internal/handlers/api.go` (`GetStudentSuccessClasses`)
- `frontend/src/pages/StudentSuccessDashboard.tsx`
- `frontend/src/pages/StudentSuccessClass.tsx`

### Rule: Mentor reports aggregation
**API**: `GET /api/reports/mentors?mentor_id=<optional>` (single unified report)  
**Metrics per mentor**:
- Compliance Score = reminders sent / (checks * 3) * 100
- Punctuality = average `delay_minutes`
- Reliability = total `is_absent`
- Complaints = `followups.type='complaint'` (excluding soft-deleted)
**Scope**: If `round_status` is omitted, backend defaults to `active` so counts reflect running classes only.
**Evidence**:
- `internal/models/compliance.go` (`GetMentorComplianceReports`)
- `internal/handlers/api.go` (`GetMentorReports`)

### Rule: Mentor reports include per-class breakdown
**API**: `GET /api/reports/mentors/classes?mentor_id=<optional>`  
**Scope**: Defaults to `active` when `round_status` is omitted.  
**Behavior**: Returns class-level metrics (compliance, avg delay, absences, complaints) for each mentor class so payout/review can be done class-by-class.  
**UI**: Reports page shows mentor summary row, then nested active-class rows under each mentor.  
**Evidence**:
- `internal/models/compliance.go` (`GetMentorClassComplianceReports`)
- `internal/handlers/api.go` (`GetMentorClassReports`)
- `frontend/src/pages/ReportsPage.tsx`

### Rule: Mentor row click opens checklist details
**Behavior**: Clicking mentor name in reports opens a modal showing per-session checklist details captured by Student Success, grouped by class for readability.  
**Data source**: `GET /api/reports/mentors/checklist?mentor_id=...`  
**Scope**: If `round_status` is omitted, backend defaults to `active` so checklist rows match the unified report.
**Class drill-down**: Clicking a class row in the nested class table opens the same modal filtered to that class only.
**Evidence**:
- `frontend/src/pages/ReportsPage.tsx`
- `internal/handlers/api.go` (`GetMentorReportChecklist`)
- `internal/models/compliance.go` (`GetMentorComplianceChecklist`)

### Rule: Compliance schedule day label uses class slot (not calendar weekday)
**Behavior**: For classes with dual-day schedules (e.g., `Sat/Tues`), session labels alternate by session number: odd→first day, even→second day.  
**Reason**: Stored dates may not always reflect instructional slot day; report must show schedule slot semantics.  
**Evidence**:
- `frontend/src/components/ComplianceModal.tsx`
- `frontend/src/pages/ReportsPage.tsx`

### Rule: Reports row removal is a soft exclusion
**Behavior**: Removing a mentor row from reports does not delete class, mentor, or compliance data.  
**Mechanism**: Insert/upsert into `mentor_report_exclusions` keyed by `(mentor_user_id, round_status)`; reports query excludes matching rows.  
**Scope**: Exclusion is per report mode:
- `active` for Mid-Round report
- `closed` for End-Round report  
**Unified report behavior**: mentor is hidden only when excluded for both `active` and `closed`.  
**Permissions**: Admin and Mentor Head only.  
**Evidence**:
- `internal/db/migrations/051_mentor_report_exclusions.sql`
- `internal/models/compliance.go` (`ExcludeMentorFromReports`, `GetMentorComplianceReports`)
- `internal/handlers/api.go` (`ExcludeMentorReportRow`)

### Rule: Schedule preference persistence
**Rule**: When a student is promoted/detached from a class, their `class_days` and `class_time` are preserved in the `scheduling` table.  
**Behavior**: Only `class_group_index` is cleared to remove them from a specific class instance. The days/time remain as defaults for the next pre-enrolment round.  
**Reason**: Students usually prefer the same schedule; this minimizes data entry for Ops Admin.  
**Evidence**: `internal/models/repository.go:PromoteStudent`

### Rule: Grade entry at session 8
**Timing**: Grades can only be entered at session 8 (final session)  
**Values**: A, B, C, F (CHECK constraint)  
**Storage**: `grades` table with `session_number = 8`  
**Uniqueness**: One grade per student per class (UNIQUE constraint on lead_id, class_key, session_number)  
**Evidence**: `migrations/046_after_class_pipeline.sql`, `internal/models/repository.go` (`CloseRound` function)

### Rule: Final grading notes are class-scoped and journey-visible
**Persistence**: Final grading notes are saved in `grades.notes` and persist in DB for that student+class record.  
**Visibility**:
- Shown in class grading context (`GET /api/grades?class_key=...`).
- Also shown in student journey timeline as `type='grade_note'` via `GET /api/students/:id/notes` (Students tab).  
**Carry-forward model**: Notes are not duplicated into `student_notes`; timeline composes data from `student_notes`, `followups`, and `grades` for cross-level visibility.  
**Implication**: Next mentor can see previous mentor final grading notes from the student record view.  
**Evidence**: `internal/db/migrations/018_create_grades.sql`, `internal/handlers/grades.go`, `internal/models/student_profile_repository.go` (`GetStudentNotesTimeline`), `frontend/src/components/StudentProfileModal.tsx`

### Rule: Student notes persistence
**Scope**: Notes added by mentors/mentor heads/student success  
**Visibility**: All notes visible to all authorized roles (mentor, mentor_head, admin, student_success)  
**Persistence**: Notes persist across sessions and rounds  
**Optional Fields**: Can be linked to specific class_key and session_number  
**Privacy**: `is_private` flag controls visibility (implementation TBD)  
**Evidence**: `migrations/019_create_student_notes.sql`, `internal/models/repository.go` (`AddStudentNote`, `GetStudentNotes`)

### Rule: Community officer feedback timing
**Collection Points**: Sessions 4 and 8 only  
**Purpose**: Mid-round (session 4) and end-of-round (session 8) feedback collection  
**Storage**: `community_officer_feedback` table  
**Evidence**: `migrations/020_create_community_officer_feedback.sql`, `docs/MILESTONE_2_IMPLEMENTATION_SUMMARY.md:170-176`

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

### Rule: Manual Mentor Head Visibility
**Rule**: Transition to `sent_to_mentor = true` is strictly manual via Ops Admin dashboard.
**Explicitly Excluded**: The `StartRound` action does NOT automatically trigger this transition.
**Evidence**: `internal/handlers/classes.go` (SendToMentor), `memory/flows/class_lifecycle.md`

### Rule: Class metadata stored in class_key
**Format**: `{level}-{days}-{time}-{number}` (e.g., "3-TuTh-5:00PM-1")  
**Evidence**: Database uses class_key as TEXT primary key in `class_groups`

---

## Pre-Enrolment Pipeline

### Rule: Lead status progression
**Pipeline**: lead_created → test_booked → tested → offer_sent → booking_confirmed → paid_full/deposit_paid → waiting_for_round → schedule_assigned → ready_to_start → in_classes  
**Constraint**: CHECK constraint on `leads.status`  
**Evidence**: `migrations/001_init.sql:19-22`, `migrations/004_classes_board.sql:3-6`

### Rule: Pre-Enrolment dashboard status filters (Milestone 3)
**Filters**: All lead statuses are available as dropdown filters and quick filter buttons in the pre-enrolment dashboard
**New Statuses**: "Waiting for Round" and "Renewal Pending" added in Milestone 3
**Badge Colors**:
- `waiting_for_round`: Light blue (#4EC6E0)
- `renewal_pending`: Orange (#FFA500)
**Evidence**: `internal/views/pre_enrolment_list.html` (dropdown options, quick filter buttons, CSS)

### Rule: Promoted student visual indicator (Milestone 3)
**Trigger**: When `leads.is_returning = true`
**Display**: Gold star (⭐) badge appears next to student name in:
- Pre-enrolment list
- Pre-enrolment detail header (gradient badge: "⭐ Promoted Student")
- Classes board student list
**Purpose**: Visually identify students who have completed a previous level and are returning for the next level
**Evidence**: `internal/views/pre_enrolment_list.html`, `internal/views/pre_enrolment_detail.html`, `internal/views/classes.html`

### Rule: Renewal student banner (Milestone 3)
**Trigger**: When lead status is `renewal_pending`
**Display**: Green informational banner in the "Offer & Pricing" section
**Message**: "✓ Renewal Student: This student has completed a previous level. Set a new offer to sell additional levels."
**Purpose**: Guide ops admin to create a new offer for returning students
**Evidence**: `internal/views/pre_enrolment_detail.html`


### Rule: Waiting list returns lead to pre-enrolment feed
**Scope**: Any action that sets `status = waiting_for_round`  
**Behavior**: Clears `sent_to_classes` so the lead appears in Pre-Enrolment and is removed from Classes board.  
**Evidence**: `internal/models/repository.go` (`UpdateLeadStatus`).

### Rule: Send-to-Classes does not join active rounds
**Scope**: Pre-Enrolment "Send to Classes" action  
**Behavior**: Assigns the lead to a non-started class group for the same level/days/time (or opens a new group if all are full).  
**Active rounds**: Joining a running class requires the Late Joiner flow.  
**Evidence**: `internal/models/repository.go` (`SendLeadToClasses`, `AssignClassGroup`, `MoveStudentBetweenGroups`), `internal/views/pre_enrolment_detail.html` note.

### Rule: Ops cannot assign into sent-to-mentor or closed classes
**Scope**: Send-to-Classes and move operations on Ops classes board.  
**Behavior**: Assignments and moves must avoid class groups that are `sent_to_mentor = true` or `round_status = 'closed'`.  
**Reason**: Prevents leaking students into mentor-managed or archived groups.  
**Evidence**: `internal/models/repository.go` (`AssignClassGroup`, `MoveStudentBetweenGroups`, `GetAvailableGroupsForMove`).

### Rule: Cancel lead requires refund modal when payments exist
**Scope**: Pre-Enrolment cancellations  
**Behavior**: Direct delete is disabled; cancellation must go through the cancel flow and show refund modal if course payments exist.  
**Evidence**: `internal/handlers/pre_enrolment.go` (cancel action), `internal/views/pre_enrolment_detail.html` cancel modal; list action routes to `?action=cancel`.

### Rule: Returning student cancellation includes unused credits refund
**Scope**: Pre-Enrolment cancellations for ANY student with `remaining_credits > 0` (includes `IsReturning`, `renewal_pending`, `waiting_for_round`)  
**Calculation**: `TotalRefundableAmount = current_cycle_payments + (remaining_credits × price_per_level)`  
**Price Source**: Original bundle purchase price from `offers.final_price` ÷ `levels_purchased_total`  
**Behavior**: Even if current-cycle payments = 0, the refund modal appears if unused credits exist  
**Modal Display**: Shows breakdown of current cycle payments, unused credits value, and total refundable amount  
**Evidence**: `internal/models/repository.go` (`CalculateUnusedCreditsRefund`), `internal/handlers/pre_enrolment.go` (cancel flow checks `hasRemainingCredits`), `internal/views/pre_enrolment_detail.html` (cancel modal)

### Rule: Bundle dropout refunds charge consumed levels at default price
**Scope**: Pre-Enrolment cancellations for students with multi-level bundles (2+ levels) who drop out after completing some levels  
**Calculation**: Refund = `Total Paid - (Consumed Levels × SINGLE_LEVEL_PRICE)`  
**Default Price**: 1,300 EGP per level  
**Rationale**: Students who buy multi-level bundles receive a discount for upfront commitment. If they drop out before completing all levels, we cancel the discounted deal and charge the consumed levels at the standard single-level price (1,300 EGP). They get refunded the difference.  
**Example**: Student paid 3,300 EGP for 3 levels (1,100 EGP/level discounted), completed 1 level. Refund = 3,300 - (1 × 1,300) = 2,000 EGP.  
**Exception**: Students who purchased a single level (1-level bundle) get refunded exactly what they paid.  
**Remaining Credits**: Calculated dynamically as `levels_purchased_total - levels_consumed` to ensure accuracy.  
**Evidence**: `internal/models/repository.go` (`CalculateUnusedCreditsRefund` with `SINGLE_LEVEL_PRICE_EGP = 1300`)

### Rule: One-to-one relationships for lead data
**Tables**: placement_tests, offers, bookings, payments, scheduling, shipping all have UNIQUE `lead_id`  
**Effect**: One placement test, one offer, one booking, etc. per lead  
**Evidence**: `migrations/001_init.sql` - UNIQUE constraints on lead_id

### Rule: Assigned level range  
**Initial**: 1-4  
**Updated**: 1-8  
**Constraint**: CHECK on `placement_tests.assigned_level`  
**Evidence**: `migrations/003_assigned_level_1_to_8.sql`

### Rule: Placement test booking vs. results ownership
**Booking**: Ops Admin sets test date/time/type in Pre-Enrolment and saves.  
**Result**: Student Success records assigned level + test notes after test.  
**Status**: Saving results sets `leads.status = tested` **only if** lead is still `lead_created` or `test_booked`.  
**Evidence**:
- `internal/handlers/pre_enrolment.go` (SaveFull auto-stage test_booked)
- `internal/models/repository.go` (ComputeStageFromFormCompletion)
- `internal/handlers/api.go` (CompletePlacementTest)

---

## Attendance & Absence

### Rule: Three attendance statuses
**Values**: `present`, `absent_excused`, `absent_unexcused`  
**Constraint**: CHECK constraint on `attendance.status`  
**Evidence**: `migrations/017_create_attendance.sql`

### Rule: Attendance deadline (24 hours)
**Rule**: Mentors can mark attendance up to 24 hours after scheduled session end time (computed in **Africa/Cairo** time)  
**Override**: Admin, Mentor Head, and Student Success can bypass for corrections by setting `enforceDeadline = false`  
**Implementation**: `MarkAttendance()` function accepts `enforceDeadline bool` parameter  
- When `true`: Blocks updates after 24 hours past scheduled session end time  
- When `false`: Allows updates at any time (for admin corrections)  
**UI**: Mentor class workspace shows a red banner when attendance is missing past 24 hours  
**Evidence**: `internal/models/repository.go:3159` (MarkAttendance signature with enforceDeadline parameter), `internal/handlers/mentor.go`, `internal/views/mentor_class_detail.html`, `frontend/src/pages/ClassWorkspace.tsx`

### Rule: Attendance required before completing session
**Rule**: A session cannot be marked completed unless attendance is recorded for all applicable students  
**Excludes**: Late joiners for sessions before their `joined_at_session_number`  
**Effect**: `CompleteSession` returns an error and blocks completion  
**Evidence**: `internal/models/repository.go` (`CompleteSession` validation)

### Rule: Unique absence follow-up per session
**Constraint**: `UNIQUE (class_key, lead_id, session_number)` on `followups` table  
**Effect**: Cannot create duplicate follow-ups for same absence  
**Evidence**: `migrations/029_create_followups_table.sql:12`

### Rule: Session number nullable for complaints
**Purpose**: Absence escalations have session_number, complaints don't  
**Migration**: Made session_number nullable  
**Evidence**: `migrations/036_make_followups_nullable_for_complaints.sql`

### Rule: Canonical session completion endpoint
**Canonical**: `POST /api/session/complete`  
**Legacy**: `POST /api/classes/:id/sessions/:n/complete` (deprecated; logs warning)  
**Behavior**: Both call the same `CompleteSession` logic for side effects.  
**Evidence**: `internal/handlers/api.go` (CompleteSession, CompleteSessionByNumber)

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
**Status**: Soft-deleted follow-ups are excluded from queries and updates (no UI yet)  
**Evidence**: `migrations/033_add_complaints_to_followups.sql:14-16`

### Rule: Late joiner capacity limits (NEW - pending implementation)
**Business Rule**: Late joiners allowed only if class has 4-5 students, max 6 after adding  
**Implementation**: NOT yet implemented - no capacity validation exists in current code  
**Evidence**: User requirement, verified missing from codebase  
**See**: `memory/flows/late_joiners.md`

---

## Finance

### Rule: Ledger backfill for legacy payments
**Scope**: Placement test payments + course payments created before finance tracking  
**Mechanism**: On finance dashboard load, missing `transactions` rows are backfilled from `placement_tests` and `lead_payments`  
**Idempotent**: Uses `ref_key` uniqueness to avoid duplicates  
**Evidence**: `internal/models/repository.go` (EnsureFinanceLedgerSync), `internal/handlers/finance.go`

### Rule: Session-based refund calculation
**Trigger**: When calculating refund for a cancelled lead  
**Logic**: Uses session completion markers (`completed_at`), NOT wall-clock time  
- **Before session 1 completed**: 100% of course paid is refundable  
- **After session 1 completed, before session 2**: 50% of course paid is refundable  
- **After session 2 completed**: 0% refundable (no refund available)  
**Implementation**: `GetRefundableAmount()` checks `class_sessions.completed_at IS NOT NULL` for sessions 1 and 2  
**UI Impact**: Cancel modal shows calculated refund amount and blocks higher refund requests  
**Evidence**: `internal/models/repository.go:3394-3447` (GetRefundableAmount function), `docs/MILESTONE_2_IMPLEMENTATION_SUMMARY.md:115-128`

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

### Rule: Mentor schedule conflict detection
**Scope**: Mentor assignment to classes  
**Validation**: System checks for overlapping sessions when assigning mentor  
**Conflict Definition**: Same mentor cannot be assigned to sessions with overlapping date/time  
**Implementation**: `CheckMentorScheduleConflict()` queries existing sessions for the mentor  
**Effect**: Assignment rejected with error message if conflict detected  
**Evidence**: `internal/models/repository.go:3499-3518` (CheckMentorScheduleConflict), `docs/MILESTONE_2_IMPLEMENTATION_SUMMARY.md:136-139`

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

## Universal Student Profile (Milestone 4)

### Rule: Global student search
**Access**: All roles (admin, moderator, mentor_head, mentor, student_success)  
**Search Criteria**: Name (partial, case-insensitive) or phone (exact match)  
**Minimum Query**: 2 characters required  
**Result Limit**: Maximum 20 results  
**Debounce**: 300ms delay to avoid excessive API calls  
**Evidence**: `internal/handlers/student_profile.go`, `frontend/src/components/StudentSearch.tsx`

### Rule: Student profile visibility
**Access**: All roles can view full student profiles (read-only)  
**Profile Data**: Name, phone, level, status, remaining credits, is_returning flag  
**Evidence**: `internal/handlers/student_profile.go` (`GetStudentProfile`), `cmd/server/main.go` (route registration)

### Rule: Academic history source
**Source**: `class_enrollments` table (historical snapshot)  
**Created**: When a round is closed via `CloseRound` function  
**Data**: Level, schedule, mentor, grade, outcome, enrollment/completion dates  
**Evidence**: `migrations/046_after_class_pipeline.sql`, `internal/models/student_profile_repository.go` (`GetAcademicHistory`)

### Rule: Current status visibility
**Condition**: Only shown if student status is `in_classes`  
**Data**: Current class info, attendance stats (present/absent/late), session-by-session breakdown  
**Calculation**: Aggregated from `attendance` table  
**Evidence**: `internal/models/student_profile_repository.go` (`GetCurrentClassStatus`)

### Rule: Notes timeline aggregation
**Sources**: Combines `student_notes` and `followups` tables  
**Order**: Chronological (newest first)  
**Privacy**: Private notes visible to all roles (as per current system design)  
**Context**: Includes class_key and session_number when applicable  
**Evidence**: `internal/models/student_profile_repository.go` (`GetStudentNotesTimeline`)

### Rule: Promoted student badge
**Display**: Star badge (⭐) shown for students with `is_returning = true`  
**Location**: Profile header, pre-enrolment lists, class rosters  
**Purpose**: Visual indicator for promoted students from previous rounds  
**Evidence**: `frontend/src/components/StudentProfileModal.tsx`, `templates/pre_enrolment_list.html`

---

## UI/UX & State Management

### Rule: Dashboard tab persistence
**Rule**: Dashboards (Mentor, Student Success) preserve the active tab on page refresh.  
**Mechanism**: Tab state is initialized lazily from URL `tab` parameter or `localStorage` on mount.  
**Evidence**: `frontend/src/pages/ClassWorkspace.tsx`, `frontend/src/pages/StudentSuccessClass.tsx`

---

## Notes

All rules listed above are **evidence-based** - they exist in migrations, database constraints, or handler code. Rules that are suspected but not confirmed are marked with **TODO** and listed in `open_questions.md`.
