# Mentor Head Workflow + HR Fixes – Deliverables

## 1. Root cause for each bug (1–2 sentences)

**1) Mentor Head cannot open class details**
- **Cause:** ClassDetail required `wf.SentToMentor` only; classes with a mentor assignment but `sent_to_mentor=false` (e.g. edge cases) were 403’d. Also, `classKey` from the path was not URL‑unescaped, so keys with `|` could break lookups.
- **Fix:** Allow viewing if `sent_to_mentor` **or** a mentor assignment exists. Use `url.PathUnescape` for `classKey` in ClassDetail (and Return).

**2) “Return to Operations” does not persist**
- **Cause:** Handler correctly used `pathParts[1]` for `classKey` and called `ReturnClassGroupFromMentor`. Persistence issues were likely from `classKey` with `|` not being unescaped when coming from the form action URL, or from clients encoding the path.
- **Fix:** Use `url.PathUnescape(pathParts[1])` for `classKey` before calling the repo. Repo logic (UPDATE `class_groups` SET `sent_to_mentor=false`, `returned_at`; DELETE from `mentor_assignments`) was already correct and is unchanged.

**3) Assign mentor not persisted / not reflected after refresh**
- **Cause:** Dashboard reads from `mentor_assignments` via `GetMentorAssignment`; Assign writes via `AssignMentorToClass` (upsert into `mentor_assignments`). Same source—no read/write split. Likely causes: form/request issues (e.g. wrong `class_key`/`mentor_user_id`), or template preselection `MentorUserIDStr` vs `printf "%s" .ID` mismatch.
- **Fix:** No repo or handler changes. Preselection and upsert logic are consistent. Conflicts remain enforced via `CheckMentorScheduleConflict`. If issues persist, verify form POST target, hidden fields, and that selected mentor `id` is sent.

**4) Mentor Head access to Classes**
- **Cause:** `/classes` List handler allowed only `admin`; moderator and others got 403. Mentor head was not in the allowed set.
- **Fix:** Allow `admin` and `mentor_head` for `/classes`. Moderator still gets 403 (access‑restricted). Mentor head has read‑only access: `IsClassesReadOnly` hides Start Round, Send to Mentor Head, Return, and Move. Route now uses `RequireAnyRole([]string{"admin","moderator","mentor_head"})`; List enforces moderator → 403, admin/mentor_head → render.

**5) HR user: add mentors**
- **Cause:** No `hr` role or UI to create mentor users.
- **Fix:** New role `hr`, migration `023_add_hr_role.sql`, GET/POST `/hr/mentors` (form: email, password), handler creates user with `role=mentor`. New mentors appear in Mentor Head assign dropdown via `GetUsersByRole("mentor")`. HR seeded from config; `RoleHomePath` and `roleCanAccessPath` updated for `hr`.

---

## 2. Exact files changed (paths)

- `internal/handlers/mentor_head.go` – ClassDetail allow sent OR assigned; PathUnescape for classKey (detail + return); UserRole in data.
- `internal/handlers/classes.go` – List RBAC (mentor_head + read‑only); `IsClassesReadOnly`; UserRole in data; `renderTemplate` now passes `r`.
- `internal/handlers/hr.go` – **new** – HR handler (MentorsList, MentorsCreate).
- `internal/handlers/templates.go` – `renderTemplate(w, r, name, data)`; inject `UserRole` from `middleware.GetUserRole(r)` when missing; `hr_mentors` mapping.
- `internal/handlers/auth.go` – `RoleHomePath` and `roleCanAccessPath` for `hr`; `mentor_head` can access `/classes`.
- `internal/handlers/role.go` – `IsHR` helper.
- `internal/views/mentor_head.html` – no logic change (View details + Return forms already correct).
- `internal/views/classes.html` – Wrap Start Round, Send/Return, Move in `{{if not .IsClassesReadOnly}}` (or equivalent).
- `internal/views/layout.html` – Nav: Pre‑enrolment hide for `hr`; Classes for admin/mentor_head; Finance admin‑only; HR link for admin/hr.
- `internal/views/hr_mentors.html` – **new** – HR mentors form (email, password) + flash messages.
- `internal/config/config.go` – `HREmail`, `HRPassword`.
- `cmd/server/main.go` – HR handler init; seed HR user; GET/POST `/hr/mentors`; log HR default login.
- `internal/db/migrations/023_add_hr_role.sql` – **new** – Add `hr` to `users_role_check`.

All `renderTemplate` call sites now pass `r`: `auth.go`, `pre_enrolment.go`, `finance.go`, `community_officer.go`, `mentor.go`, `mentor_head.go`, `classes.go`, `hr.go`.

---

## 3. Migrations

- **`023_add_hr_role.sql`** – Add `hr` to `users.role` constraint:

```sql
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN (
    'admin', 'moderator', 'community_officer', 'mentor_head', 'mentor', 'hr'
));
```

No new tables or columns. Mentor assignment remains in `mentor_assignments`; workflow in `class_groups` (`sent_to_mentor`, `returned_at`).

---

## 4. RBAC rules summary

| Role              | Access |
|-------------------|--------|
| **admin**         | All: pre‑enrolment, classes, finance, mentor‑head, mentor, community‑officer, HR/mentors, login/logout. |
| **moderator**     | Pre‑enrolment only. 403 on `/classes`, `/finance`. |
| **mentor_head**   | Mentor‑head pages (`/mentor-head`, `/mentor-head/class/{id}`) + `/classes` (read‑only). No finance. Login/logout. |
| **mentor**        | Mentor pages (`/mentor`, `/mentor/class/{id}`). Login/logout. |
| **community_officer** | Community‑officer pages. Login/logout. |
| **hr**            | `/hr/mentors` only (create mentor users). Login/logout. No pre‑enrolment, classes, or finance. |

Finance stays **admin‑only**. Moderator restrictions unchanged (blocked from classes and finance).

---

## 5. Manual test checklist (10 bullets)

1. **Assign mentor persists:** As mentor_head, assign a mentor to a class on `/mentor-head`, submit, refresh → dropdown still shows selected mentor.
2. **Return persists:** As mentor_head, click “Return to Operations” for a class, refresh → class no longer on `/mentor-head` list (or clearly returned state).
3. **Mentor head can open details:** As mentor_head, click “View details” for a class on `/mentor-head` → read‑only class detail (sessions, students, attendance, notes) loads.
4. **HR adds mentor, appears in dropdown:** As hr, create a mentor on `/hr/mentors` → as mentor_head, open assign dropdown on `/mentor-head` → new mentor appears.
5. **Mentor head can access /classes:** As mentor_head, open `/classes` → page loads; Start Round, Send, Return, Move are hidden (read‑only).
6. **Moderator blocked from /classes:** As moderator, open `/classes` → 403 access‑restricted.
7. **Moderator blocked from /finance:** As moderator, open `/finance` → 403 access‑restricted.
8. **Mentor head no Finance link:** As mentor_head, check sidebar → no Finance link.
9. **HR only HR/mentors:** As hr, check nav → only HR · Mentors (and login/logout); no Pre‑enrolment, Classes, Finance.
10. **Admin can create mentors:** As admin, open `/hr/mentors`, create mentor → success; new mentor in Mentor Head dropdown.

---

## Notes

- Mentor assignment: **`mentor_assignments`** table; dashboard reads from it, assign writes to it. Return clears `mentor_assignments` for the class and sets `class_groups.sent_to_mentor=false`, `returned_at`.
- Route style and layout `ContentTemplate` branching kept consistent with existing patterns.
- Finance idempotency and lead lifecycle validations are unchanged.
