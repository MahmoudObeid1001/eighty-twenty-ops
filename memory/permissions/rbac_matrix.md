# RBAC Matrix

**Purpose**: Define what each role can do in the system.

**Evidence Sources**: 
- `cmd/server/main.go` - Route middleware
- `internal/handlers/*.go` - Handler-level checks
- `frontend/src/components/AppLayout.tsx` - UI role checks

---

## Role Capabilities Matrix

| Feature | admin | moderator | mentor_head | mentor | community_officer | hr | student_success |
|---------|-------|-----------|-------------|---------|-------------------|-----|-----------------|
| **Pre-Enrolment** | | | | | | | |
| View leads list | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| View lead detail | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Create new lead | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Update lead (actions) | ✅ | ⚠️ * | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Classes Board** | | | | | | | |
| ViewAll classes | ✅ | ⚠️ * | ✅ (read-only) | ❌ | ❌ | ❌ | ❌ |
| Send to mentor | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Return from mentor | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Move class | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Start round (admin) | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Mentor Head** | | | | | | | |
| View dashboard | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| View archive | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Assign mentor to class | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Unassign mentor | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Return to ops | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Start round | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Close round | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| View/edit evaluations | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Mentor** | | | | | | | |
| View my classes | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| Open class workspace | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| Mark attendance | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| Complete session | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| Add/delete notes | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| View student details | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| Final grading (edit) | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ❌ |
| **Student Success** | | | | | | | |
| View classes list | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| View class detail | ✅ | ❌ | ✅ (read-only) | ❌ | ❌ | ❌ | ✅ |
| View absence feed | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ✅ |
| Create follow-up | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ✅ |
| Resolve absence | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ✅ |
| Submit feedback | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| Update feedback status | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| Upload feedback collected | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| Remove feedback collected | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| View feedback collected | ✅ | ❌ | ✅ (read-only) | ❌ | ❌ | ❌ | ✅ |
| View placement tests queue | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| Record placement test results | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| **Complaints** | | | | | | | |
| Create complaint | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |
| View complaints list | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Update complaint status | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| Resolve complaint | ✅ | ❌ | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Finance** | | | | | | | |
| View dashboard | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **Community Officer** | | | | | | | |
| View CO dashboard | ✅ | ❌ | ❌ | ❌ | ✅ | ❌ | ❌ |
| **HR** | | | | | | | |
| View HR mentors page | ✅ | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ |
| **Notifications** | | | | | | | |
| View late joiner alerts | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |
| Acknowledge alerts | ❌ | ❌ | ✅ | ✅ | ❌ | ❌ | ✅ |

---

## Evidence Notes

### ⚠️ Moderator Special Cases

**Evidence**: `cmd/server/main.go` route definitions, handler checks

- Moderator is restricted to Pre-Enrolment only (read-only for certain actions).
- Moderator no longer has `/classes` or `/finance` access at the router level.

### Admin Privileges

**Evidence**: Route definitions throughout `main.go`

- Admin has access to nearly all endpoints
- **Exception**: Cannot unassign mentors (mentor_head only)
- **Exception**: Cannot view/edit mentor evaluations (mentor_head only)

### Shared Access student_success + mentor_head

**Evidence**: `main.go:312-389`

Both roles share access to:
- Student Success class detail (`/api/student-success/class`) for viewing tabs (mentor_head read-only)
- Absence feed (`/api/student-success/class/absence-feed`)
- Follow-ups (`/api/student-success/followups`)
- Resolve absence (`/api/student-success/resolve-absence`)
- Absence case actions (`/api/absence-cases/:id/*`)

**Reasoning**: mentor_head needs visibility into student success workflows

### Class Workspace Multi-Role Access

**Evidence**: `main.go:217`, `main.go:226`

Allowed: `mentor`, `mentor_head`, `admin`, `student_success`

**Reasoning**: 
- Mentor needs it for teaching
- Mentor Head needs read access for oversight
- Student Success needs it for absence tracking
- Admin has god-mode access

---

## Role Definitions

| Role | Primary Purpose | Evidence |
|------|-----------------|----------|
| `admin` | System administrator - full control | `migrations/001_init.sql` |
| `moderator` | Read-only operations assistant | `migrations/001_init.sql` |
| `mentor_head` | Manages mentors and class assignments | `migrations/014_add_mentor_roles.sql` |
| `mentor` | Teaches classes, tracks attendance | `migrations/014_add_mentor_roles.sql` |
| `community_officer` | Community engagement | `migrations/014_add_mentor_roles.sql` |
| `hr` | HR operations for mentors | `migrations/023_add_hr_role.sql` |
| `student_success` | Student support and follow-ups | `migrations/028_add_student_success_role.sql` |

---

## TODOs

- [ ] Document `community_officer` actual capabilities (routes exist but unclear functionality)
- [ ] Document `hr` actual capabilities (routes exist but unclear functionality) 
- [ ] Clarify moderator's intended vs actual permissions (403 discrepancy)
- [ ] Verify if `admin` should have `/mentor-head/evaluations` access or if truly mentor_head-only
