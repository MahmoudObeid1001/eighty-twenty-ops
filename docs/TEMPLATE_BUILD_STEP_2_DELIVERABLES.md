# Template Build Step 2 — Deliverables

**Date:** 2026-01-26  
**Scope:** SCOPE A (new templates + layout branches) + SCOPE B (follow-up filter)

---

## 1) FILES CREATED

| File | Description |
|------|-------------|
| `internal/views/mentor_head.html` | Mentor Head dashboard; block `mentor_head_content` |
| `internal/views/mentor.html` | Mentor dashboard (My Classes); block `mentor_content` |
| `internal/views/mentor_class_detail.html` | Mentor class detail (sessions 1–8, attendance, grades, notes); block `mentor_class_detail_content` |
| `internal/views/community_officer.html` | Community Officer dashboard (pending feedback S4/S8, log follow-up); block `community_officer_content` |

---

## 2) FILES MODIFIED

| File | Changes |
|------|---------|
| `internal/views/layout.html` | Added `ContentTemplate` branches for `mentor_head_content`, `mentor_content`, `mentor_class_detail_content`, `community_officer_content`. No changes to existing branches. |
| `internal/views/pre_enrolment_list.html` | Added “High Priority Follow-Up” checkbox; form submits to GET `/pre-enrolment` with `follow_up=high_priority` when checked; preserves other params; quick-filters preserve `follow_up` when set. |
| `internal/handlers/pre_enrolment.go` | List handler now passes `FollowUpFilter` in template data. |
| `internal/handlers/mentor_head.go` | Dashboard passes `Title`, `IsModerator`, and flash query params (`assigned`, `returned`, `rescheduled`, `closed`) to template. |
| `internal/handlers/mentor.go` | Dashboard and ClassDetail pass `Title`, `IsModerator` to template. |
| `internal/handlers/community_officer.go` | Dashboard passes `Title`, `IsModerator`, `feedback_submitted`, `follow_up_logged` to template. |

---

## 3) FORMS IMPLEMENTED

| Endpoint | Method | Form fields used |
|----------|--------|-------------------|
| `/mentor-head/assign` | POST | `class_key`, `mentor_user_id` |
| `/mentor-head/{{.ClassKey}}/return` | POST | (classKey in URL) |
| `/mentor-head/session/cancel` | POST | `session_id`, `compensation_date`, `compensation_time` |
| `/mentor-head/close-round` | POST | `class_key` |
| `/mentor/attendance` | POST | `session_id`, `lead_id`, `attended`, `class_key` |
| `/mentor/grade` | POST | `lead_id`, `class_key`, `grade` |
| `/mentor/note` | POST | `lead_id`, `class_key`, `note_text` |
| `/mentor/session/complete` | POST | `session_id`, `class_key` |
| `/community-officer/feedback` | POST | `lead_id`, `class_key`, `session_number`, `feedback_text`, `follow_up_required` (optional) |
| `/community-officer/follow-up` | POST | `lead_id`; optional: `session_id`, `message_sent`, `reason`, `student_reply`, `action_taken`, `notes` |

---

## 4) QUICK MANUAL UI TESTING STEPS

### Admin

1. **Login** as admin.
2. **Pre-enrolment**
   - Open `/pre-enrolment`. Verify “High Priority Follow-Up” checkbox in filter bar.
   - Check it, submit → URL includes `follow_up=high_priority`; list filters to high-priority leads (if any).
   - Uncheck, submit → `follow_up` removed; list shows all (per other filters).
3. **Mentor Head**
   - Open `/mentor-head`. Verify layout (sidebar, main content).
   - If classes exist: assign mentor (dropdown + Assign), Return, Close round, Cancel & reschedule form.
   - Trigger assign/return/reschedule/close → redirect with `?assigned=1` etc. → flash messages on next load.
4. **Mentor**
   - Open `/mentor`. See “My Classes” table; “View class” → `/mentor/class/{classKey}`.
5. **Mentor class detail**
   - Open `/mentor/class/{classKey}`. Verify sessions table, Complete button, students × attendance (P/A), grade dropdown, Add note.
   - Mark P/A, change grade, add note, Complete session → redirect; re-open → data persisted.
6. **Community Officer**
   - Open `/community-officer`. See pending feedback S4/S8 tables and “Log absence follow-up” form.
   - Submit feedback → redirect `?feedback_submitted=1` → flash. Submit follow-up → `?follow_up_logged=1` → flash.

### Mentor Head

1. **Login** as `mentor_head`.
2. **Mentor Head**
   - GET `/mentor-head` → 200, Mentor Head dashboard in normal layout.
3. **Mentor / Community Officer**
   - GET `/mentor`, `/community-officer` → 403 (only mentor / community_officer + admin allowed).
4. **Pre-enrolment**
   - GET `/pre-enrolment` → 200. High Priority Follow-Up filter works as for admin.

### Mentor

1. **Login** as `mentor`.
2. **Mentor**
   - GET `/mentor` → 200, “My Classes” in layout.
   - GET `/mentor/class/{classKey}` for an assigned class → 200, class detail.
   - For a class **not** assigned to this mentor → 403.
3. **Mentor Head / Community Officer**
   - GET `/mentor-head`, `/community-officer` → 403.

### Community Officer

1. **Login** as `community_officer`.
2. **Community Officer**
   - GET `/community-officer` → 200, dashboard in layout.
   - Submit feedback, log follow-up → redirect + flash.
3. **Mentor Head / Mentor**
   - GET `/mentor-head`, `/mentor` → 403.

### Moderator

1. **Login** as moderator.
2. **Pre-enrolment**
   - GET `/pre-enrolment` → 200. High Priority Follow-Up filter works.
3. **Classes / Finance**
   - GET `/classes`, `/finance` → 403, custom access-restricted page (unchanged).
4. **Mentor Head / Mentor / Community Officer**
   - GET `/mentor-head`, `/mentor`, `/community-officer` → 403 (no access).

---

## 5) OUT OF SCOPE (NOT DONE)

- **Active Class tab** in `pre_enrolment_detail.html`: not implemented (detail handler does not provide Class/Sessions/Attendance). No placeholder added.
- **Active Classes tab** in `classes.html`: not implemented.
- **Nav links** for Mentor Head, Mentor, Community Officer: not added to sidebar.
- **JavaScript**: none; all actions via plain form POST/GET.

---

## 6) NOTES

- **Layout:** New pages use the same `layout` and sidebar as existing app pages; only new `ContentTemplate` branches were added.
- **CSS:** Reuses existing classes (`btn`, `btn-primary`, `btn-secondary`, `table-container`, `badge`, etc.) and patterns from `classes.html` / `pre_enrolment_*`.
- **Return form:** Uses `POST /mentor-head/{{.ClassKey}}/return`. If `ClassKey` contains `/` (e.g. `Sun/Wed`), URL encoding may be needed; same pattern as classes board.
- **Follow-up filter:** Handler passes `FollowUpFilter`; checkbox and quick-filters preserve `follow_up=high_priority` when set.
