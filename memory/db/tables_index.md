# Tables Index

**Purpose**: Quick reference for what each table does and which features use it.

**Evidence Sources**: Migrations + handler code

---

## Core Tables

### `users`
**Purpose**: System users with role-based access  
**Features**: Authentication, authorization, audit trails  
**Key Fields**: email (login), password_hash, role  
**Evidence**: `migrations/001_init.sql:4-10`

### `leads`
**Purpose**: Student pipeline from lead creation → in classes  
**Features**: Pre-enrolment workflow, class assignments  
**Key Fields**: full_name, phone (unique), status (pipeline state), offer_sent_at (cold-lead timing anchor)  
**Evidence**: `migrations/001_init.sql:13-26`

### ` settings`
**Purpose**: System-wide configuration (key-value store)  
**Features**: Current round tracking  
**Key Fields**: key (PK), value  
**Evidence**: `migrations/004_classes_board.sql:12-16`

---

## Pre-Enrolment Tables (lead_id FK → leads)

All tables have UNIQUE `lead_id` - one record per lead.

| Table | Purpose | Features Using It |
|-------|---------|------------------|
| `placement_tests` | Test scheduling & results | Pre-enrolment workflow, level assignment |
| `offers` | Bundle pricing & discounts | Pre-enrolment workflow |
| `bookings` | Book delivery details | Pre-enrolment workflow |
| `payments` | Payment tracking (full/deposit) | Pre-enrolment workflow, finance |
| `scheduling` | Class schedule assignment | Pre-enrolment workflow, class assignment |
| `shipping` | Book shipment tracking | Pre-enrolment workflow |

**Evidence**: `migrations/001_init.sql`

---

## Classes & Mentors Tables

### `class_groups`
**Purpose**: Represents a class (group of students in same level/days/time)  
**Features**: Classes board, mentor head dashboard, mentor workflow  
**Key Fields**: class_key (PK), level, class_days, class_time, sent_to_mentor, round_status  
**Used By**: Admin (ops), Mentor Head (assignment), Mentor (teaching)  
**Evidence**: `migrations/005_class_groups_workflow.sql`

### `mentor_assignments`
**Purpose**: Links mentors (users) to classes  
**Features**: Mentor head dashboard, mentor workflow  
**Key Fields**: mentor_user_id FK → users(id), class_key FK → class_groups(class_key) UNIQUE  
**Constraint**: One mentor per class  
**Evidence**: `migrations/022_create_mentor_assignments.sql`

### `class_sessions`
**Purpose**: Individual class sessions (8 per class)  
**Features**: Mentor workflow (session completion), attendance tracking  
**Key Fields**: class_key, session_number, status, completed_at  
**Evidence**: `migrations/016_create_class_sessions.sql`

### `attendance`
**Purpose**: Tracks attendance per student per session  
**Features**: Mentor workflow, student success absence feed  
**Key Fields**: session_id, lead_id, status (present/absent_excused/absent_unexcused)  
**Evidence**: `migrations/017_create_attendance.sql`

### `late_joiner_notifications`
**Purpose**: Tracks awareness alerts for mentors/heads when students join mid-round  
**Features**: Dashboard notification banner  
**Key Fields**: lead_id, user_id, class_key, acknowledged_at (NULL = pending)  
**Evidence**: `migrations/044_late_joiner_notifications.sql`

---

## Student Success Tables

### `followups`
**Purpose**: Dual-purpose table for absence escalations AND complaints  
**Features**: Student Success absence workflow, Complaints workflow  
**Key Fields**: type (absence_escalation | complaint), class_key, lead_id, session_number (nullable for complaints), status  
**Evidence**: `migrations/029_create_followups_table.sql`, `migrations/033_add_complaints_to_followups.sql`

**Types**:
-`type = 'absence_escalation'`: Student missed session, SS creates follow-up, tracks contact attempts  
- `type = 'complaint'`: Student complaint, SS creates, Mentor Head handles

### `followup_case_notes`
**Purpose**: Tracks updates/resolutions for follow-ups  
**Features**: Student Success absence workflow, Complaints workflow  
**Key Fields**: followup_id, note_type (status_update | resolution), new_status, resolution_note  
**Evidence**: `migrations/034_create_followup_case_notes.sql`

---

## Finance Tables

### `lead_payments` 
**Purpose**: Payment tracking linked to leads  
**Features**: Finance dashboard, pre-enrolment workflow  
**Evidence**: `migrations/001_init.sql:65-73` (as `payments`)

**Note**: Finance module has additional tables not fully documented here (see migrations/007_finance_tracking.sql, 008_finance_ledger_sync.sql)

### `payment_cycles`
**Purpose**: Explicit cycle boundary for returning-student entitlement and refunds  
**Features**: Cancel refund guard, current-cycle cash calculation, carryover valuation  
**Key Fields**: lead_id, started_at, bundle_levels, final_price, consumed_baseline, status  
**Rule**: At most one active cycle per lead

---

## Other Tables

### `student_notes`
**Purpose**: Notes about individual students  
**Features**: Class workspace (mentor/mentor_head/student_success view)  
**Evidence**: `migrations/019_create_student_notes.sql`

### `grades`
**Purpose**: Student grades tracking  
**Features**: Class workspace  
**Evidence**: `migrations/018_create_grades.sql`

### `community_officer_feedback`
**Purpose**: Community officer feedback forms  
**Features**: Community officer workflow  
**Evidence**: `migrations/020_create_community_officer_feedback.sql`

### `mentor_evaluations`
**Purpose**: Mentor performance evaluations  
**Features**: Mentor Head evaluations page  
**Evidence**: `migrations/025_create_mentor_evaluations.sql`

---

## Table Feature Matrix

| Feature | Core Tables |
|---------|-------------|
| **Pre-Enrolment Workflow** | leads, placement_tests, offers, bookings, payments, scheduling, shipping |
| **Classes Board (Admin)** | class_groups, leads (status transitions), scheduling |
| **Mentor Head Dashboard** | class_groups, mentor_assignments, users (mentors) |
| **Mentor Workflow** | class_groups, mentor_assignments, class_sessions, attendance, student_notes, leads |
| **Student Success Absence** | attendance, followups (type=absence_escalation), followup_case_notes, leads |
| **Complaints Workflow** | followups (type=complaint), followup_case_notes, leads |
| **Finance** | payments, (other finance tables) |
| **Mentor Evaluations** | mentor_evaluations, users (mentors) |

---

## Soft Delete Support

Only `followups` table has soft delete support:
- `deleted_at` timestamp
- `deleted_by_user_id` FK
- `delete_reason` text

**Evidence**: `migrations/033_add_complaints_to_followups.sql:14-16`

**Note**: This was added for manager role's delete complaint feature (now removed), but schema remains.
