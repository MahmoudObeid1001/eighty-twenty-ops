# SOURCE OF TRUTH - Business Rules

**Last Updated**: 2026-02-18  
**Status**: Definitive - All implementation must match these rules

---

## Maintenance Notes

- 2026-02-20: Staff Management enhancement implemented for manager user removal.
  - Removal action is manager-only from `/app/staff`.
  - API route: `DELETE /api/manager/users/:id`.
  - Safety guards:
    - manager cannot delete themselves.
    - if user has FK-linked records, deletion is blocked with a conflict response.
- 2026-02-20: Cancel-refund total fix for returning students with remaining credits.
  - `unused_credits_value` is already a final refundable amount; it must not be added to `current_cycle_paid`.
  - Cancel modal and backend validator now compute:
    - if `unused_credits_value > 0` => `total_refundable = unused_credits_value`
    - else => `total_refundable = total_course_paid`
- 2026-02-19: Manager role + staff provisioning implemented:
  - DB migration `060_reactivate_manager_and_force_password_change.sql` added `users.must_change_password` and reactivated `manager` in role constraint.
  - Manager APIs added: `GET /api/manager/users`, `POST /api/manager/users`.
  - First-login setup endpoint added: `POST /api/auth/force-change-password`.
  - Login flow now redirects `must_change_password=true` users to `/app/setup-password`.
  - Frontend added `/app/staff` (manager-only navigation) and `/app/setup-password`.
- 2026-02-19: Manager role re-activation approved.
  - Manager is now the top-privilege role ("god mode") with route-level bypass over role checks.
  - Only Manager can create staff users from Staff Management.
  - New users are created with a temporary password and `must_change_password = true`.
  - Users flagged with `must_change_password = true` are blocked from normal app/API flows until they set a permanent password.
- 2026-02-18: Lint/quality cleanup pass (`golangci-lint`) completed. No business rules or runtime behavior were intentionally changed.
- 2026-02-19: Mobile compatibility polish added as CSS-only responsive overrides (no workflow/business-logic changes). Desktop behavior remains source-of-truth baseline.
- 2026-02-19: SSR views mobile polish extended to `/pre-enrolment`, `/finance`, and `/classes` via template class hooks + media-query CSS only (no server/business logic changes).
- 2026-02-19: Pre-enrolment cancel/late-join modals and list action rows received extra small-screen layout hardening (stacked actions, full-width mobile dialog body).
- 2026-02-19: Mobile hardening pass added responsive handling for React BI filter controls and Class Workspace toast alerts (to avoid off-screen fixed notices on narrow devices).
- 2026-02-19: Deployment safety fix for React CSS path:
  - React shell now references `./static/main.css` in source, which builds to `/app/static/main.css`.
  - Backend serves `/app/static/*` from `frontend/dist` to avoid cross-origin/localhost CSS leakage in production.
- 2026-02-19: Mobile CSS hardening for dense data views:
  - SSR table containers enforce horizontal scroll with mobile `min-width` table strategy.
  - Classes cards allow header/meta wrapping on small screens to prevent clipped schedule/student text.
- 2026-02-19: Mobile safe-area rule added for fixed floating UI overlays:
  - `main-content` gets extra right padding on small screens to keep controls readable/clickable when third-party floating widgets are present.

---

## Role Definitions

### Student Success (SS)
**Replaces**: Community Officer (removed)  
**Active**: Yes

**Visibility**:
- Sees ONLY classes where `round_status = 'IN_PROGRESS'` (Mentor Head flagged as "Start Round")
- Classes grouped by mentor (Mentor → list of classes)

**Responsibilities**:
1. **Absence Follow-Up Workflow** (per class, per session)
2. **Feedback Collection** (sessions 4 & 8)
3. **Complaint Management** (creation only)

### HR
**Active**: Yes  
**Purpose**: Mentor lifecycle management

**Responsibilities**:
- Create and manage mentor accounts (hire/onboard)
- Edit mentor profile fields (name, email, phone, role, status active/inactive)
- Maintain mentor availability and admin metadata (optional later)
- Does NOT handle sessions or attendance directly

**Create Rule**:
- HR mentor creation requires `full_name`, `email`, `phone`, and `password`.
- DB enforces mentor profile completeness (`role='mentor'` requires non-empty name and phone).

### Manager
**Status**: Active
**Purpose**: Full-system owner role with global access and staff provisioning authority

**Responsibilities**:
- Access every protected route (middleware bypass over explicit role lists)
- Create staff users with temporary passwords
- Force first-login password setup for newly created users

**Security Rules**:
- `must_change_password = true` users cannot access business endpoints/pages until password setup is completed.
- Allowed while blocked: authentication context endpoints required for setup, force-change password action, logout.

---

## Student Success Workflows

### A) Absence Follow-Up Workflow

**Business Rule**: Per class, per session

**Steps**:
1. Reviews **Absence Feed** (absent/late events) grouped by session number
2. Uses **WhatsApp action** to contact students
3. Uses **Follow-up modal** to write notes + set follow-up status:
   - `contacted` (same day)
   - `not_replied` (after 1 day)
   - `no_response` (after 4 days)

**Escalation Rule**:
- `contacted` / `not_replied` ⇒ stays in Absence Feed
- `no_response` ⇒ escalates to Follow-ups queue AND disappears from Absence Feed

**Resolution**:
- When resolved (student replied OR SS resolves manually), it disappears and stays gone after refresh

### B) Feedback Collection

**Business Rule**: Sessions 4 and 8 only

**Steps**:
1. Sends feedback requests
2. Tracks whether feedback was received
3. Marks "received" when student submits feedback
4. "Remove" deletes the log if wrong/irrelevant

### C) Note Visibility

**Business Rule**: SS notes are private

**Visibility**:
- SS + Mentor Head can see SS notes
- Mentors should NOT see SS private notes (mentor sees only mentor-facing notes)

---

## Escalation Rules

### Absence Escalation Timeline

**Required behavior**:

| Status | Timeline | Action |
|--------|----------|--------|
| `contacted` | Same day | Stays in Absence Feed |
| `not_replied` | After 1 day | Stays in Absence Feed |
| `no_response` | After 4 days | Escalates to Follow-ups queue |

### High Priority Flag

**Business Rule**: If a student reaches **3 missed sessions** in the same level, raise a "high priority" flag

**Visibility**:
- Mentor Head + Student Success
- Mentor sees escalation in global Student Card modal (one unified modal)

---

## Attendance Rules

### 24-Hour Deadline

**Business Rule**: Mentors must mark attendance within 24 hours after session time (Africa/Cairo)

**Behavior**:
- After 24 hours passes: Show **red banner reminder** to mentor
- Once attendance is recorded: Banner disappears
- Mentors are blocked from marking attendance after deadline
- Mentor Head, Admin, and Student Success can override for corrections

### Session Completion Requires Attendance

**Business Rule**: A session cannot be marked completed until attendance is recorded for all applicable students.  
**Exception**: Late joiners are excluded for sessions before their join session.

### Mentor Pre-Start Class Visibility

**Business Rule**:
- Mentors can open assigned class workspace and view students/sessions when class is `sent_to_mentor = true` and `round_status = 'not_started'`.
- This pre-start mentor view is read-only: mentors cannot mark attendance or complete sessions until Mentor Head starts the round (`round_status = 'active'`).

### Late Joiner Eligibility (Sent-Not-Started Exception)

**Business Rule**: Late Joiner can target classes that are either:
- `round_status = 'active'` (normal case), or
- `sent_to_mentor = true` and `round_status = 'not_started'` (pre-start exception).
**Lead Status Gate**: Late Join is available only when the lead is `ready_to_start`. It must not be available for `renewal_pending` or earlier pipeline states.

**Capacity Rule**: Keep the same late-join capacity gate (4-5 current students).

**Notification Rule**:
- If late join happens into a not-started sent class, show a notification banner to **Mentor Head only**.
- Banner must be dismissible via close `X` (acknowledge/dismiss action).

**State Invariant**:
- After a successful late join, lead lifecycle state is authoritative:
  - `leads.status` must remain `in_classes`
  - `leads.sent_to_classes` must remain `true`
- Generic Pre-Enrolment `Save` must not downgrade or reset these fields.
- Revert is only through explicit `Undo Late Join`.

### Final Grading Repeat Risk Indicator

**Business Rule**: Students with more than 2 missed sessions must be flagged during final grading.  
**Purpose**: Align grading UI with repeat criteria (absences > 2).

### Returning Students Credits & Repeat Flag

**Business Rule**: Remaining credits are computed as `levels_purchased_total - levels_consumed` (min 0).  
**Implementation Invariant**: For returning students, `levels_consumed` is cumulative across cycles, so when a new bundle is purchased, `levels_purchased_total` must be stored as a cumulative target: `levels_consumed_at_purchase + bundle_levels_bought`.  
**Placement Test Rule**: Returning-cycle leads (`is_returning=true` or statuses `renewal_pending`/`waiting_for_round`/`schedule_assigned`/`ready_to_start`/`in_classes`) must not be moved back to `test_booked` by Ops actions.
**Offer Action Rule**: Paid waiting-flow leads (`waiting_for_round`/`schedule_assigned`/`ready_to_start`/`in_classes`) and leads with remaining credits must not be moved to `offer_sent` by quick actions (e.g., `Packages Sent`).
**Course Payment Stage Rule**: Course-payment entry is locked until lead reaches payment-collection statuses (`offer_sent`, `booking_confirmed`, `deposit_paid`); it stays locked for `renewal_pending`.
**Renewal Refusal Rule**: Returning leads with consumed entitlement (`remaining credits = 0`) can be explicitly marked as `refused_renewal` and moved to `cold_lead` from pre-enrolment action controls (admin only); refusal must be audit-stored with timestamp.
**UI**: Pre‑enrolment list shows a **REPEAT** badge if the latest class outcome is `repeated`.  
**Status**: When remaining credits = 0 after close round, status must be `renewal_pending` (not paid_full).

**Filter**: Pre‑enrolment list includes a Repeat Level filter.

### Placement Test Discount Payment Gate

**Business Rule**:
- Placement-test payment required amount must be the discounted final fee, not the raw base fee.
- If final discounted fee is `0`, system must allow booking/save with `paid = 0` and no payment method/date requirement.
- Generic `save` remains draft-safe (does not enforce booking settlement gate).
- Explicit booking (`mark_test_booked`) enforces settlement gate using discounted final fee.

### Pre‑Enrolment Filter Consistency

**Business Rule**:
- Quick filters (`Hot Leads`, `Cold Leads`, etc.) are explicit modes and must apply only when their own chip is clicked.
- Using the main filter form (`All Statuses`, status dropdown, payment dropdown, search) must not silently keep `hot`/`cold` query constraints.
- Quick-filter chips are toggleable: clicking an already active chip clears that chip's filter.

**Status Filter Rule**:
- Use canonical stage status values in filter URLs (`RENEWAL_PENDING`, `WAITING_FOR_ROUND`, etc.).
- Backward compatibility for legacy lowercase status query values is allowed.

**Cold List Marker Rule**:
- `cold_lead` rows that have a renewal refusal audit record must show `REFUSED RENEWAL` marker in the name cell.
- `PROMOTED` marker is suppressed when row status is `cold_lead` to keep retargeting context clear.

**Cold Leads Clock Rule**:
- Cold Leads quick filter must include explicitly marked `status = cold_lead`.
- For `offer_sent` candidates, timing must use `leads.offer_sent_at` (7+ days) rather than generic `updated_at`.
- For legacy rows where `offer_sent_at` is null, fallback to `updated_at` is allowed temporarily.
 - Default pre-enrolment feed must exclude `cold_lead`; cold rows appear only in explicit cold mode (`cold=1`) or explicit `COLD_LEAD` status filter.

**Cold Leads Level Filter Rule**:
- In Pre-Enrolment when `cold=1`, show one level dropdown for filtering.
- Selecting a level filters the bottom leads table to that assigned level.
- Within a selected level, ordering remains newest first (`updated_at DESC`).
- Leads without assigned level are excluded when a specific level is selected.
- Implementation note (2026-02-14): server derives level options from current cold dataset and applies selected-level filtering in list handler.

**Hot Leads Badge Clock Rule**:
- Unpaid `tested` leads: use `placement_tests.test_date` to age the HOT/WARM/COOL badge.
- Unpaid `offer_sent` leads: use `leads.offer_sent_at` to age the HOT/WARM/COOL badge.
- Fallback for legacy null timestamps is `leads.updated_at`.
- Bucket thresholds remain: HOT 0-6 days, WARM 7-13 days, COOL 14+ days.

### Cancellation Refund Guard (Returning Students)

**Business Rule**: Cancel flow refund checks must use computed remaining credits (`levels_purchased_total - levels_consumed`), not stale cached values.  
**Required behavior**: If computed remaining credits > 0, cancellation must enforce refund modal/validation for unused credits value.
**Cycle Scope Rule**: In cancel flow, unused-credit valuation for returning students must be derived from pre-cycle carryover entitlement (latest payment before current cycle start), while current-cycle paid cash is calculated separately via current-cycle payment totals. This avoids cross-cycle mixing.
**No Double-Count Rule**: When unused-credit valuation is present, it is the refundable total for that case and must not be added again to current-cycle paid.
**Implementation Rule**: The system persists explicit cycle records in `payment_cycles`. Refund and current-cycle paid computations should read cycle boundaries from `payment_cycles.started_at` when present (legacy fallback allowed only for older rows without cycle records).

---

## Complaint Management

### Categories (Fixed List)

**Required values**:
- `mentor_behavior`
- `session_quality`
- `scheduling`
- `content`
- `technical`
- `admin_process`
- `student_behavior`
- `other`

### Urgency Levels (Fixed List)

**Required values**:
- `low`
- `medium`
- `high`
- `critical`

**Business Rules**:
- `high`/`critical` float to top + highlighted

### Complaint Tiers

**Current Implementation** (Tier 1-2):
1. **Tier 1**: Student Success creates complaint
2. **Tier 2**: Mentor Head handles lifecycle (contacted, notes, actions, resolved)

**Future** (Tier 3):
3. **Tier 3**: Manager override
   - Can delete complaints
   - Override resolution
   - Reopen
   - **Not implemented now** - document as "future"

---

## Reports - Business Intelligence (Admin Dashboard)

### Access Scope

**Business Rule**:
- BI reports under `/app/reports` are for `admin` and `mentor_head`.
- Existing mentor-compliance report workflows for Student Success stay unchanged.

### Report 1: Sales Pipeline Efficiency

**Business Questions**:
- Are leads converting from test-booked to paid?
- Which leads are stuck in `offer_sent` too long?
- Are returning students renewing?

**Required Outputs**:
- Conversion metric for test-booked to paid in recent window.
- Bottleneck list: leads in `offer_sent` older than 3 days.
- Renewal rate for `is_returning` students based on new-cycle payment activity.

### Report 2: Financial Leakage & Liability

**Business Questions**:
- Are there underpaid students already in classes?
- What is service liability in unused credits?
- What is the current-round cash pulse?

**Required Outputs**:
- Ghost students list: `in_classes` with paid amount below current offer/cycle due.
- Refund/service liability: sum of `remaining_credits × standard_level_price`.
- Revenue pulse metric: payments collected in current active round.

### Report 3: Retention & Drop-off

**Business Questions**:
- Who churned after finishing?
- Who paid but is stalled before scheduling?

**Required Outputs**:
- Lost list: finished-level students with `remaining_credits = 0` and status `renewal_pending`.
- Stalled list: finished-level students with `remaining_credits > 0` and status `waiting_for_round`.

### Report 4: Operational Volume

**Business Questions**:
- Is operating capacity growing or shrinking?
- How many learners were active in a date range?

**Required Outputs**:
- Active classes trend by month.
- Started learners count in selected range:
  - distinct students whose enrollment started in period (`class_enrollments.joined_at` or fallback `class_enrollments.enrolled_at`)
  - includes late joiners naturally through enrollment start date.
- Finished learners count in selected range:
  - distinct students with completed enrollments in period (`class_enrollments.completed_at` range).

**Implementation Notes (2026-02-18)**:
- API endpoint: `GET /api/reports/bi?from=YYYY-MM-DD&to=YYYY-MM-DD` (roles: `admin`, `mentor_head`).
- Date filter defaults to rolling 6-month range when `from`/`to` are omitted.
- Report 4 learner volume metrics are split for growth analysis:
  - `started_learners`: distinct `class_enrollments.lead_id` where `enrolled_at` is in range.
  - `finished_learners`: distinct `class_enrollments.lead_id` where `completed_at` is in range.
  - This is the quarter-over-quarter source of truth for “entered learning” vs “finished level”.
- Liability valuation uses bundle-weighted per-credit pricing by purchased bundle tier:
  - bundle 1 = `1300`
  - bundle 2 = `1200`
  - bundle 3 = `1100`
  - bundle 4 = `1000`
- Finance dashboard `CREDITS (Remaining Levels)` widget is aligned to BI scope:
  - include only non-cancelled leads
  - remaining credits computed as `GREATEST(levels_purchased_total - levels_consumed, 0)`
  - student breakdown includes zero-credit students (`0/1/2/3+` buckets) to keep totals interpretable
  - card shows both:
    - students with credits `> 0`
    - total students in scope (including `0` credits)

### Responsive UI Safety Rule

**Business Rule**:
- Mobile-friendliness is a presentation-layer enhancement only.
- Existing desktop layout/behavior is the baseline and must not be changed by responsive work.

**Implementation Rule**:
- Prefer media-query CSS overrides and class hooks.
- Do not alter server/business flow, status transitions, scoring, or permission gates for responsive changes.


---

## Grades Feature

**Status**: Active (automated scoring + mentor head override)

**Rules**:
- **Scale**: A | B | C only (not 0-100)
- **Scope**: Per student per class per level
- **Who enters**: Mentor (primary), Mentor Head can edit/override
- **Storage**: Grade letter + optional short note

**Visibility**:
- Mentor Head + Student Success can view
- Mentor can view for their class

**Purpose**: Student card needs "last level grade" + progress signal

### Grade Notes Sync Rule

**Business Rule**: Final grading notes entered in the grading portal must also appear in the student's global notes feed.

**Required behavior**:
- Saving/updating a final grade note updates `grades.notes` and mirrors it to `student_notes`.
- Clearing a final grade note removes the mirrored note from `student_notes`.
- The mirrored note is mentor-facing (not private) and tied to the same class/session context.
**Display Rule**:
- Student timeline/card views must render this as one visible entry.
- Implementation rule: when `/api/students/:id/notes` is built, drop `type='note'` rows whose `id` equals `gradeMirrorNoteID(grade_note.id)`; keep the `grade_note` row as the single displayed source.

### Final Grade Automation Rule

**Business Rule**: Final grade is data-driven from session attendance + task completion + participation stars.

**Scoring Algorithm (100 points)**:
- Attendance (50): proportional by attendance rate, with severe-absence guard.
  - If absences in class `> 2` (3+), attendance score is `0`.
  - Else `attendance_score = (present_sessions / 8) * 50`.
  - Implemented in backend grade preview calculation (`internal/models/repository.go`), and consumed by grading UI/report card.
- Tasks (40): from sessions `2..8` (7 tasks total).
  - If completed tasks `<= 1` then `0`.
  - Else `(completed_tasks / 7) * 40`.
- Participation (10): average stars `(1..5)` across attended sessions, `(avg_stars / 5) * 10`.
**Data Entry Rule**:
- Session task completion and participation stars may be entered even if attendance is `ABSENT`.
- Participation scoring remains computed from attended sessions only; absent-session stars are stored as evidence but do not affect participation average.

**Grade Letter Mapping**:
- `A`: score `>= 85`
- `B`: score `>= 70` and `< 85`
- `C`: score `>= 50` and `< 70`
- `F`: score `< 50`

**Override Rule**:
- Mentors must submit the calculated grade (server validates).
- Mentor Head may override manually.

**Legacy Safety Rule**:
- Missing legacy session-performance rows must not mass-fail historical classes.
- Safe fallback:
  - Missing task rows for historical data are treated as completed by default for scoring fallback.
  - Missing participation score defaults to neutral `3/5` for attended sessions.

### Student Report Card Rule

**Business Rule**: Each student can generate a print-friendly report card that justifies final grade with session evidence.

**Data Contract**:
- Report consumes `GET /api/student?lead_id=...&class_key=...` payload including:
  - student identity + class level
  - final grade + mentor comment
  - calculated score breakdown (attendance/tasks/participation + total)
  - per-session evidence (attendance, task status, stars)

**Certificate Rule**:
- Certificate page is rendered only when final grade is not `F`.
- If grade is `F`, only report card page is printed.

**Visibility Rule**:
- Student Success can open and print/download the report card from final grading in read-only mode.
- Report visibility does not grant grade-edit privileges.

**Implementation note (2026-02-15)**:
- New React component: `frontend/src/components/StudentReportCard.tsx`.
- Final Grading UI includes "View Report" action in class workspace.
- Report print uses `@media print` and `window.print()`.

### Mentor Round Report Rule

**Business Rule**: Mentor Head can generate a print-friendly mentor performance report per class for round review discussions.

**Scope Rule**:
- Evaluations endpoint supports scope filtering:
  - `scope=active` (default)
  - `scope=closed` (post-round discussion)
- Closed scope supports additional filters:
  - `q` for mentor name/email
  - `from` and `to` for class close date (`YYYY-MM-DD`)
- Closed scope resolves mentor ownership from `closed_mentor_user_id` because mentor assignments are removed at close.

**Report Content Rule**:
- Report includes:
  - Collective KPI (weighted)
  - Manual KPIs (session quality, students feedback, Trello compliance)
  - Auto KPIs (attendance punctuality, WhatsApp groups management)
  - Session evidence strip (attendance by session + Trello checks)

**Editing Rule**:
- Closed scope is discussion/reporting context; manual editing remains active-round only.

### Mentor Total Active Report Rule
**Business Rule**:
- Mentor Head can generate one combined "Total Active Report" for current active classes.
- Report includes:
  - global totals (active mentors, active classes),
  - mentor-level summary (active class count + collective KPI),
  - class-level KPI rows for each active class under each mentor.

### Pre-Enrolment Smart Steps Rule (Arabic)
**Business Rule**:
- Pre-enrolment detail includes a compact Arabic "next steps" checklist for all leads (new + returning), directly under the top workflow summary area.
- The decision logic remains rule-based and deterministic from lead status and payment/schedule/credits context.

**AI Rewrite Guardrail**:
- Optional AI rewrite may improve Arabic wording only.
- AI must not add/remove/reorder steps or alter workflow decisions.
- If AI result is unavailable/invalid, system uses deterministic Arabic templates.
- AI rewrite is feature-flagged (`SMART_STEPS_AI_ENABLED`) and requires `OPENAI_API_KEY`.

---

## Architecture Direction

### Frontend

**Primary**: React under `/app` for dashboards
- `mentor_head`
- `mentor`
- `student_success`
- `hr`

**Removed**: `community_officer` (replaced by `student_success`)

**Legacy**: SSR stays temporarily for:
- Admin/pre-enrolment
- Finance
- Until migrated safely

**URLs**: Role-based landing always goes to `/app/...` for app roles

---

## Local Development Ports

- **Frontend (Vite dev server)**: `http://localhost:3000`
- **Backend (Go server / API)**: `http://localhost:3001`

---

## Implementation Priority

1. **High**: Student Success workflows (absence, feedback, notes)
2. **High**: Attendance 24-hour deadline banner
3. **High**: Complaint categories/urgency constraints
4. **Medium**: Escalation rules (timeline + 3-absence flag)
5. **Medium**: Grades feature (A/B/C)
6. **Low**: Finance documentation
7. **Future**: Manager role (Tier 3)
