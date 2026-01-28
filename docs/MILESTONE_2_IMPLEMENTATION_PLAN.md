# Milestone 2 (Active Classes) — Implementation Plan

**Date:** 2026-01-25  
**Status:** PLAN ONLY (Step 1) — Ready for Build Step 2  
**Source of Truth:** `docs/SYSTEM_SPEC_SNAPSHOT.md` + actual codebase inspection

---

## 1) REPO REALITY SUMMARY

### Existing Routes/Pages Involved

**Classes Board (`/classes`):**
- Handler: `classesHandler.List` (GET)
- Template: `classes.html` → `classes_content`
- Current behavior: Shows class groups (level, days, time, group_index) with READY/NOT READY/LOCKED status
- Actions: Start Round, Send to Mentor Head, Return, Move students
- Access: Admin only (moderator gets 403 access-restricted page)

**Pre-Enrolment (`/pre-enrolment`):**
- Handler: `preEnrolmentHandler.List` (GET), `preEnrolmentHandler.Detail` (GET), `preEnrolmentHandler.Update` (POST)
- Templates: `pre_enrolment_list.html`, `pre_enrolment_detail.html`
- Current behavior: Lead management, status transitions, payments, scheduling
- Access: Admin + Moderator (moderator has limited edit)

**Finance (`/finance`):**
- Handler: `financeHandler.Dashboard` (GET), `financeHandler.CreateRefund` (POST)
- Template: `finance.html`
- Current behavior: Ledger, cash balance, refunds
- Access: Admin only (moderator gets 403 access-restricted page)

### Existing DB Migration Method

- **Pattern:** Sequential numbered SQL files in `internal/db/migrations/` (001_init.sql, 002_..., etc.)
- **Naming:** `{number}_{descriptive_name}.sql`
- **Execution:** `db.RunMigrations()` reads files in order, tracks applied migrations in `schema_migrations` table
- **Style:** Uses `CREATE TABLE IF NOT EXISTS`, `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`, `ON CONFLICT DO NOTHING` for idempotency
- **Constraints:** Uses `CHECK` constraints for enums, `UNIQUE` constraints with `WHERE ... IS NOT NULL` for nullable unique keys
- **Indexes:** Created with `CREATE INDEX IF NOT EXISTS`

### Existing Classes/Round Data Structure

**Current State:**
- `class_groups` table: Tracks workflow (`sent_to_mentor`, `sent_at`, `returned_at`)
- `scheduling` table: Links leads to class groups via `class_group_index`
- `settings` table: Stores `current_round` (integer as TEXT)
- **Round Start Logic:** `StartRound()` in `repository.go`:
  - Finds students in READY (4-5 students) or LOCKED (6+ students) groups
  - Updates `leads.status = 'in_classes'` for those students
  - Increments `current_round` in settings
  - **Does NOT create sessions** (gap identified in snapshot)

**Class Key Format:**
- `GenerateClassKey(level, classDays, classTime, classNumber)` → `"L{level}|{days}|{time}|{index}"`
- Example: `"L1|Sun/Wed|07:30|1"`

**Student Grouping:**
- Grouped by: `(placement_tests.assigned_level, scheduling.class_days, scheduling.class_time, scheduling.class_group_index)`
- Readiness: READY = 6 students, NOT READY < 6, LOCKED > 6

### Existing RBAC Pattern

- **Middleware:** `middleware.RequireAnyRole([]string{"admin", "moderator"}, sessionSecret)`
- **Access-Restricted Page:** Custom 403 page via `access_restricted.html` template (used for `/classes` and `/finance` for moderators)
- **Role Enum:** `users.role` CHECK constraint (currently: 'admin', 'moderator', 'community_officer')
- **Helper Functions:** `IsModerator(r)`, `IsAdmin(r)` in `internal/handlers/role.go`

### Existing Refund/Cancel Flow

- **Cancel Flow:** `CreateCancelRefundIdempotent()` uses `ref_key="cancel_refund:{leadID}:{date}:{amount}"` with `ON CONFLICT (ref_key) DO NOTHING`
- **Finance Refund:** `CreateRefund()` uses `ref_key="lead:{leadID}:refund:{uuid}"`
- **Validation:** Uses `GetTotalCoursePaid(leadID)` = sum(`lead_payments`) - sum(refund transactions)
- **No session-based refund rules** currently implemented

### Timezone Handling

- **Current Pattern:** Uses `time.Now()` (server local timezone)
- **Date Parsing:** `util.ParseDateLocal()` parses YYYY-MM-DD and returns start of day in local timezone
- **DB Storage:** `TIMESTAMP WITH TIME ZONE` for timestamps, `DATE` for dates, `TIME` for times
- **No explicit UTC conversion** in current codebase; assumes server timezone = local timezone
- **Recommendation:** Store session times in UTC, display in local (need to add timezone conversion utilities)

---

## 2) MINIMAL DB CHANGES

### New Tables

#### `class_sessions`
```sql
CREATE TABLE IF NOT EXISTS class_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    session_number INTEGER NOT NULL CHECK (session_number >= 1 AND session_number <= 8),
    scheduled_date DATE NOT NULL,
    scheduled_time TIME NOT NULL,
    scheduled_end_time TIME, -- Calculated: scheduled_time + 2 hours (or configurable duration)
    actual_date DATE,
    actual_time TIME,
    actual_end_time TIME,
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'completed', 'cancelled')),
    completed_at TIMESTAMP WITH TIME ZONE, -- Timestamp when session was marked completed (for refund rule)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (class_key, session_number)
);

CREATE INDEX IF NOT EXISTS idx_class_sessions_class_key ON class_sessions(class_key);
CREATE INDEX IF NOT EXISTS idx_class_sessions_status ON class_sessions(status);
CREATE INDEX IF NOT EXISTS idx_class_sessions_date_time ON class_sessions(scheduled_date, scheduled_time);
CREATE INDEX IF NOT EXISTS idx_class_sessions_completed_at ON class_sessions(completed_at) WHERE completed_at IS NOT NULL;
```

**Note:** 
- `scheduled_end_time` and `actual_end_time` needed for mentor conflict detection (overlap checking).
- `completed_at` timestamp used for refund rule calculation (session 1/2 completion detection).

#### `attendance`
```sql
CREATE TABLE IF NOT EXISTS attendance (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    attended BOOLEAN NOT NULL DEFAULT false,
    notes TEXT,
    marked_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (session_id, lead_id)
);

CREATE INDEX IF NOT EXISTS idx_attendance_session_id ON attendance(session_id);
CREATE INDEX IF NOT EXISTS idx_attendance_lead_id ON attendance(lead_id);
CREATE INDEX IF NOT EXISTS idx_attendance_attended ON attendance(attended) WHERE attended = false;
```

#### `grades`
```sql
CREATE TABLE IF NOT EXISTS grades (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    session_number INTEGER NOT NULL CHECK (session_number = 8),
    grade TEXT NOT NULL CHECK (grade IN ('A', 'B', 'C', 'F')),
    notes TEXT,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (lead_id, class_key, session_number)
);

CREATE INDEX IF NOT EXISTS idx_grades_lead_id ON grades(lead_id);
CREATE INDEX IF NOT EXISTS idx_grades_class_key ON grades(class_key);
CREATE INDEX IF NOT EXISTS idx_grades_grade ON grades(grade);
```

#### `student_notes`
```sql
CREATE TABLE IF NOT EXISTS student_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT REFERENCES class_groups(class_key) ON DELETE SET NULL,
    session_number INTEGER,
    note_text TEXT NOT NULL,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_student_notes_lead_id ON student_notes(lead_id);
CREATE INDEX IF NOT EXISTS idx_student_notes_class_key ON student_notes(class_key) WHERE class_key IS NOT NULL;
```

#### `community_officer_feedback`
```sql
CREATE TABLE IF NOT EXISTS community_officer_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    session_number INTEGER NOT NULL CHECK (session_number IN (4, 8)),
    feedback_text TEXT NOT NULL,
    follow_up_required BOOLEAN DEFAULT false,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (lead_id, class_key, session_number)
);

CREATE INDEX IF NOT EXISTS idx_co_feedback_lead_id ON community_officer_feedback(lead_id);
CREATE INDEX IF NOT EXISTS idx_co_feedback_class_key ON community_officer_feedback(class_key);
CREATE INDEX IF NOT EXISTS idx_co_feedback_follow_up ON community_officer_feedback(follow_up_required) WHERE follow_up_required = true;
```

#### `absence_follow_up_logs`
```sql
CREATE TABLE IF NOT EXISTS absence_follow_up_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    session_id UUID REFERENCES class_sessions(id) ON DELETE SET NULL,
    message_sent BOOLEAN DEFAULT false,
    reason TEXT,
    student_reply TEXT,
    action_taken TEXT,
    notes TEXT,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_absence_logs_lead_id ON absence_follow_up_logs(lead_id);
CREATE INDEX IF NOT EXISTS idx_absence_logs_session_id ON absence_follow_up_logs(session_id) WHERE session_id IS NOT NULL;
```

#### `mentor_assignments`
```sql
CREATE TABLE IF NOT EXISTS mentor_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mentor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by_user_id UUID REFERENCES users(id),
    UNIQUE (class_key) -- One mentor per class
);

CREATE INDEX IF NOT EXISTS idx_mentor_assignments_mentor_user_id ON mentor_assignments(mentor_user_id);
CREATE INDEX IF NOT EXISTS idx_mentor_assignments_class_key ON mentor_assignments(class_key);
```

**Note:** Uses `users` table directly (role='mentor'), no separate `mentors` table.

### Modified Tables

#### `users` (add roles)
```sql
-- Migration: 014_add_mentor_roles.sql
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN (
    'admin', 'moderator', 'community_officer', 'mentor_head', 'mentor'
));
```

#### `leads` (add follow-up flag)
```sql
-- Migration: 015_add_follow_up_flag.sql
ALTER TABLE leads ADD COLUMN IF NOT EXISTS high_priority_follow_up BOOLEAN DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_leads_high_priority_follow_up ON leads(high_priority_follow_up) WHERE high_priority_follow_up = true;
```

**Purpose:** Mark students who need Operations follow-up after round closes (no remaining credits).

**Important:** 
- Only set by `mentor_head` when closing round (explicit state, not auto-set by queries).
- Viewable/filterable on `/pre-enrolment?follow_up=high_priority` by admin and moderator.

### New Enums

**Session Status:**
- `'scheduled'` (default)
- `'completed'`
- `'cancelled'`

**Grade:**
- `'A'`, `'B'`, `'C'`, `'F'`

**User Roles (updated):**
- `'admin'`, `'moderator'`, `'community_officer'`, `'mentor_head'`, `'mentor'`

### Unique Constraints

- `class_sessions`: `UNIQUE (class_key, session_number)` — prevents duplicate sessions per class
- `attendance`: `UNIQUE (session_id, lead_id)` — one attendance record per student per session
- `grades`: `UNIQUE (lead_id, class_key, session_number)` — one grade per student per class (session 8 only)
- `community_officer_feedback`: `UNIQUE (lead_id, class_key, session_number)` — one feedback per student per milestone session
- `mentor_assignments`: `UNIQUE (class_key)` — one mentor per class

### Indexes for Performance

**Mentor Conflict Detection:**
- `idx_class_sessions_date_time` on `(scheduled_date, scheduled_time)` — for overlap queries
- `idx_mentor_assignments_mentor_user_id` — for finding all classes assigned to a mentor

**Attendance Queries:**
- `idx_attendance_attended` on `(attended) WHERE attended = false` — for absence reports

**Follow-up Queries:**
- `idx_leads_high_priority_follow_up` — for Operations filter
- `idx_co_feedback_follow_up` — for pending follow-ups

---

## 3) ENDPOINTS

### Mentor Head Routes

#### `GET /mentor-head` (Protected: mentor_head + admin)
- **Handler:** `mentorHeadHandler.Dashboard`
- **Purpose:** List all classes where `class_groups.sent_to_mentor = true`
- **Data:** Class groups with student count, readiness, assigned mentor (if any)
- **Actions:** Assign mentor, return class, view mentor schedule conflicts

#### `POST /mentor-head/assign` (Protected: mentor_head + admin)
- **Handler:** `mentorHeadHandler.AssignMentor`
- **Parameters:** `class_key`, `mentor_user_id` (UUID of user with role='mentor')
- **Validation:**
  - Verify user exists and has role='mentor'
  - Check mentor schedule conflicts (overlapping sessions)
  - Ensure class is sent to mentor head (`sent_to_mentor = true`)
- **Action:** Insert/update `mentor_assignments` (mentor_user_id references users.id), log assignment

#### `POST /mentor-head/return/{classKey}` (Protected: mentor_head + admin)
- **Handler:** `mentorHeadHandler.ReturnClass`
- **Purpose:** Return class from mentor head back to Operations
- **Action:** Update `class_groups.sent_to_mentor = false`, `returned_at = NOW()`

#### `POST /mentor-head/session/cancel` (Protected: mentor_head + admin)
- **Handler:** `mentorHeadHandler.CancelSession`
- **Parameters:** `session_id`, `compensation_date`, `compensation_time`
- **Purpose:** Cancel a session and reschedule it (compensation)
- **Action:**
  - Update `class_sessions.status = 'cancelled'` for target session
  - **Reschedule the same session_number** (update `scheduled_date` and `scheduled_time` to compensation values, set `status = 'scheduled'`)
  - **Do NOT create session 9** — round always has exactly 8 sessions
  - Validate mentor schedule conflict for rescheduled session

#### `POST /mentor-head/close-round` (Protected: mentor_head + admin)
- **Handler:** `mentorHeadHandler.CloseRound`
- **Purpose:** Close the round, compute outcomes, send back to Operations
- **Action:**
  - For each student in class:
    - Count absences (from `attendance` where `attended = false`)
    - Get grade (from `grades` where `session_number = 8`)
    - Compute decision: repeat if `absences > 2 OR grade = 'F'`, else promote
    - **If `levels_consumed >= levels_purchased_total`:** set `leads.high_priority_follow_up = true` (explicit state, only set here)
  - Update `class_groups.sent_to_mentor = false` (return to Operations)
  - Update lead statuses (new status TBD: `'round_completed'` or keep `'in_classes'` with outcome flag)

### Mentor Routes

#### `GET /mentor` (Protected: mentor + admin)
- **Handler:** `mentorHandler.Dashboard`
- **Purpose:** List all classes assigned to current mentor (`mentor_assignments.mentor_user_id = current_user.id`)
- **Data:** Classes with session timeline, attendance summary, pending actions

#### `GET /mentor/class/{classKey}` (Protected: mentor + admin)
- **Handler:** `mentorHandler.ClassDetail`
- **Purpose:** Show class detail with 8 sessions, student list, attendance entry forms
- **Data:** Class info, sessions (with status), students with attendance history, notes

#### `POST /mentor/attendance` (Protected: mentor + admin)
- **Handler:** `mentorHandler.MarkAttendance`
- **Parameters:** `session_id`, `lead_id`, `attended` (boolean), `notes` (optional)
- **Validation:**
  - Session must belong to a class assigned to current mentor
  - Session status must be `'completed'` or `'scheduled'` (can mark attendance for completed sessions retroactively)
- **Action:** Upsert `attendance` record (ON CONFLICT update)

#### `POST /mentor/grade` (Protected: mentor + admin)
- **Handler:** `mentorHandler.EnterGrade`
- **Parameters:** `lead_id`, `class_key`, `grade` ('A', 'B', 'C', 'F'), `notes` (optional)
- **Validation:**
  - Session 8 must be `'completed'` for the class
  - Grade must be one of: 'A', 'B', 'C', 'F'
  - Class must be assigned to current mentor
- **Action:** Insert/update `grades` record (UNIQUE on lead_id, class_key, session_number=8)

#### `POST /mentor/note` (Protected: mentor + admin)
- **Handler:** `mentorHandler.AddNote`
- **Parameters:** `lead_id`, `class_key`, `session_number` (optional), `note_text`
- **Action:** Insert `student_notes` record

#### `POST /mentor/session/complete` (Protected: mentor + admin)
- **Handler:** `mentorHandler.CompleteSession`
- **Parameters:** `session_id`, `actual_date` (optional), `actual_time` (optional)
- **Validation:**
  - Session must belong to a class assigned to current mentor
  - Session status must be `'scheduled'`
- **Action:**
  - Update `class_sessions.status = 'completed'`, set `actual_date`, `actual_time`, `completed_at = NOW()`
  - If `session_number = 1`: Auto-increment `leads.levels_consumed += 1` (credit consumption)
  - Auto-update attendance for all students in class (default to `attended = false`, mentor can edit later)

### Community Officer Routes

#### `POST /community-officer/feedback` (Protected: community_officer + admin)
- **Handler:** `communityOfficerHandler.SubmitFeedback`
- **Parameters:** `lead_id`, `class_key`, `session_number` (4 or 8), `feedback_text`, `follow_up_required` (boolean)
- **Validation:**
  - Session number must be 4 or 8
  - Session must be `'completed'`
- **Action:** Insert/update `community_officer_feedback` (UNIQUE on lead_id, class_key, session_number)

#### `POST /community-officer/follow-up` (Protected: community_officer + admin)
- **Handler:** `communityOfficerHandler.LogFollowUp`
- **Parameters:** `lead_id`, `session_id` (optional), `message_sent` (boolean), `reason`, `student_reply`, `action_taken`, `notes`
- **Action:** Insert `absence_follow_up_logs` record

### Classes Routes (Extended)

#### `GET /classes/active` (Protected: admin only)
- **Handler:** `classesHandler.ActiveClasses`
- **Purpose:** Show classes with `status = 'in_classes'` (active round)
- **Data:** Classes with session timeline, attendance status, grades, community officer feedback status
- **Note:** Extends existing `/classes` page; could be a tab/filter rather than separate route

#### `POST /classes/{classKey}/sessions/create` (Protected: admin only)
- **Handler:** `classesHandler.CreateSessions`
- **Purpose:** Create 8 sessions for a class when round starts
- **Parameters:** `class_key`, `start_date`, `start_time`, `session_duration_hours` (default: 2)
- **Action:**
  - For session 1-8:
    - `scheduled_date = start_date + (session_number - 1) * 7 days` (weekly sessions)
    - `scheduled_time = start_time`
    - `scheduled_end_time = start_time + duration`
  - Insert 8 `class_sessions` records
- **Called from:** `StartRound()` function (modify existing function)

### Pre-Enrolment Routes (Extended)

#### `GET /pre-enrolment?follow_up=high_priority` (Protected: admin + moderator)
- **Handler:** `preEnrolmentHandler.List` (extend existing)
- **Purpose:** Filter leads where `high_priority_follow_up = true` (set explicitly by mentor_head on round close)
- **Change:** Add filter parameter to existing `GetAllLeads()` query
- **Note:** Filter is read-only; does NOT auto-set the flag (only mentor_head close-round sets it)

#### `GET /pre-enrolment/{leadID}?tab=class` (Protected: admin + moderator)
- **Handler:** `preEnrolmentHandler.Detail` (extend existing)
- **Purpose:** Show active class view (sessions, attendance, grades, notes) as a tab/section
- **Data:** Current class assignment, session timeline, attendance history, grade, student notes, community officer feedback
- **Note:** Extends existing detail page; could be a tab toggle rather than separate route

---

## 4) UI PAGES

### New Pages

#### Mentor Head Dashboard (`/mentor-head`)
- **Template:** `mentor_head.html` → `mentor_head_content`
- **Layout:** Uses main app layout (with sidebar)
- **Content:**
  - Header: "Mentor Head Dashboard"
  - List of classes where `sent_to_mentor = true`:
    - Class card: Level, Days, Time, Class #, Student count, Readiness
    - "Assign Mentor" button → opens modal with mentor dropdown + conflict warning
    - "Return to Operations" button
    - "View Schedule Conflicts" link
  - Mentor assignment modal:
    - Dropdown: Select mentor (from `users` table where `role = 'mentor'`)
    - Conflict warning: "This mentor has overlapping sessions on [dates]. Proceed anyway?"
    - "Assign" button
  - Actions:
    - Assign mentor → POST `/mentor-head/assign`
    - Return class → POST `/mentor-head/return/{classKey}`
    - Cancel session → opens modal → POST `/mentor-head/session/cancel`
    - Close round → POST `/mentor-head/close-round` (with confirmation)

#### Mentor Dashboard (`/mentor`)
- **Template:** `mentor.html` → `mentor_content`
- **Layout:** Uses main app layout (with sidebar)
- **Content:**
  - Header: "My Classes"
  - List of assigned classes (from `mentor_assignments` where `mentor_user_id = current_user.id`):
    - Class card: Level, Days, Time, Class #, Next session info
    - "View Class" button → GET `/mentor/class/{classKey}`
  - Actions:
    - View class detail → GET `/mentor/class/{classKey}`

#### Mentor Class Detail (`/mentor/class/{classKey}`)
- **Template:** `mentor_class_detail.html` → `mentor_class_detail_content`
- **Layout:** Uses main app layout (with sidebar)
- **Content:**
  - Header: Class info (Level X, Days, Time, Class #)
  - Session Timeline (8 sessions):
    - Each session card: Session #, Scheduled date/time, Status badge, "Mark Complete" button
    - For completed sessions: Show actual date/time
  - Students List:
    - Per student: Name, phone, attendance checkboxes (per session), "Add Note" button
    - Attendance entry: Checkbox per session (present/absent), notes field
    - Grade entry (session 8 only): Dropdown (A/B/C/F), notes
  - Actions:
    - Mark attendance → POST `/mentor/attendance` (AJAX or form submit)
    - Enter grade → POST `/mentor/grade` (session 8 only)
    - Add note → POST `/mentor/note`
    - Complete session → POST `/mentor/session/complete`

#### Community Officer Dashboard (`/community-officer`)
- **Template:** `community_officer.html` → `community_officer_content`
- **Layout:** Uses main app layout (with sidebar)
- **Content:**
  - Header: "Community Officer Dashboard"
  - Pending Feedback List:
    - Students who completed session 4 or 8 but no feedback logged
    - "Submit Feedback" button → opens feedback form
  - Absence Follow-up List:
    - Students with absences (from `attendance` where `attended = false`)
    - "Log Follow-up" button → opens follow-up form
  - Actions:
    - Submit feedback → POST `/community-officer/feedback`
    - Log follow-up → POST `/community-officer/follow-up`

### Modified Pages

#### Classes Board (`/classes`) — Add Active Classes Tab
- **Current:** Shows `ready_to_start` students grouped by level/days/time
- **Addition:** Add tab/filter "Active Classes" → shows `in_classes` students
- **New Section in Template:**
  - Tabs: "Pre-Start" (current view) | "Active Classes" (new)
  - Active Classes tab shows:
    - Classes with `status = 'in_classes'`
    - Session timeline (8 sessions per class)
    - Attendance summary per student
    - Community officer feedback status (pending/completed)
  - Actions:
    - View class sessions → expand inline or link to detail
    - Filter by round (if multiple active rounds exist)

#### Pre-Enrolment Detail (`/pre-enrolment/{leadID}`) — Add Class Tab
- **Current:** Shows lead info, placement test, offer, payments, schedule, etc.
- **Addition:** Add "Active Class" tab/section (only visible if `status = 'in_classes'`)
- **New Section in Template:**
  - Tabs: "Lead Info" (current) | "Active Class" (new, conditional)
  - Active Class tab shows:
    - Current class assignment (level, days, time, class #)
    - Session timeline with attendance status
    - Grade (if session 8 completed)
    - Student notes (all notes for this student)
    - Community officer feedback (session 4, 8)
  - **Read-only for moderators** (moderators cannot see this tab)

#### Pre-Enrolment List (`/pre-enrolment`) — Add Follow-Up Filter
- **Current:** Filter by status, payment, search, hot leads
- **Addition:** Add filter "High Priority Follow-Up" checkbox
- **Change:** When checked, show only leads where `high_priority_follow_up = true`
- **Visual:** Badge/indicator on leads that need follow-up

---

## 5) ACCEPTANCE SCENARIOS

### Scenario 1: Credit Consumption After Session 1
**Steps:**
1. Admin starts round → 8 sessions created for class
2. Mentor completes session 1 → POST `/mentor/session/complete`
3. System auto-increments `leads.levels_consumed += 1` for all students in class
4. Verify: `levels_consumed` increased, `levels_remaining = levels_purchased_total - levels_consumed` updated

**Acceptance:** Credit consumed automatically, no manual update needed.

### Scenario 2: Refund Blocked After Session 2 Completed
**Steps:**
1. Student has course payment of 3300 EGP
2. Round starts, session 1 completed (`completed_at` set), session 2 completed (`completed_at` set)
3. Admin tries to cancel lead with refund
4. System calculates: session 2 has `completed_at IS NOT NULL` → refundable amount = 0
5. Cancel modal shows: "No refund available. Session 2 has been completed."

**Acceptance:** Refund amount = 0, cancel proceeds without refund prompt. Uses `completed_at` marker, not wall-clock time.

### Scenario 3: Refund 50% After Session 1 Completed
**Steps:**
1. Student has course payment of 3300 EGP
2. Round starts, session 1 completed (`completed_at` set), session 2 NOT completed (`completed_at IS NULL`)
3. Admin tries to cancel lead with refund
4. System calculates: session 1 has `completed_at IS NOT NULL`, session 2 has `completed_at IS NULL` → max refund = 50% = 1650 EGP
5. Cancel modal shows: "Maximum refund: 1650 EGP (50% of course paid)"

**Acceptance:** Refund capped at 50%, validation enforces limit. Uses `completed_at` marker, not wall-clock time.

### Scenario 4: Mentor Conflict Blocked When Overlapping Sessions
**Steps:**
1. Mentor A assigned to Class 1 (Sun/Wed 07:30, sessions 1-8)
2. Mentor Head tries to assign Mentor A to Class 2 (Sun/Wed 07:30, sessions 1-8)
3. System detects overlap: same date/time intervals
4. Assignment blocked with error: "Mentor has conflicting session on [date] at [time]"

**Acceptance:** Assignment rejected, conflict clearly displayed.

### Scenario 5: Missed 3 Sessions Forces Repeat Even If Grade A
**Steps:**
1. Student completes all 8 sessions
2. Attendance: 5 present, 3 absent (sessions 2, 4, 6)
3. Mentor assigns grade A
4. Mentor Head closes round
5. System computes: absences = 3 > 2 → decision = REPEAT
6. Grade A is ignored (repeat rule takes precedence)

**Acceptance:** Student marked for repeat, grade A stored but not used for promotion.

### Scenario 6: Grade F Forces Repeat Regardless of Attendance
**Steps:**
1. Student completes all 8 sessions
2. Attendance: 8 present (perfect)
3. Mentor assigns grade F
4. Mentor Head closes round
5. System computes: grade = F → decision = REPEAT

**Acceptance:** Student marked for repeat despite perfect attendance.

### Scenario 7: No Remaining Credits Triggers High Priority Follow-Up
**Steps:**
1. Student has `levels_purchased_total = 2`, `levels_consumed = 2`
2. Round completes (session 8 done, credit consumed)
3. Mentor Head closes round
4. System checks: `levels_consumed >= levels_purchased_total` → sets `high_priority_follow_up = true`
5. Operations views `/pre-enrolment?follow_up=high_priority` → student appears in list

**Acceptance:** Student flagged for Operations follow-up, filter works.

### Scenario 8: Session Cancellation Reschedules Same Session
**Steps:**
1. Mentor Head cancels session 3 (scheduled for 2026-02-10 07:30)
2. Sets compensation: 2026-02-17 07:30
3. System: Updates session 3 `scheduled_date = 2026-02-17`, `scheduled_time = 07:30`, `status = 'scheduled'` (same session_number = 3)
4. Verify: Class still has exactly 8 sessions, session 3 rescheduled to new date/time

**Acceptance:** Same session rescheduled, no session 9 created. Round always has exactly 8 sessions.

### Scenario 9: Community Officer Feedback Required After Session 4 and 8
**Steps:**
1. Session 4 completed
2. Community Officer dashboard shows: "Pending feedback for [student] (Session 4)"
3. CO submits feedback: "Student progressing well, needs encouragement"
4. Session 8 completed
5. CO dashboard shows: "Pending feedback for [student] (Session 8)"
6. CO submits feedback: "Ready for next level"

**Acceptance:** Feedback prompts appear, submissions stored, UNIQUE constraint prevents duplicates.

### Scenario 10: Absence Follow-Up Logged Per Session
**Steps:**
1. Student absent in session 3 (attendance marked `attended = false`)
2. Community Officer logs follow-up:
   - Message sent: true
   - Reason: "No response to WhatsApp"
   - Student reply: "Will attend next session"
   - Action taken: "Reminder sent"
3. Verify: `absence_follow_up_logs` record created with session_id link

**Acceptance:** Follow-up logged, linked to specific session, visible in student history.

### Scenario 11: Moderator Cannot Access Mentor/Mentor Head Sections
**Steps:**
1. Moderator logs in
2. Moderator tries to access `/mentor-head` or `/mentor`
3. System returns 403 access-restricted page (same pattern as `/classes` and `/finance`)
4. Message: "Access Restricted: This section is available to [mentor_head/mentor] only."

**Acceptance:** Moderator sees friendly 403, cannot access mentor features.

### Scenario 12: Student Notes Carry Over Between Sessions
**Steps:**
1. Mentor adds note in session 2: "Student struggles with pronunciation"
2. Session 3: Different mentor views class detail
3. Previous note visible in "Student Notes" section
4. New mentor adds note: "Pronunciation improving, continue practice"
5. Both notes visible in chronological order

**Acceptance:** Notes persist, visible to all mentors, chronological display.

### Scenario 13: Round Close Computes Outcomes Correctly
**Steps:**
1. Class has 6 students
2. Student A: 7 present, 1 absent, grade B → PROMOTE
3. Student B: 5 present, 3 absent, grade A → REPEAT (absences > 2)
4. Student C: 8 present, 0 absent, grade F → REPEAT (grade F)
5. Student D: 6 present, 2 absent, grade C → PROMOTE
6. Mentor Head closes round
7. System computes outcomes, updates lead statuses (or adds outcome flags)

**Acceptance:** Outcomes computed correctly, repeat/promote decisions stored.

### Scenario 14: Refund Rule Uses Completion Markers Not Wall-Clock Time
**Steps:**
1. Session 2 scheduled for 2026-02-10 07:30
2. Current time: 2026-02-10 08:00:00 (session time has passed)
3. Session 2 NOT yet completed by mentor (status = 'scheduled', `completed_at IS NULL`)
4. Admin tries to cancel lead with refund
5. System checks: session 2 `completed_at IS NULL` → refundable = 50% (session 1 completed, session 2 not completed)
6. Even though wall-clock time passed, refund still available because session not marked completed

**Acceptance:** Refund rule uses `completed_at` markers, not wall-clock time. Cancellations/reschedules don't incorrectly block refunds.

### Scenario 15: Multiple Mentors, No Conflicts
**Steps:**
1. Mentor A assigned to Class 1 (Sun/Wed 07:30)
2. Mentor B assigned to Class 2 (Sun/Wed 07:30) — same time slot
3. System allows (different mentors, no conflict)
4. Mentor Head tries to assign Mentor A to Class 2 → blocked (Mentor A already has Class 1 at same time)

**Acceptance:** Same-time classes allowed for different mentors, same mentor blocked.

---

## REPO MISMATCHES (Snapshot vs Reality)

**None identified.** The snapshot accurately reflects:
- Table structures match migrations
- Route patterns match `main.go`
- Handler function names match actual code
- Status enum values match DB constraints
- Refund flow matches implementation

**One clarification:**
- Snapshot mentions `scheduling.class_time` as `TIME` type, which is correct
- Current code uses `UpsertSchedulingClassDaysTime()` which stores `class_time` as string, then casts to TIME in SQL
- This is acceptable; no mismatch, just implementation detail

---

## IMPLEMENTATION NOTES

### Session Date/Time Calculation
- **Pattern:** Weekly sessions (every 7 days)
- **Formula:** `session_N.scheduled_date = start_date + (N - 1) * 7 days`
- **Time:** All sessions use same `scheduled_time` (from `scheduling.class_time`)
- **Duration:** Default 2 hours (configurable via settings or hardcoded)

### Credit Consumption Logic
- **Trigger:** When `class_sessions.status` changes from `'scheduled'` to `'completed'` AND `session_number = 1`
- **Action:** `UPDATE leads SET levels_consumed = levels_consumed + 1 WHERE id IN (SELECT lead_id FROM scheduling WHERE class_key = $1)`
- **Validation:** Before allowing session 1, check `levels_consumed < levels_purchased_total`

### Refund Policy Integration
- **Modify:** `GetTotalCoursePaid()` or create `GetRefundableAmount(leadID)` that:
  1. Gets `total_course_paid` (existing logic)
  2. Queries `class_sessions` for lead's class:
     - If session 2 has `completed_at IS NOT NULL`: return 0 (session 2 completed)
     - If session 1 has `completed_at IS NOT NULL` AND session 2 has `completed_at IS NULL`: return `total_course_paid * 0.5`
     - Otherwise: return `total_course_paid`
  3. **Use `completed_at` markers, NOT wall-clock time** (prevents incorrect blocking on cancellations/reschedules)
- **Use in:** `CreateRefund()`, `CreateCancelRefundIdempotent()`, cancel modal validation

### Mentor Conflict Detection
- **Query:** When assigning mentor, check:
  ```sql
  SELECT COUNT(*) FROM class_sessions cs
  INNER JOIN mentor_assignments ma ON cs.class_key = ma.class_key
  WHERE ma.mentor_user_id = $1
  AND cs.scheduled_date = $2
  AND (
    (cs.scheduled_time <= $3 AND cs.scheduled_end_time > $3) OR
    (cs.scheduled_time < $4 AND cs.scheduled_end_time >= $4) OR
    (cs.scheduled_time >= $3 AND cs.scheduled_end_time <= $4)
  )
  AND cs.status != 'cancelled'
  ```
- **Block if:** COUNT > 0 (overlap detected)
- **Note:** Uses `mentor_user_id` (references `users.id`), not separate `mentors` table

### Round Close Outcome Storage
- **Option A:** Add `round_outcome` table (lead_id, class_key, round_number, decision, absences, grade)
- **Option B:** Add columns to `leads`: `last_round_outcome` (TEXT: 'promote'/'repeat'), `last_round_absences` (INTEGER)
- **Recommendation:** Option B (simpler, aligns with existing lead-centric model)

### Timezone Handling
- **Current:** Server uses local timezone (no explicit UTC conversion)
- **Recommendation:** 
  - Store session times in UTC in DB (`TIMESTAMP WITH TIME ZONE`)
  - Display in local timezone in UI (use Go's `time` package location conversion)
  - Add timezone setting to `settings` table (key='timezone', value='Africa/Cairo' or similar)

---

---

## 6) BUILD ORDER SEQUENCE

### Phase 1: Database Migrations
1. **Migration 014:** Add mentor roles to `users.role` enum (`mentor_head`, `mentor`)
2. **Migration 015:** Add `high_priority_follow_up` column to `leads`
3. **Migration 016:** Create `class_sessions` table (with `completed_at` column)
4. **Migration 017:** Create `attendance` table
5. **Migration 018:** Create `grades` table
6. **Migration 019:** Create `student_notes` table
7. **Migration 020:** Create `community_officer_feedback` table
8. **Migration 021:** Create `absence_follow_up_logs` table
9. **Migration 022:** Create `mentor_assignments` table (with `mentor_user_id` referencing `users.id`)

### Phase 2: Backend Models & Repository
1. **Models:** Add structs for `ClassSession`, `Attendance`, `Grade`, `StudentNote`, `CommunityOfficerFeedback`, `AbsenceFollowUpLog`, `MentorAssignment`
2. **Repository Functions:**
   - `CreateClassSessions(classKey, startDate, startTime)` — creates 8 sessions
   - `GetClassSessions(classKey)` — fetch sessions for a class
   - `CompleteSession(sessionID)` — mark session completed, set `completed_at`, handle credit consumption
   - `CancelAndRescheduleSession(sessionID, newDate, newTime)` — cancel and reschedule same session_number
   - `MarkAttendance(sessionID, leadID, attended, notes)`
   - `EnterGrade(leadID, classKey, grade, notes)`
   - `AddStudentNote(leadID, classKey, sessionNumber, noteText)`
   - `GetRefundableAmount(leadID)` — uses `completed_at` markers
   - `AssignMentorToClass(classKey, mentorUserID)` — with conflict check
   - `CheckMentorScheduleConflict(mentorUserID, date, time, duration)` — overlap detection
   - `CloseRound(classKey)` — compute outcomes, set `high_priority_follow_up`
   - `GetMentorClasses(mentorUserID)` — fetch assigned classes
   - `GetPendingFeedback(sessionNumber)` — for community officer
   - `SubmitFeedback(leadID, classKey, sessionNumber, feedbackText)`
   - `LogAbsenceFollowUp(leadID, sessionID, messageSent, reason, studentReply, actionTaken)`

### Phase 3: Backend Handlers
1. **Mentor Head Handler:**
   - `Dashboard` (GET `/mentor-head`)
   - `AssignMentor` (POST `/mentor-head/assign`)
   - `ReturnClass` (POST `/mentor-head/return/{classKey}`)
   - `CancelSession` (POST `/mentor-head/session/cancel`) — reschedule same session
   - `CloseRound` (POST `/mentor-head/close-round`)
2. **Mentor Handler:**
   - `Dashboard` (GET `/mentor`)
   - `ClassDetail` (GET `/mentor/class/{classKey}`)
   - `MarkAttendance` (POST `/mentor/attendance`)
   - `EnterGrade` (POST `/mentor/grade`)
   - `AddNote` (POST `/mentor/note`)
   - `CompleteSession` (POST `/mentor/session/complete`)
3. **Community Officer Handler:**
   - `Dashboard` (GET `/community-officer`)
   - `SubmitFeedback` (POST `/community-officer/feedback`)
   - `LogFollowUp` (POST `/community-officer/follow-up`)
4. **Classes Handler (Extended):**
   - Modify `StartRound()` to call `CreateClassSessions()` for each class
   - Add `ActiveClasses` (GET `/classes/active`) or extend existing `List` with tab
5. **Pre-Enrolment Handler (Extended):**
   - Extend `List` to support `?follow_up=high_priority` filter
   - Extend `Detail` to show "Active Class" tab (if `status = 'in_classes'`)
6. **Finance Handler (Extended):**
   - Modify `CreateRefund()` and cancel flow to use `GetRefundableAmount()` (session-based refund rules)

### Phase 4: Middleware & Access Control
1. Add `RequireMentorHead` middleware
2. Add `RequireMentor` middleware
3. Add `RequireCommunityOfficer` middleware
4. Update `main.go` routes with new middleware
5. Add role helper functions: `IsMentorHead(r)`, `IsMentor(r)`, `IsCommunityOfficer(r)`

### Phase 5: UI Templates
1. **New Templates:**
   - `mentor_head.html` (dashboard, assign mentor modal, cancel/reschedule session modal)
   - `mentor.html` (dashboard)
   - `mentor_class_detail.html` (8 sessions timeline, attendance entry, grade entry, notes)
   - `community_officer.html` (pending feedback, absence follow-up)
2. **Modified Templates:**
   - `classes.html` — add "Active Classes" tab
   - `pre_enrolment_list.html` — add "High Priority Follow-Up" filter checkbox
   - `pre_enrolment_detail.html` — add "Active Class" tab (conditional, hidden for moderators)
   - `layout.html` — add nav links for mentor_head, mentor, community_officer (role-based visibility)
   - `access_restricted.html` — extend for mentor_head/mentor/community_officer access messages

### Phase 6: Integration & Testing
1. **Test Credit Consumption:**
   - Start round → verify sessions created
   - Complete session 1 → verify `levels_consumed` incremented
2. **Test Refund Rules:**
   - Session 1 completed, session 2 not completed → verify 50% refund
   - Session 2 completed → verify 0% refund
   - Test with cancelled/rescheduled sessions → verify uses `completed_at`, not wall-clock
3. **Test Mentor Conflicts:**
   - Assign mentor to overlapping sessions → verify blocked
   - Assign different mentors to same time → verify allowed
4. **Test Session Cancellation/Reschedule:**
   - Cancel session 3, reschedule → verify same session_number, no session 9
5. **Test Round Close:**
   - Close round → verify outcomes computed, `high_priority_follow_up` set for no-credit students
6. **Test Follow-Up Filter:**
   - View `/pre-enrolment?follow_up=high_priority` → verify only flagged students shown
7. **Test Access Control:**
   - Moderator tries `/mentor-head`, `/mentor` → verify 403 access-restricted page
8. **Test Attendance & Grades:**
   - Mark attendance for multiple sessions → verify persisted
   - Enter grade at session 8 → verify stored
   - Verify missed > 2 sessions forces repeat even with grade A
9. **Test Community Officer Workflow:**
   - Complete session 4 → verify feedback prompt appears
   - Submit feedback → verify stored
   - Log absence follow-up → verify linked to session

---

## 7) CORRECTED SUMMARY (A-E)

### A) Updated DB Changes List

**New Tables (7):**
1. `class_sessions` — 8 sessions per class, includes `completed_at` timestamp for refund rules
2. `attendance` — per-session, per-student attendance records
3. `grades` — grade A/B/C/F at session 8 only
4. `student_notes` — carry-over notes per student
5. `community_officer_feedback` — feedback at sessions 4 and 8
6. `absence_follow_up_logs` — absence follow-up tracking
7. `mentor_assignments` — links `mentor_user_id` (references `users.id`) to `class_key`

**Modified Tables (2):**
1. `users` — add roles: `'mentor_head'`, `'mentor'` (no separate `mentors` table)
2. `leads` — add `high_priority_follow_up` boolean (only set by mentor_head on round close)

**Key Constraints:**
- `class_sessions`: `UNIQUE (class_key, session_number)` — exactly 8 sessions, no session 9
- `mentor_assignments`: `UNIQUE (class_key)` — one mentor per class
- `mentor_assignments.mentor_user_id` → `users.id` (role='mentor')

**Key Indexes:**
- `idx_class_sessions_completed_at` — for refund rule queries
- `idx_mentor_assignments_mentor_user_id` — for mentor conflict detection

### B) Updated Endpoints List

**Mentor Head (5 endpoints):**
- `GET /mentor-head` — dashboard
- `POST /mentor-head/assign` — assign mentor (uses `mentor_user_id`, not `mentor_id`)
- `POST /mentor-head/return/{classKey}` — return to Operations
- `POST /mentor-head/session/cancel` — cancel and reschedule same session_number (no session 9)
- `POST /mentor-head/close-round` — compute outcomes, set `high_priority_follow_up` explicitly

**Mentor (6 endpoints):**
- `GET /mentor` — dashboard (uses `mentor_user_id = current_user.id`)
- `GET /mentor/class/{classKey}` — class detail
- `POST /mentor/attendance` — mark attendance
- `POST /mentor/grade` — enter grade (session 8)
- `POST /mentor/note` — add student note
- `POST /mentor/session/complete` — complete session, set `completed_at` timestamp

**Community Officer (3 endpoints):**
- `GET /community-officer` — dashboard
- `POST /community-officer/feedback` — submit feedback (sessions 4, 8)
- `POST /community-officer/follow-up` — log absence follow-up

**Extended (2 endpoints):**
- `GET /pre-enrolment?follow_up=high_priority` — filter (read-only, does not set flag)
- `GET /pre-enrolment/{leadID}?tab=class` — active class view tab

### C) Updated UI Changes List

**New Pages (4):**
1. Mentor Head Dashboard — assign mentors, cancel/reschedule sessions, close round
2. Mentor Dashboard — list assigned classes
3. Mentor Class Detail — 8 sessions timeline, attendance entry, grade entry, notes
4. Community Officer Dashboard — pending feedback, absence follow-up

**Modified Pages (3):**
1. Classes Board — add "Active Classes" tab (shows `in_classes` students with sessions)
2. Pre-Enrolment List — add "High Priority Follow-Up" filter checkbox
3. Pre-Enrolment Detail — add "Active Class" tab (conditional, hidden for moderators)

**Key UI Notes:**
- Mentor assignment dropdown uses `users` table (role='mentor'), not separate `mentors` table
- Session cancellation UI shows reschedule form (same session_number), not "create session 9"
- Follow-up filter is read-only (does not auto-set flag)

### D) Updated Acceptance Scenarios (12+)

1. **Credit Consumption After Session 1** — auto-increment `levels_consumed` when session 1 completed
2. **Refund Blocked After Session 2 Completed** — uses `completed_at` marker, not wall-clock time
3. **Refund 50% After Session 1 Completed** — uses `completed_at` marker, session 2 not completed
4. **Mentor Conflict Blocked** — overlapping sessions for same mentor blocked
5. **Missed 3 Sessions Forces Repeat** — even with grade A
6. **Grade F Forces Repeat** — regardless of attendance
7. **No Remaining Credits Triggers Follow-Up** — set explicitly by mentor_head on round close
8. **Session Cancellation Reschedules Same Session** — no session 9, always exactly 8 sessions
9. **Community Officer Feedback Required** — sessions 4 and 8
10. **Absence Follow-Up Logged Per Session** — linked to specific session
11. **Moderator Cannot Access Mentor Sections** — 403 access-restricted page
12. **Student Notes Carry Over** — visible to all mentors, chronological
13. **Round Close Computes Outcomes** — repeat/promote decisions
14. **Refund Rule Uses Completion Markers** — not wall-clock time (cancellations don't block refunds incorrectly)

### E) Build Order Sequence

**Phase 1: Migrations (9 files)**
- 014: Add mentor roles
- 015: Add follow-up flag
- 016-022: Create 7 new tables

**Phase 2: Backend Models & Repository**
- Structs for all new entities
- Repository functions (session creation, completion, refund calculation, mentor assignment, round close, etc.)

**Phase 3: Backend Handlers**
- Mentor Head, Mentor, Community Officer handlers
- Extended Classes and Pre-Enrolment handlers
- Modified Finance handler (refund rules)

**Phase 4: Middleware & Access Control**
- Role-based middleware
- Route protection

**Phase 5: UI Templates**
- 4 new templates
- 3 modified templates

**Phase 6: Integration & Testing**
- Credit consumption, refund rules, mentor conflicts, session reschedule, round close, follow-up filter, access control, attendance/grades, community officer workflow

---

**READY FOR BUILD PROMPT 2**
