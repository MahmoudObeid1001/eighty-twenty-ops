# SOURCE OF TRUTH - Business Rules

**Last Updated**: 2026-01-31  
**Status**: Definitive - All implementation must match these rules

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

### Manager
**Status**: Future feature (Tier 3)  
**Current**: Not implemented

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

### Final Grading Repeat Risk Indicator

**Business Rule**: Students with more than 2 missed sessions must be flagged during final grading.  
**Purpose**: Align grading UI with repeat criteria (absences > 2).

### Returning Students Credits & Repeat Flag

**Business Rule**: Remaining credits are computed as `levels_purchased_total - levels_consumed` (min 0).  
**UI**: Pre‑enrolment list shows a **REPEAT** badge if the latest class outcome is `repeated`.  
**Status**: When remaining credits = 0 after close round, status must be `renewal_pending` (not paid_full).

**Filter**: Pre‑enrolment list includes a Repeat Level filter.

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

## Grades Feature

**Status**: Planned (implement)

**Rules**:
- **Scale**: A | B | C only (not 0-100)
- **Scope**: Per student per class per level
- **Who enters**: Mentor (primary), Mentor Head can edit/override
- **Storage**: Grade letter + optional short note

**Visibility**:
- Mentor Head + Student Success can view
- Mentor can view for their class

**Purpose**: Student card needs "last level grade" + progress signal

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
