# Route Access Map

**Purpose**: Map every route to its allowed roles (from middleware checks).

**Evidence Source**: `cmd/server/main.go` route definitions

---

## API Routes

| Route | Method | Allowed Roles | Handler | Evidence |
|-------|--------|---------------|---------|----------|
| `/api/me` | ANY | Authenticated | `api Handler.GetMe` | `main.go:98-101` |
| `/api/attendance` | POST | Authenticated | `apiHandler.MarkAttendance` | `main.go:104-107` |
| `/api/session/complete` | POST | mentor, mentor_head, admin, student_success | `apiHandler.CompleteSession` | `main.go:109-112` |
| `/api/mentor/classes` | GET | mentor, admin, student_success | `apiHandler.GetMentorClasses` | `main.go:114-117` |
| `/api/mentor-head/mentors` | GET | mentor_head, admin | `apiHandler.GetMentors` | `main.go:119-122` |
| `/api/mentor-head/classes` | GET | mentor_head, admin | `apiHandler.GetMentorHeadClasses` | `main.go:124-127` |
| `/api/mentor-head/dashboard` | GET | mentor_head, admin | `apiHandler.GetMentorHeadDashboard` | `main.go:129-136` |
| `/api/mentor-head/archive` | GET | mentor_head, admin | `apiHandler.GetMentorHeadArchive` | `main.go:138-145` |
| `/api/mentor-head/assign-mentor` | POST | mentor_head, admin | `apiHandler.AssignMentor` | `main.go:147-154` |
| `/api/mentor-head/return-to-ops` | POST | mentor_head, admin | `apiHandler.ReturnToOps` | `main.go:156-163` |
| `/api/mentor-head/return-class` | POST | mentor_head, admin | `apiHandler.ReturnClass` | `main.go:165-172` |
| `/api/mentor-head/unassign` | POST | mentor_head | `apiHandler.UnassignMentor` | `main.go:174-181` |
| `/api/mentor-head/start-round` | POST | mentor_head, admin | `apiHandler.StartRound` | `main.go:183-190` |
| `/api/mentor-head/classes/start-round` | POST | mentor_head, admin | `apiHandler.StartRound` | `main.go:192-199` |
| `/api/mentor-head/close-round` | POST | mentor_head, admin | `apiHandler.CloseRound` | `main.go:201-208` |
| `/api/mentor-head/reopen-round` | POST | mentor_head, admin | `apiHandler.ReopenRound` | `main.go:210-220` |
| `/api/class-workspace` | GET | mentor, mentor_head, admin, student_success | `apiHandler.GetClassWorkspace` | `main.go:210-222` |
| `/api/class` | GET | mentor, mentor_head, admin, student_success | `apiHandler.GetClass` | `main.go:224-231` |
| `/api/notes` | GET | mentor, mentor_head, admin, student_success | `apiHandler.GetNotes` | `main.go:234-244` |
| `/api/notes` | POST | mentor, mentor_head, admin, student_success | `apiHandler.CreateNote` | `main.go:234-244` |
| `/api/notes` | DELETE | mentor, mentor_head, admin, student_success | `apiHandler.DeleteNote` | `main.go:234-244` |
| `/api/student` | GET | mentor, mentor_head, admin, student_success | `apiHandler.GetStudent` | `main.go:246-253` |
| `/api/mentor-head/evaluations` | GET | mentor_head | `apiHandler.GetMentorEvaluations` | `main.go:256-268` |
| `/api/mentor-head/evaluations/:mentorId` | PUT | mentor_head | `apiHandler.UpdateMentorEvaluation` | `main.go:270-284` |
| `/api/student-success/classes` | GET | student_success | `apiHandler.GetStudentSuccessClasses` | `main.go:286-297` |
| `/api/student-success/class` | GET | student_success | `apiHandler.GetStudentSuccessClass` | `main.go:299-310` |
| `/api/student-success/placement-tests` | GET | student_success | `apiHandler.GetStudentSuccessPlacementTests` | `main.go` |
| `/api/student-success/placement-tests/complete` | POST | student_success | `apiHandler.CompletePlacementTest` | `main.go` |
| `/api/student-success/class/absence-feed` | GET | student_success, mentor_head, admin | `apiHandler.GetAbsenceFeed` | `main.go:312-315` |
| `/api/student-success/followups` | GET | student_success, mentor_head, admin | `apiHandler.GetFollowUps` | `main.go:317-326` |
| `/api/student-success/followups` | POST | student_success, mentor_head, admin | `apiHandler.CreateFollowUp` | `main.go:317-326` |
| `/api/student-success/resolve-absence` | POST | student_success, mentor_head, admin | `apiHandler.ResolveAbsence` | `main.go:328-331` |
| `/api/student-success/feedback` | POST | student_success, admin | `apiHandler.SubmitFeedback` | `main.go:333-336` |
| `/api/student-success/feedback/status` | PUT | student_success, admin | `apiHandler.UpdateFeedbackStatus` | `main.go:338-341` |
| `/api/student-success/complaints` | POST | student_success, admin | `apiHandler.CreateComplaint` | `main.go:344-347` |
| `/api/mentor-head/complaints` | GET | mentor_head, admin | `apiHandler.GetMentorHeadComplaints` | `main.go:350-357` |
| `/api/mentor-head/complaints/:id/update` | POST | mentor_head, admin | `apiHandler.UpdateComplaintStatusHandler` | `main.go:360-375` |
| `/api/mentor-head/complaints/:id/resolve` | POST | mentor_head, admin | `apiHandler.ResolveComplaintHandler` | `main.go:360-375` |
| `/api/absence-cases/:id/follow-up` | POST | student_success, mentor_head, admin | `apiHandler.PostFollowUpUpdate` | `main.go:377-389` |
| `/api/absence-cases/:id/resolve` | POST | student_success, mentor_head, admin | `apiHandler.ResolveFollowUp` | `main.go:377-389` |
| `/api/compliance/check` | POST | student_success | `apiHandler.UpsertComplianceCheck` | `main.go` |
| `/api/compliance/class/:class_key` | GET | student_success | `apiHandler.GetComplianceByClass` | `main.go` |
| `/api/reports/mentors` | GET | student_success, mentor_head, admin, manager | `apiHandler.GetMentorReports` | `main.go` |
| `/api/reports/mentors/checklist` | GET | student_success, mentor_head, admin, manager | `apiHandler.GetMentorReportChecklist` | `main.go` |
| `/api/reports/mentors/exclude` | POST | mentor_head, admin | `apiHandler.ExcludeMentorReportRow` | `main.go` |
| `/api/classes/:id/sessions` | GET | mentor, mentor_head, admin, student_success | `apiHandler.ListClassSessions` | `main.go:391-406` |
| `/api/classes/:id/sessions/:n/complete` | POST | mentor, mentor_head, admin, student_success | `apiHandler.CompleteSessionByNumber` (deprecated) | `main.go:391-406` |
| `/api/grades/preview` | GET | mentor, mentor_head, student_success, admin | `apiHandler.GetGradesPreview` | `main.go` |
| `/api/notifications/late-join` | GET | mentor, mentor_head, student_success | `apiHandler.GetLateJoinNotifications` | `main.go (new)` |
| `/api/notifications/late-join/:id/acknowledge` | POST | mentor, mentor_head, student_success | `apiHandler.AcknowledgeLateJoinNotification` | `main.go (new)` |

---

## Page Routes (SSR Templates)

| Route | Method | Allowed Roles | Handler | Evidence |
|-------|--------|---------------|---------|----------|
| `/login` | GET | Public | `authHandler.LoginForm` | `main.go:409-419` |
| `/login` | POST | Public | `authHandler.Login` | `main.go:409-419` |
| `/logout` | GET/POST | Authenticated | `authHandler.Logout` | `main.go:420-428` |
| `/pre-enrolment/new` | GET | admin, moderator | `preEnrolmentHandler.NewForm` | `main.go:432-449` |
| `/pre-enrolment/new` | POST | admin, moderator | `preEnrolmentHandler.Create` | `main.go:432-449` |
| `/pre-enrolment/:id` | GET | admin, moderator | `preEnrolmentHandler.Detail` | `main.go:453-482` |
| `/pre-enrolment/:id` | POST | admin, moderator | `preEnrolmentHandler.Update` | `main.go:453-482` |
| `/pre-enrolment` | GET | admin, moderator | `preEnrolmentHandler.List` | `main.go:484-500` |
| `/classes` | GET | admin, mentor_head | `classesHandler.List` | `main.go` |
| `/classes/move` | POST | admin | `classesHandler.Move` | `main.go:519-533` |
| `/classes/start-round` | POST | admin | `classesHandler.StartRound` | `main.go:535-549` |
| `/classes/send` | POST | admin | `classesHandler.SendToMentor` | `main.go:551-565` |
| `/classes/return` | POST | admin | `classesHandler.ReturnFromMentor` | `main.go:567-581` |
| `/finance` | GET | admin | `financeHandler.Dashboard` | `main.go` |

---

## React App Routes (Frontend)

> **Note**: React routes are protected by `RequireAuth` middleware at the app level, then role-specific rendering happens client-side.

**Evidence Source**: `frontend/src/App.tsx` and component role checks

| Route | Component | Role Requirements | Evidence |
|-------|-----------|-------------------|----------|
| `/app/mentor` | `MentorDashboard` | mentor | `App.tsx`, `AppLayout.tsx` |
| `/app/mentor-head` | `MentorHeadDashboard` | mentor_head | `App.tsx`, `AppLayout.tsx` |
| `/app/mentor-head/evaluations` | `MentorEvaluations` | mentor_head | `App.tsx`, `AppLayout.tsx` |
| `/app/mentor/class` | `ClassWorkspace` | mentor | `App.tsx` |
| `/app/mentor-head/class` | `ClassWorkspace` | mentor_head | `App.tsx` |
| `/app/community-officer` | `CommunityOfficerDashboard` | community_officer | `App.tsx`, `AppLayout.tsx` |
| `/app/hr/mentors` | `HRMentorsPage` | hr | `App.tsx`, `AppLayout.tsx` |
| `/app/pre-enrolment` | `PreEnrolmentDashboard` | student_success | `App.tsx`, `AppLayout.tsx` |
| `/app/student-success` | `StudentSuccessDashboard` | student_success | `App.tsx`, `AppLayout.tsx` |
| `/app/reports` | `ReportsPage` | student_success, mentor_head, admin | `App.tsx`, `AppLayout.tsx` |
| `/app/admin/mentors` | `AdminMentorsPage` | admin | `App.tsx`, `AppLayout.tsx` |

---

## Key Patterns

### Middleware Stack
1. **`RequireAuth`** - Validates session cookie exists
2. **`RequireAnyRole([]string{...})`** - Checks if user's role is in allowed list
3. **Handler** - Executes business logic

### Role Inheritance
- **`admin`** typically has access to most routes (except specific mentor_head-only)
- **`moderator`** has read access to pre-enrolment and finances but gets 403 on write operations
- **`mentor_head`** can access mentor-head dashboard + mentor class workspace (read-only classes board)
- **`student_success`** has access to absence feeds and follow-ups shared with mentor_head

### Legacy Mentor Head SSR
- Legacy SSR mentor-head handlers and templates are removed; `/mentor-head` and `/mentor-head/class` remain as redirects to the React app for backward compatibility.
**Evidence**: `cmd/server/main.go`

### TODO: Investigate
- [ ] `moderator` role behavior - confirm intended access after router alignment (no `/classes` or `/finance`)
- [ ] React app session validation - how does `/app/*` ensure roles match? *(check AppLayout.tsx)*

### Notes
- `/api/student` supports both `student_id` and `lead_id` query params.
- When `class_key` is passed, `/api/student` also returns `report_card` payload for printable grading justification.
- `/api/mentor-head/evaluations` supports `scope` query param:
  - `scope=active` (default) for ongoing rounds
  - `scope=closed` for post-round discussion/reporting
- Closed-scope filters on `/api/mentor-head/evaluations`:
  - `q` (mentor name/email search)
  - `from` (closed date start, `YYYY-MM-DD`)
  - `to` (closed date end, `YYYY-MM-DD`)
