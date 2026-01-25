# System Spec Snapshot — Milestone 1 (Pre-Enrolment + Finance + RBAC)

**Date:** 2026-01-25  
**Purpose:** Complete system specification before implementing Milestone 2 (Active Classes)  
**Scope:** All implemented features, data models, workflows, and identified gaps

---

## 1) CURRENT FEATURES (Milestone 1 Reality)

### Frontend Pages + Routes

#### `/login` (Public)
- **GET:** Login form (`authHandler.LoginForm`)
- **POST:** Authenticate (`authHandler.Login`)
- **Actions:** Login button → POST to `/login`

#### `/logout` (Protected)
- **GET/POST:** Logout (`authHandler.Logout`)
- **Actions:** Logout button → clears session, redirects to `/login`

#### `/pre-enrolment` (Protected: Admin + Moderator)
- **GET:** List all leads (`preEnrolmentHandler.List`)
- **Actions:**
  - **"New Lead"** button → GET `/pre-enrolment/new`
  - **"Open"** button → GET `/pre-enrolment/{leadID}`
  - **"Delete"** button (Admin only, hidden for Moderator) → POST `/pre-enrolment/{leadID}` with `action=delete`
  - **"Send to Classes"** button (Admin only, hidden for Moderator) → POST `/pre-enrolment/{leadID}` with `action=send_to_classes` → sets `sent_to_classes=true`, status remains `ready_to_start`
  - Filter/search controls → GET `/pre-enrolment?status=...&search=...&payment=...&hot=1&include_cancelled=1`

#### `/pre-enrolment/new` (Protected: Admin + Moderator)
- **GET:** New lead form (`preEnrolmentHandler.NewForm`)
- **POST:** Create lead (`preEnrolmentHandler.Create`)
- **Actions:**
  - **"Save"** button → POST `/pre-enrolment/new`
  - On phone duplicate error: shows "Open existing lead" link → GET `/pre-enrolment/{existingLeadID}`

#### `/pre-enrolment/{leadID}` (Protected: Admin + Moderator)
- **GET:** Lead detail page (`preEnrolmentHandler.Detail`)
- **POST:** Update lead (`preEnrolmentHandler.Update` with `action` parameter)
- **Actions (Admin only):**
  - **"Save"** → POST with `action=save` (or empty) → `SaveFull`
  - **"Mark Test Booked"** → POST with `action=mark_test_booked`
  - **"Mark Tested"** → POST with `action=mark_tested`
  - **"Mark Offer Sent"** → POST with `action=mark_offer_sent`
  - **"Move to Waiting List"** → POST with `action=move_waiting`
  - **"Mark Ready to Start"** → POST with `action=mark_ready`
  - **"Send to Classes"** → POST with `action=send_to_classes` → sets `sent_to_classes=true` (status remains `ready_to_start`) → sets `sent_to_classes=true` (status remains `ready_to_start`)
  - **"Cancel Lead"** → GET with `?action=cancel` (shows modal), then POST with `action=cancel` + refund fields
  - **"Reopen Lead"** → POST with `action=reopen` (only if status=cancelled)
  - **"Create Refund"** (in Refund section) → POST `/finance/refund/{leadID}`
- **Actions (Moderator only):**
  - **"Save"** → POST with `action=save` → only updates `full_name`, `phone`, `source`, `notes` (basic fields only)
  - All other actions blocked (403 Forbidden or hidden in UI)

#### `/classes` (Protected: Admin + Moderator)
- **GET:** Classes board (`classesHandler.List`)
- **Moderator:** Gets custom 403 access-restricted page (styled HTML, "Back to Pre-Enrolment" link)
- **Admin Actions:**
  - **"Start Round"** button → POST `/classes/start-round`
  - **"Send to Mentor Head"** button (per class card) → POST `/classes/send` with `class_key`, `level`, `class_days`, `class_time`, `class_number`
  - **"Return"** button (if sent) → POST `/classes/{classKey}/return`
  - **"Move to..."** dropdown (per student) → POST `/classes/move` with `lead_id`, `target_group`
  - **"Open"** link (per student) → GET `/pre-enrolment/{leadID}`

#### `/finance` (Protected: Admin + Moderator)
- **GET:** Finance dashboard (`financeHandler.Dashboard`)
- **Moderator:** Gets custom 403 access-restricted page (styled HTML, "Back to Pre-Enrolment" link)
- **Admin Actions:**
  - Filter controls (date range, category, payment method, transaction type) → GET `/finance?date_from=...&date_to=...&category=...&payment_method=...&transaction_type=...`
  - **"New Expense"** link → GET `/finance/new-expense`

#### `/finance/new-expense` (Protected: Admin only)
- **GET:** New expense form (`financeHandler.NewExpenseForm`)
- **POST:** Create expense (`financeHandler.CreateExpense`)
- **Actions:**
  - **"Create Expense"** button → POST `/finance/new-expense`

#### `/finance/refund/{leadID}` (Protected: Admin only)
- **POST:** Create refund (`financeHandler.CreateRefund`)
- **Called from:** Pre-enrolment detail page "Create Refund" form

#### `/` (Root, Protected)
- **GET:** Redirects to `/pre-enrolment` (`middleware.RequireAuth`)

### Role Permissions Summary

| Feature | Admin | Moderator |
|---------|-------|-----------|
| View leads list | ✅ | ✅ |
| Create new lead | ✅ | ✅ |
| View lead detail | ✅ | ✅ |
| Edit basic info (name, phone, source, notes) | ✅ | ✅ |
| Edit placement test | ✅ | ❌ |
| Edit offer/pricing | ✅ | ❌ |
| Edit booking/materials | ✅ | ❌ |
| Add course payments | ✅ | ❌ |
| Edit schedule | ✅ | ❌ |
| Update lead status (mark tested, offer sent, ready, etc.) | ✅ | ❌ |
| Cancel lead | ✅ | ❌ |
| Reopen lead | ✅ | ❌ |
| Delete lead | ✅ | ❌ |
| Send to classes | ✅ | ❌ |
| View Classes board | ✅ | ❌ (403 access-restricted) |
| Move students between groups | ✅ | ❌ |
| Start round | ✅ | ❌ |
| Send to mentor head | ✅ | ❌ |
| View Finance dashboard | ✅ | ❌ (403 access-restricted) |
| Create expense | ✅ | ❌ |
| Create refund | ✅ | ❌ |

---

## 2) DATA MODEL (DB)

### Tables

#### `users`
```sql
id UUID PRIMARY KEY
email TEXT UNIQUE NOT NULL
password_hash TEXT NOT NULL
role TEXT NOT NULL CHECK (role IN ('admin', 'moderator', 'community_officer'))
created_at TIMESTAMP WITH TIME ZONE
```
**Indexes:** `idx_users_email`

#### `leads`
```sql
id UUID PRIMARY KEY
full_name TEXT NOT NULL
phone TEXT UNIQUE NOT NULL
source TEXT
notes TEXT
status TEXT NOT NULL DEFAULT 'lead_created' CHECK (status IN (
    'lead_created', 'test_booked', 'tested', 'offer_sent', 'booking_confirmed',
    'paid_full', 'deposit_paid', 'waiting_for_round', 'schedule_assigned', 
    'ready_to_start', 'in_classes', 'cancelled', 'paused'
))
sent_to_classes BOOLEAN DEFAULT false
levels_purchased_total INTEGER DEFAULT 0
levels_consumed INTEGER DEFAULT 0
bundle_type TEXT CHECK (bundle_type IN ('none', 'single', 'bundle2', 'bundle3', 'bundle4'))
cancelled_at TIMESTAMP WITH TIME ZONE
created_by_user_id UUID REFERENCES users(id)
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
```
**Indexes:** `idx_leads_status`, `idx_leads_phone`, `idx_leads_created_at`, `idx_leads_sent_to_classes`, `idx_leads_levels_remaining`, `idx_leads_status_cancelled`, `idx_leads_status_paused`

#### `placement_tests`
```sql
id UUID PRIMARY KEY
lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE
test_date DATE
test_time TIME
test_type TEXT CHECK (test_type IN ('online', 'live'))
assigned_level INTEGER CHECK (assigned_level >= 1 AND assigned_level <= 8)
test_notes TEXT
run_by_user_id UUID REFERENCES users(id)
placement_test_fee INTEGER DEFAULT 100
placement_test_fee_paid INTEGER DEFAULT 0
placement_test_payment_date DATE
placement_test_payment_method TEXT CHECK (placement_test_payment_method IN ('vodafone_cash', 'bank_transfer', 'paypal', 'other'))
updated_at TIMESTAMP WITH TIME ZONE
```

#### `offers`
```sql
id UUID PRIMARY KEY
lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE
bundle_levels INTEGER CHECK (bundle_levels >= 1 AND bundle_levels <= 4)
base_price INTEGER
discount_value INTEGER
discount_type TEXT CHECK (discount_type IN ('amount', 'percent'))
final_price INTEGER
updated_at TIMESTAMP WITH TIME ZONE
```

#### `bookings`
```sql
id UUID PRIMARY KEY
lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE
book_format TEXT CHECK (book_format IN ('pdf', 'printed'))
address TEXT
city TEXT
delivery_notes TEXT
updated_at TIMESTAMP WITH TIME ZONE
```

#### `payments` (Legacy — deprecated, use `lead_payments` instead)
```sql
id UUID PRIMARY KEY
lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE
payment_type TEXT CHECK (payment_type IN ('full', 'deposit'))
amount_paid INTEGER DEFAULT 0
remaining_balance INTEGER DEFAULT 0
payment_date DATE
updated_at TIMESTAMP WITH TIME ZONE
```

#### `lead_payments` (Active — course payments)
```sql
id UUID PRIMARY KEY
lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE
kind TEXT NOT NULL DEFAULT 'course' CHECK (kind = 'course')
amount INTEGER NOT NULL CHECK (amount > 0)
payment_method TEXT NOT NULL CHECK (payment_method IN ('vodafone_cash', 'bank_transfer', 'paypal', 'other'))
payment_date DATE NOT NULL
notes TEXT
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
```
**Indexes:** `idx_lead_payments_lead_id`, `idx_lead_payments_date`

#### `scheduling`
```sql
id UUID PRIMARY KEY
lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE
expected_round TEXT
class_days TEXT
class_time TIME
start_date DATE
start_time TIME
class_group_index INTEGER
updated_at TIMESTAMP WITH TIME ZONE
```
**Indexes:** `idx_scheduling_class_group`

#### `shipping`
```sql
id UUID PRIMARY KEY
lead_id UUID UNIQUE REFERENCES leads(id) ON DELETE CASCADE
shipment_status TEXT CHECK (shipment_status IN ('pending', 'sent', 'delivered'))
shipment_date DATE
updated_at TIMESTAMP WITH TIME ZONE
```

#### `transactions` (Finance ledger)
```sql
id UUID PRIMARY KEY
transaction_date DATE NOT NULL DEFAULT CURRENT_DATE
transaction_type TEXT NOT NULL CHECK (transaction_type IN ('IN', 'OUT'))
category TEXT NOT NULL
amount INTEGER NOT NULL CHECK (amount > 0)
payment_method TEXT CHECK (payment_method IN ('vodafone_cash', 'bank_transfer', 'paypal', 'other'))
lead_id UUID REFERENCES leads(id) ON DELETE SET NULL
notes TEXT
source_key TEXT (deprecated, use ref_key)
ref_type TEXT
ref_id TEXT
ref_sub_type TEXT
ref_key TEXT UNIQUE (WHERE ref_key IS NOT NULL)
bundle_levels INTEGER
levels_purchased INTEGER
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
```
**Indexes:** `idx_transactions_date`, `idx_transactions_type`, `idx_transactions_category`, `idx_transactions_lead_id`, `idx_transactions_ref_id`, `idx_transactions_ref_sub_type`, `idx_transactions_ref_key_unique`

**Transaction Categories:**
- **IN:** `placement_test`, `course_payment`, `teacher_salary`, `ads`, `rent`, `software`, `moderator`, `content_creator`, `other`
- **OUT:** `refund`, `teacher_salary`, `ads`, `rent`, `software`, `moderator`, `content_creator`, `other`

#### `class_groups` (Classes board workflow)
```sql
class_key TEXT PRIMARY KEY
level INTEGER NOT NULL
class_days TEXT NOT NULL
class_time TEXT NOT NULL
class_number INTEGER NOT NULL
sent_to_mentor BOOLEAN DEFAULT false
sent_at TIMESTAMP WITH TIME ZONE
returned_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
```
**Indexes:** `idx_class_groups_key`

#### `settings`
```sql
key TEXT PRIMARY KEY
value TEXT NOT NULL
updated_at TIMESTAMP WITH TIME ZONE
```
**Current keys:** `current_round` (default: "1")

### Lead, Payment/Ledger, Offer, Bundle/Credits Representation

**Lead:**
- Stored in `leads` table
- `levels_purchased_total`: Total levels purchased (from bundle selection)
- `levels_consumed`: Levels consumed when rounds start (currently not auto-updated)
- `bundle_type`: `none`, `single`, `bundle2`, `bundle3`, `bundle4`
- `sent_to_classes`: Boolean flag indicating manual "Send to Classes" action

**Payment/Ledger:**
- **Course payments:** Stored in `lead_payments` table (kind='course')
- **Placement test payments:** Stored in `placement_tests.placement_test_fee_paid`
- **Ledger sync:** Every placement test payment (paid > 0) creates IN transaction (category='placement_test', ref_key="lead:{leadID}:placement_test")
- Every course payment creates IN transaction (category='course_payment', ref_key="lead:{leadID}:course_payment:{paymentID}")
- Every refund creates OUT transaction (category='refund', ref_key="lead:{leadID}:refund:{uuid}" or "cancel_refund:{leadID}:{date}:{amount}")

**Offer:**
- Stored in `offers` table (one per lead, UNIQUE on `lead_id`)
- `bundle_levels`: 1, 2, 3, or 4
- `base_price`: Auto-calculated from bundle (hardcoded: 1=1300, 2=2400, 3=3300, 4=4000 EGP)
- `discount_value` + `discount_type` (amount or percent)
- `final_price`: Calculated as base_price - discount, or manually set

**Bundle/Credits:**
- Bundle selection sets `offers.bundle_levels` (1-4)
- `leads.levels_purchased_total` tracks total levels purchased (from bundle)
- `leads.levels_consumed` tracks consumed levels (currently manual, not auto-updated on round start)
- `transactions.levels_purchased` field exists but not actively used

### Cancellations and Refunds

**Cancellation:**
- Soft delete: `leads.status = 'cancelled'`, `leads.cancelled_at` timestamp set
- `CancelLead()` function sets status and timestamp
- `ReopenLead()` sets status back to `'lead_created'` and clears `cancelled_at`

**Refunds:**
- Stored in `transactions` table (transaction_type='OUT', category='refund')
- **Validation:**
  - Refund amount ≤ `GetTotalCoursePaid(leadID)` (sum of `lead_payments` - sum of refunds)
  - Placement test fees are **not refundable** (only course payments can be refunded)
  - Transaction date must be ≤ today (no future dates)
- **Idempotency:**
  - Cancel flow: `CreateCancelRefundIdempotent()` uses `ref_key="cancel_refund:{leadID}:{date}:{amount}"` with `ON CONFLICT (ref_key) DO NOTHING`
  - Finance refund: `CreateRefund()` uses `ref_key="lead:{leadID}:refund:{uuid}"` (unique per refund)
- **Refund rules:** Currently no automatic refund policy (manual only). No session-based refund rules implemented.

---

## 3) LEAD LIFECYCLE (IMPLEMENTED)

### Lead Status Enum Values

```sql
'lead_created'        -- Initial state
'test_booked'         -- Placement test scheduled
'tested'              -- Test completed, level assigned
'offer_sent'          -- Offer (bundle + final price) sent
'booking_confirmed'   -- Legacy status (mapped to offer_sent or paid_full)
'paid_full'           -- Full payment received
'deposit_paid'        -- Partial payment received
'waiting_for_round'   -- Moved to waiting list (pre-start)
'schedule_assigned'   -- Class days/time set
'ready_to_start'      -- Fully paid + schedule set + level assigned
'in_classes'          -- Round started, student in active classes
'cancelled'           -- Soft cancelled
'paused'              -- Mid-round pause (on-hold)
```

### Status Transitions (Allowed)

| From | To | Validation | Who Can Do |
|------|----|-----------|------------|
| `lead_created` | `test_booked` | test_date, test_time, test_type required | Admin only |
| `test_booked` | `tested` | assigned_level or test_notes provided | Admin only |
| `tested` | `offer_sent` | bundle + final_price required | Admin only |
| `offer_sent` | `paid_full` | Auto: when `total_course_paid >= final_price` | Auto (via `UpdateLeadStatusFromPayment`) |
| `offer_sent` | `deposit_paid` | Auto: when `total_course_paid > 0` but `< final_price` | Auto (via `UpdateLeadStatusFromPayment`) |
| `paid_full` | `schedule_assigned` | Auto: when class_days + class_time set | Auto (via `ComputeStageFromFormCompletion`) |
| `schedule_assigned` | `ready_to_start` | Auto: when fully paid + schedule + level | Auto (via `ComputeStageFromFormCompletion`) |
| `ready_to_start` | `in_classes` | Manual: "Start Round" button | Admin only |
| Any (except cancelled) | `waiting_for_round` | No validation | Admin only |
| Any (except cancelled) | `cancelled` | If `total_course_paid > 0`, refund required | Admin only |
| `cancelled` | `lead_created` | Reopen action | Admin only |
| Any | `paused` | Not implemented in handlers (status exists in DB) | N/A |

**Note:** Status can also **downgrade** automatically:
- `paid_full` → `offer_sent` when refund reduces `total_course_paid < final_price` (via `UpdateLeadStatusFromPayment`)

### Waiting List

- **Status:** `waiting_for_round`
- **Transition:** "Move to Waiting List" button (action=`move_waiting`)
- **Allowed from:** Any status (except cancelled)
- **Behavior:** No refund created, payments unchanged, status set to `waiting_for_round`
- **Who can do:** Admin only

---

## 4) FINANCE INTEGRITY (IMPLEMENTED)

### Ledger Model

**Transaction Types:**
- **IN:** Income (placement_test, course_payment, teacher_salary, ads, rent, software, moderator, content_creator, other)
- **OUT:** Expenses/Refunds (refund, teacher_salary, ads, rent, software, moderator, content_creator, other)

**Categories:**
- `placement_test` (IN only)
- `course_payment` (IN only)
- `refund` (OUT only)
- `teacher_salary` (IN or OUT)
- `ads`, `rent`, `software`, `moderator`, `content_creator`, `other` (IN or OUT)

**Payment Methods:**
- `vodafone_cash`
- `bank_transfer`
- `paypal`
- `other`

**Daily Totals Computation:**
- `GetFinanceSummary()` groups transactions by date, computes:
  - `TodayIN`, `TodayOUT`, `TodayNet` (for today's date)
  - `RangeIN`, `RangeOUT`, `RangeNet` (for date range filter)
  - `INByCategory`, `OUTByCategory` (aggregated by category)
- `GetCurrentCashBalance()`: `SUM(IN) - SUM(OUT)` over all history (no date filter)
- `GetCurrentCashBalanceByPaymentMethod()`: Groups as Cash (vodafone_cash + other) vs Bank (bank_transfer + paypal)

**Ledger Integrity:**
- Every placement test payment syncs to IN transaction (idempotent via `ref_key="lead:{leadID}:placement_test"`)
- Every course payment syncs to IN transaction (`ref_key="lead:{leadID}:course_payment:{paymentID}"`)
- Every refund syncs to OUT transaction (`ref_key="lead:{leadID}:refund:{uuid}"` or idempotent cancel key)
- `ref_key` has UNIQUE constraint (WHERE ref_key IS NOT NULL) to prevent duplicates

### Current Refund Rules

**Enforced Today:**
1. Refund amount ≤ `GetTotalCoursePaid(leadID)` (net: payments - refunds)
2. Placement test fees are **not refundable** (only course payments)
3. Transaction date must be ≤ today (no future dates)
4. Payment method must be one of: vodafone_cash, bank_transfer, paypal, other
5. Cancel flow uses idempotent refund creation (retries don't double-create)

**Not Implemented:**
- Session-based refund policy (50% after session 1, no refund after session 2 starts)
- Automatic refund calculation based on sessions attended

---

## 5) CLASSES BOARD (CURRENT)

### Round

- **Storage:** `settings` table, key='current_round', value='1' (or higher)
- **Purpose:** Tracks which round/cycle classes are in
- **Actions:**
  - **"Start Round"** button → `StartRound()`:
    - Increments `current_round` by 1
    - Sets `leads.status = 'in_classes'` for all students in READY or LOCKED classes
    - NOT READY classes remain in `ready_to_start` status for next round
- **Current behavior:** Round is just a counter; no session tracking or date-based logic

### Level Sections

- **Organization:** Classes board groups students by `placement_tests.assigned_level` (1-8)
- **Display:** Each level shown as a section with color-coded background
- **Eligibility:** Students must have:
  - `status = 'ready_to_start'`
  - `sent_to_classes = true` (manually sent from pre-enrolment detail)
  - `assigned_level` set (1-8)
  - `class_days` set (Sun/Wed, Sat/Tues, Mon/Thu)
  - `class_time` set (07:30, 10:00)

### Class Cards (READY / NOT READY / LOCKED)

- **Grouping:** Students grouped by (level, class_days, class_time, class_group_index)
- **Readiness Logic:**
  - **READY:** Student count = 6
  - **NOT READY:** Student count < 6
  - **LOCKED:** Student count > 6 (overflow)
- **Display:**
  - Card shows: `{class_days} @ {class_time}`, Class #{group_index}, Student count (e.g., "4/6"), Readiness badge
  - Color coding: READY=green, NOT READY=yellow, LOCKED=red

### "Send to Mentor Head" Button

- **Current behavior:**
  - POST to `/classes/send` with `class_key`, `level`, `class_days`, `class_time`, `class_number`
  - Calls `SendClassGroupToMentor()`:
    - Inserts/updates `class_groups` table:
      - `sent_to_mentor = true`
      - `sent_at = CURRENT_TIMESTAMP`
      - `returned_at = NULL`
  - **Does NOT:**
    - Create mentor head user/role
    - Send email/notification
    - Change lead status
    - Create any session records
  - **Visual:** Card shows "Sent to Mentor Head" text, "Return" button appears, card opacity reduced

### "Return" Button

- **Current behavior:**
  - POST to `/classes/{classKey}/return`
  - Calls `ReturnClassGroupFromMentor()`:
    - Updates `class_groups`:
      - `sent_to_mentor = false`
      - `returned_at = CURRENT_TIMESTAMP`
  - **Does NOT:**
    - Change any student data
    - Create any records

### Student Attachment to Classes

- **Current mechanism:**
  - Students attached via `scheduling.class_group_index` (1, 2, 3...)
  - `class_group_index` groups students with same (level, days, time)
  - Auto-assignment: `AssignClassGroup()` finds first group with < 6 students, assigns `class_group_index`
  - Manual move: `MoveStudentBetweenGroups()` updates `class_group_index`
- **No session tracking:** No table or concept of "sessions" (8 sessions per class)
- **No attendance:** No attendance records
- **No grades:** No grade records

### Mentor Concept

- **Database:** No `mentors` table, no `mentor_id` foreign keys
- **Users table:** Has `role` enum including `'community_officer'` but not used in classes board
- **UI:** No mentor assignment, no mentor schedule, no mentor conflict prevention
- **Workflow:** "Send to Mentor Head" is just a boolean flag in `class_groups.sent_to_mentor`

---

## 6) WHAT'S MISSING FOR MILESTONE 2 (GAPS)

### Mentor Head Role

**Missing:**
- No `mentor_head` role in `users.role` enum (currently: admin, moderator, community_officer)
- No mentor head user creation/seeding
- No mentor head-specific routes or handlers
- No mentor head dashboard/page
- No mentor head permissions/access control

### Mentor Role

**Missing:**
- No `mentor` role in `users.role` enum
- No `mentors` table (name, email, phone, assigned classes, schedule)
- No mentor assignment to classes
- No mentor schedule conflict prevention (no two classes same slot)
- No mentor dashboard/page
- No mentor-specific routes

### 8 Sessions Per Class

**Missing:**
- No `class_sessions` table (class_key, session_number 1-8, scheduled_date, scheduled_time, actual_date, actual_time, status)
- No session creation when round starts
- No session date/time assignment logic
- No session rescheduling
- No session status tracking (scheduled, completed, cancelled)

### Attendance Per Session Per Student

**Missing:**
- No `attendance` table (session_id, lead_id, attended BOOLEAN, notes)
- No attendance marking UI
- No attendance reports/statistics
- No absence tracking/follow-up triggers

### Grades A/B/C/F at Session 8

**Missing:**
- No `grades` table (lead_id, class_key, session_number, grade TEXT, notes)
- No grade entry UI
- No grade validation (A, B, C, F only)
- No grade reports

### Notes Per Student (Carry-Over)

**Missing:**
- No `student_notes` table (lead_id, note_text, created_by_user_id, created_at, session_number)
- No notes UI in classes board or student detail
- No notes carry-over between sessions/rounds

### Community Officer Feedback

**Missing:**
- No `community_officer_feedback` table (lead_id, session_number, feedback_text, follow_up_required BOOLEAN, created_at)
- No feedback entry UI (after session 4 and 8)
- No absence follow-up logs table
- No automatic triggers for feedback entry

### Credit Consumption After Session 1

**Missing:**
- No automatic `levels_consumed` increment when session 1 completes
- No credit consumption tracking per session
- No validation that student has remaining credits before session starts

### Refund Policy (Session-Based)

**Missing:**
- No session-based refund calculation
- No automatic refund eligibility check (50% after session 1, 0% after session 2)
- No refund policy enforcement in cancel flow

### Mentor Schedule Conflict Prevention

**Missing:**
- No mentor schedule table (mentor_id, class_key, session_date, session_time)
- No conflict detection logic (same mentor, same date/time)
- No validation when assigning mentor to class

---

## 7) RECOMMENDED MINIMAL IMPLEMENTATION PLAN

### Proposed New Tables

#### `mentors`
```sql
id UUID PRIMARY KEY
user_id UUID UNIQUE REFERENCES users(id) ON DELETE CASCADE
full_name TEXT NOT NULL
email TEXT UNIQUE
phone TEXT
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
```

#### `class_sessions`
```sql
id UUID PRIMARY KEY
class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE
session_number INTEGER NOT NULL CHECK (session_number >= 1 AND session_number <= 8)
scheduled_date DATE NOT NULL
scheduled_time TIME NOT NULL
actual_date DATE
actual_time TIME
status TEXT DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'completed', 'cancelled'))
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
UNIQUE (class_key, session_number)
```

#### `attendance`
```sql
id UUID PRIMARY KEY
session_id UUID NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE
lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE
attended BOOLEAN NOT NULL DEFAULT false
notes TEXT
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
UNIQUE (session_id, lead_id)
```

#### `grades`
```sql
id UUID PRIMARY KEY
lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE
class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE
session_number INTEGER NOT NULL CHECK (session_number = 8)
grade TEXT NOT NULL CHECK (grade IN ('A', 'B', 'C', 'F'))
notes TEXT
created_by_user_id UUID REFERENCES users(id)
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
UNIQUE (lead_id, class_key, session_number)
```

#### `student_notes`
```sql
id UUID PRIMARY KEY
lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE
class_key TEXT REFERENCES class_groups(class_key) ON DELETE SET NULL
session_number INTEGER
note_text TEXT NOT NULL
created_by_user_id UUID REFERENCES users(id)
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
```

#### `community_officer_feedback`
```sql
id UUID PRIMARY KEY
lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE
class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE
session_number INTEGER NOT NULL CHECK (session_number IN (4, 8))
feedback_text TEXT NOT NULL
follow_up_required BOOLEAN DEFAULT false
created_by_user_id UUID REFERENCES users(id)
created_at TIMESTAMP WITH TIME ZONE
updated_at TIMESTAMP WITH TIME ZONE
```

#### `absence_follow_up_logs`
```sql
id UUID PRIMARY KEY
lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE
session_id UUID REFERENCES class_sessions(id) ON DELETE SET NULL
action_taken TEXT NOT NULL
notes TEXT
created_by_user_id UUID REFERENCES users(id)
created_at TIMESTAMP WITH TIME ZONE
```

#### `mentor_assignments`
```sql
id UUID PRIMARY KEY
mentor_id UUID NOT NULL REFERENCES mentors(id) ON DELETE CASCADE
class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE
assigned_at TIMESTAMP WITH TIME ZONE
created_by_user_id UUID REFERENCES users(id)
UNIQUE (class_key) -- One mentor per class
```

### Proposed New Enums

**Update `users.role`:**
```sql
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN (
    'admin', 'moderator', 'community_officer', 'mentor_head', 'mentor'
));
```

### Proposed New UI Screens/Tabs

1. **Mentor Head Dashboard** (`/mentor-head`)
   - List of classes sent to mentor head (`class_groups.sent_to_mentor = true`)
   - Assign mentor to each class
   - View mentor schedule conflicts
   - Return classes to admin

2. **Mentor Dashboard** (`/mentor`)
   - List of assigned classes
   - Session schedule view
   - Attendance entry form (per session)
   - Grade entry form (session 8 only)
   - Student notes entry

3. **Classes Board — Active Classes Tab** (`/classes/active`)
   - Show classes with `status = 'in_classes'`
   - Session timeline (8 sessions per class)
   - Attendance status per student per session
   - Community officer feedback prompts (after session 4, 8)

4. **Student Detail — Active Class View** (`/pre-enrolment/{leadID}/class`)
   - Show current class assignment
   - Attendance history
   - Grades
   - Notes
   - Community officer feedback

### Proposed Backend Endpoints

#### Mentor Head Routes
- `GET /mentor-head` → List classes sent to mentor head
- `POST /mentor-head/assign` → Assign mentor to class
- `POST /mentor-head/return/{classKey}` → Return class to admin

#### Mentor Routes
- `GET /mentor` → Mentor dashboard (assigned classes)
- `GET /mentor/class/{classKey}` → Class detail with sessions
- `POST /mentor/attendance` → Mark attendance (session_id, lead_id, attended)
- `POST /mentor/grade` → Enter grade (lead_id, class_key, session_number=8, grade)
- `POST /mentor/note` → Add student note

#### Classes Routes (Extended)
- `GET /classes/active` → Active classes view (in_classes status)
- `GET /classes/{classKey}/sessions` → Session list for class
- `POST /classes/{classKey}/sessions/create` → Create 8 sessions when round starts
- `POST /classes/{classKey}/sessions/{sessionNumber}/complete` → Mark session completed

#### Community Officer Routes
- `POST /community-officer/feedback` → Submit feedback (lead_id, class_key, session_number, feedback_text, follow_up_required)
- `POST /community-officer/follow-up` → Log absence follow-up action

### Risky Migration/Refactor Notes

1. **Session Creation on Round Start:**
   - `StartRound()` currently only updates status to `in_classes`
   - Need to create 8 `class_sessions` records per class group when round starts
   - Must handle date/time calculation (e.g., session 1 = start_date, session 2 = start_date + 7 days, etc.)

2. **Credit Consumption:**
   - Currently `levels_consumed` is manual
   - Need to auto-increment when session 1 completes
   - Must validate `levels_consumed < levels_purchased_total` before allowing sessions

3. **Refund Policy Integration:**
   - Cancel flow currently uses `GetTotalCoursePaid()` for refund validation
   - Need to add session-based refund calculation:
     - If session 1 completed: refund = 50% of course paid
     - If session 2 started: refund = 0
   - Must query `class_sessions` and `attendance` to determine session state

4. **Mentor Schedule Conflicts:**
   - When assigning mentor to class, must check `class_sessions` for all assigned classes
   - Prevent overlap: same mentor, same `scheduled_date`, same `scheduled_time`

5. **Status Transition:**
   - `ready_to_start` → `in_classes` currently happens on "Start Round"
   - May need to add validation: all students must have remaining credits, all sessions must be scheduled

6. **Classes Board Readiness:**
   - Current "READY" logic (count = 6) may need to consider mentor assignment
   - May need "READY" = (count = 6 AND mentor assigned)

---

**End of System Spec Snapshot**
