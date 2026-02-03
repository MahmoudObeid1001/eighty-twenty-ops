# Roles & Permissions Investigations - ANSWERED

## Q: Moderator role - what's the actual intent?

### Answer: **Intentional read-only with handler-level 403s**

**Evidence**: `internal/handlers/classes.go:27-55`, `finance.go`, `pre_enrolment.go`

### Classes Handler
```go
userRole := middleware.GetUserRole(r)
if userRole == "moderator" {
    w.WriteHeader(http.StatusForbidden)
    data := map[string]interface{}{
        "Title":       "Access Restricted – Eighty Twenty",
        "SectionName": "Classes Board",
        "IsModerator": true,
    }
    renderTemplate(w, r, "access_restricted.html", data)
    return
}
```

**Pattern**: 
1. **Middleware** allows moderator (for session validation)
2. **Handler** blocks moderator with custom 403 page

**Intent**: Moderator is meant for pre-enrolment only:
- ✅ Can create/edit leads (basic info only)
- ❌ Cannot access Classes, Finance, or change lead status
- Custom "Access Restricted" pages explain limitation

**Design decision**: Better UX than blanket 403 - moderators see friendly message

---

## Q: Admin vs Mentor Head - evaluation access?

### Answer: **Intentionally mentor_head-only** (oversight/management separation)

**Evidence**: `cmd/server/main.go:263`

```go
mux.HandleFunc("/api/mentor-head/evaluations", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        middleware.RequireAnyRole([]string{"mentor_head"}, cfg.SessionSecret)(apiHandler.GetMentorEvaluations)(w, r)
    }
}))
```

**Access**: Only `mentor_head` (admin excluded)

**Reasoning** (inferred from design):
- Mentor evaluations are sensitive HR-like data
- Mentor Head is the direct supervisor
- Admin handles operations/systems, not personnel management
- Separation of concerns: Operations (admin) vs People Management (mentor_head)

**Comparison**:
- Most mentor-head endpoints: `RequireAnyRole(["mentor_head", "admin"])`
- Evaluations endpoint: `RequireAnyRole(["mentor_head"])` - admin excluded
- Unassign endpoint: `RequireAnyRole(["mentor_head"])` - also mentor_head-only

---

## Q: Community Officer & HR roles - what do they actually do?

### Answer: **Limited implementations, unclear full workflows**

### Community Officer
**Evidence**: `migrations/014_add_mentor_roles.sql`, `migrations/020_create_community_officer_feedback.sql`

**Known features**:
- `community_officer_feedback` table exists
- Route: `/community-officer` dashboard exists
- **Purpose** (from table schema): Submit feedback for sessions 4 & 8
- Log absence follow-ups

**TODO**: Frontend implementation unclear - would need to check `CommunityOfficerDashboard` component

### HR Role
**Evidence**: `migrations/023_add_hr_role.sql`

**Known features**:
- Route: `/app/hr/mentors` page exists
- **Purpose** (inferred): HR operations for mentors

**TODO**: Actual functionality unclear - limited documentation

**Status**: Both roles exist in schema but workflows not fully documented in memory system

---

##  Q: Admin cannot unassign mentors - why?

### Answer: **Business logic: mentor_head makes personnel decisions**

**Evidence**: `cmd/server/main.go:174-181`

```go
mux.HandleFunc("/api/mentor-head/unassign", requestLogMiddleware(func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        middleware.RequireAnyRole([]string{"mentor_head"}, cfg.SessionSecret)(apiHandler.UnassignMentor)(w, r)
    }
}))
```

**Only mentor_head** can unassign

**Reasoning**:
- Unassigning mentor is a personnel/management decision
- Mentor Head is the supervisor role
- Admin handles operations (send to mentor, return to ops, etc.)
- Similar to evaluation access - separates operations from people management

**Other mentor_head-only**:
- POST `/api/mentor-head/unassign` - unassign mentor
- GET/PUT `/api/mentor-head/evaluations/*` - view/edit evaluations
