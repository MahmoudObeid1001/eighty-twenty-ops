# classKey Path Param Fix – Deliverables

## 1. Root cause (2–3 sentences)

**classKey** is generated as `L{level}|{class_days}|{class_time}|{group_index}` (e.g. `L2|Sun/Wed|07:30:00|1`). **class_days** can contain **`/`** (e.g. `Sun/Wed`). Using classKey as a **path segment** in routes like `/mentor-head/{classKey}/return` or `/mentor-head/class/{classKey}` makes the `/` inside classKey act as a path separator. The router then sees multiple segments (`L2|Sun`, `Wed|07:30:00|1`, `return`), so we only parse the first segment as classKey and pass a **truncated** value (e.g. `L2|Sun`) to the handler. The DB stores the full key (`L2|Sun/Wed|07:30:00|1`), so the UPDATE matches **0 rows**, Return “succeeds” (before RowsAffected check) or returns “no class_group updated”, and the class stays on the Mentor Head list.

## 2. Exact files changed

| Path | Changes |
|------|---------|
| `internal/handlers/templates.go` | Add `net/url` import; add `urlquery` to template `FuncMap`; parse with `template.New("").Funcs(funcMap).ParseFS(...)`. |
| `internal/handlers/mentor_head.go` | **ReturnClass:** read `class_key` from `r.FormValue("class_key")`; remove path parsing. **ClassDetail:** read `class_key` from `r.URL.Query().Get("class_key")`; remove path parsing. Remove unused `splitPath` / `splitString`. |
| `internal/handlers/mentor.go` | **ClassDetail:** read `class_key` from `r.URL.Query().Get("class_key")`; remove path parsing. Add `net/url`; redirects (attendance, grade, note, session complete) use `/mentor/class?class_key=...` with `url.QueryEscape(classKey)`. |
| `internal/handlers/classes.go` | **ReturnFromMentor:** read `class_key` from `r.FormValue("class_key")`; remove path parsing. |
| `internal/views/mentor_head.html` | View details: `href="/mentor-head/class?class_key={{urlquery $class.ClassKey}}"`. Return form: `action="/mentor-head/return"` + hidden `class_key`. |
| `internal/views/mentor.html` | View class: `href="/mentor/class?class_key={{urlquery .ClassKey}}"`. |
| `internal/views/classes.html` | Return form: `action="/classes/return"` + hidden `class_key`. |
| `cmd/server/main.go` | **Mentor-head:** `/mentor-head/class/` (dynamic) → `/mentor-head/class` (exact GET, query). `/mentor-head/{classKey}/return` (dynamic POST) → `POST /mentor-head/return` (form body). **Mentor:** `/mentor/class/` (dynamic) → `/mentor/class` (exact GET, query). **Classes:** `/classes/{classKey}/return` (dynamic POST) → `POST /classes/return` (form body). |

## 3. Route changes (old → new)

| Old | New |
|-----|-----|
| `POST /mentor-head/{classKey}/return` | `POST /mentor-head/return` (form: `class_key`) |
| `GET /mentor-head/class/{classKey}` | `GET /mentor-head/class?class_key=...` |
| `GET /mentor/class/{classKey}` | `GET /mentor/class?class_key=...` |
| `POST /classes/{classKey}/return` | `POST /classes/return` (form: `class_key`) |

**Unchanged:** `POST /mentor-head/close-round` (already uses form `class_key`). Mentor actions (attendance, grade, note, session complete) still POST with `class_key` in body; redirects now use `?class_key=...`.

## 4. Canonical classKey format

- **Generated:** `models.GenerateClassKey(level, classDays, classTime, groupIndex)` → `L%d|%s|%s|%d` (e.g. `L2|Sun/Wed|07:30:00|1`). Used when building class groups (e.g. Classes board “Send to mentor head”) and when ensuring `class_groups` rows exist.
- **Stored:** `class_groups.class_key` (and `mentor_assignments.class_key`, etc.).
- **Contains `/`:** Yes. `class_days` can be `Sun/Wed`, so classKey is **not** safe as a path segment. Using it in the path breaks routing; we avoid that by using **form body** or **query params** everywhere.

## 5. Manual test checklist (5 steps)

1. **Return removes class:** As mentor_head, open `/mentor-head`, pick a class (e.g. `L2|Sun/Wed|07:30:00|1`), click **Return to Operations** → success banner → **refresh** → that class is **gone** from the list. No 500.
2. **Ops can send again:** As admin, open `/classes`, find the same class (e.g. Sun/Wed 07:30), click **Send to Mentor Head** → success → as mentor_head, refresh `/mentor-head` → class **reappears**.
3. **View details works:** As mentor_head, click **View details** for a class (with `/` in days) → class detail loads (sessions, students, attendance, notes). **Back to Mentor Head** returns to `/mentor-head`.
4. **Mentor class detail:** As mentor, open **View class** for an assigned class (with `/` in days) → detail loads. Submit attendance / grade / note / complete session → redirect back to `/mentor/class?class_key=...` and detail still shows.
5. **Classes Return:** As admin, on `/classes` use **Return** for a “Sent to Mentor Head” class → success → class no longer “Sent to Mentor Head”; Mentor Head list no longer shows it.

---

**Summary:** classKey is no longer used in path params. Return uses `POST .../return` + form `class_key`; view-detail uses `GET .../class?class_key=...`. Encoding is via form values or `urlquery` / `QueryEscape`. Return persists (`sent_to_mentor=false`), class disappears from mentor-head after refresh, and Operations can send again.
