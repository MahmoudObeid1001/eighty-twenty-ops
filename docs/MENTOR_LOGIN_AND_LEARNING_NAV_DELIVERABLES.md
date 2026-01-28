# Mentor Login + Learning Nav – Deliverables

## TASK 1 — Fix mentor login

### Root cause (2–3 sentences)

Login and HR create both use bcrypt (same hashing), and HR stores hashed passwords. The failure came from **email lookup**: `GetUserByEmail` used `WHERE email = $1` (case-sensitive). HR stores normalized (lowercased) email; login used `r.FormValue("email")` as-is. If the user logged in with different case (e.g. `Mentor@test.com` vs `mentor@test.com`), the lookup failed, we showed “Invalid email or password,” and we never reached password verification. Additional factors: no trim (spaces), and we didn’t log the real error.

### Fixes applied

1. **Email normalization:** `normalizeEmail(s) = strings.ToLower(strings.TrimSpace(s))` in `auth.go`. Login normalizes email before `GetUserByEmail`. HR normalizes before “email exists” check and before `CreateUser`, so we **store** normalized email for new mentors.
2. **Case-insensitive lookup:** `GetUserByEmail` now uses `WHERE LOWER(TRIM(email)) = LOWER(TRIM($1))` so we find users regardless of stored case (seed users unchanged, HR users normalized).
3. **Server-side logging:** On login failure we log `LOGIN: user not found or db error email=...` or `LOGIN: password mismatch email=...`. User still sees “Invalid email or password.”
4. **HR create:** Normalize email, use same bcrypt as seed, log on hash/DB errors. Passwords are always hashed; we never store raw passwords.

### Exact files changed (Task 1)

| Path | Changes |
|------|---------|
| `internal/handlers/auth.go` | Add `normalizeEmail`; `log` import. Login: normalize email, call `GetUserByEmail`/`CompareHashAndPassword`, log on failure; keep “Invalid email or password” for user. |
| `internal/handlers/hr.go` | Normalize email before exists check and `CreateUser`. Log “HR create mentor: hash failed” / “db insert failed” on error. |
| `internal/models/repository.go` | `GetUserByEmail`: `WHERE LOWER(TRIM(email)) = LOWER(TRIM($1))` for case-insensitive lookup. |

### Manual test steps (Task 1)

1. Log in as HR → open `/hr/mentors`.
2. Create mentor: email `mentor2@test.com`, password `mentor2pass` → “Mentor created successfully.”
3. Log out.
4. Log in with `mentor2@test.com` / `mentor2pass` → **success**, redirect to `/mentor` (Learning).
5. Log out, log in with `Mentor2@test.com` (different case) / `mentor2pass` → **success** (case-insensitive lookup).
6. Log in with `mentor2@test.com` / wrong password → “Invalid email or password”; server log shows `LOGIN: password mismatch email=...`.

---

## TASK 2 & 3 — Learning nav + /learning redirect

### Sidebar rules (Task 2)

- **mentor:** Learning → `/learning` + Logout.
- **mentor_head:** Learning → `/learning` + Classes → `/classes` + Logout.
- **hr:** Learning → `/learning` + Logout.
- **admin:** Unchanged (Pre-Enrolment, Classes, Finance, Learning placeholder, Reports placeholder).
- **moderator:** Pre-Enrolment only.
- **community_officer:** Learning → `/learning` + Logout.

### /learning redirect (Task 3)

- **GET /learning:** Requires auth, redirects to `RoleHomePath(role)`:
  - **mentor** → `/mentor`
  - **mentor_head** → `/mentor-head`
  - **hr** → `/hr/mentors`
  - **community_officer** → `/community-officer`
  - **admin / moderator** → `/pre-enrolment`

Sidebar “Learning” links to `/learning` for mentor, mentor_head, hr, community_officer. No guessing of URLs.

### Exact files changed (Tasks 2 & 3)

| Path | Changes |
|------|---------|
| `internal/handlers/auth.go` | `LearningRedirect`: GET, redirect to `RoleHomePath(GetUserRole(r))`. `roleCanAccessPath`: allow `/learning` for mentor, mentor_head, hr, community_officer. |
| `internal/views/layout.html` | Learning links for mentor, mentor_head, hr, community_officer point to `/learning` (not direct /mentor, /mentor-head, etc.). |
| `cmd/server/main.go` | Register **GET /learning** → `RequireAuth(authHandler.LearningRedirect)`. |

### Confirmations

- **Role-based sidebar:** mentor sees Learning + Logout; mentor_head sees Learning + Classes + Logout; hr sees Learning + Logout; admin/moderator unchanged.
- **mentor_head Learning:** Clicks Learning → `/learning` → redirect to `/mentor-head` → **no 403**; Mentor Head dashboard loads.
- **Mentors land on /mentor:** Login as mentor → redirect to `RoleHomePath("mentor")` = `/mentor`. Root `/` also redirects to role home.

---

## Summary

- **Task 1:** HR-created mentors can log in. Email normalized on create and login; case-insensitive lookup; bcrypt only; login failures logged, user message unchanged.
- **Task 2:** Sidebar is role-based; Learning tab and territory access as above.
- **Task 3:** `/learning` redirects by role; mentor_head uses Learning → `/mentor-head` with no 403.

Finance permissions and Milestone 1 (pre-enrolment + moderator) are unchanged.
