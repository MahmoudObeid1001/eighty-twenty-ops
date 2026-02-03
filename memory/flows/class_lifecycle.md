# Class Lifecycle Workflow

**Purpose**: Shows how a class moves through states from creation → closed.

**Evidence Sources**:
- `migrations/005_class_groups_workflow.sql` - class_groups table
- `migrations/027_class_groups_round_status.sql` - round_status field
- `internal/handlers/api.go` - Handler functions
- `cmd/server/main.go` - Route definitions

---

## State Diagram

```mermaid
stateDiagram-v2
    [*] --> Created: Admin creates class group
    
    Created --> SentToMentor: Admin sends to mentor head
    SentToMentor --> Assigned: Mentor Head assigns mentor
    Assigned --> RoundNotStarted: Class has mentor
    
    RoundNotStarted --> RoundInProgress: Mentor Head starts round
    RoundInProgress --> SessionsInProgress: Mentor teaches sessions (1-8)
    SessionsInProgress --> AllSessionsComplete: 8/8 sessions completed
    AllSessionsComplete --> RoundClosed: Mentor Head closes round
    
    RoundClosed --> [*]: Class archived (removed from Mentor active feed)
    
    SentToMentor --> ReturnedToOps: Too many students cause this state.
    Assigned --> ReturnedToOps: Mentor Head unassigns mentor
    
    ReturnedToOps --> SentToMentor: Admin re-sends
    
    note right of SentToMentor: sent_to_mentor = true
    note right of Assigned: mentor_assignment exists
    note right of RoundNotStarted: round_status = 'not_started'
    note right of RoundInProgress: round_status = 'active'
    note right of RoundClosed: round_status = 'closed'
```

---

## Evidence

### State: Created
**How**: Admin creates class via `/classes` operations  
**Database**: `class_groups` record created with `sent_to_mentor = false`  
**Evidence**: 
- `migrations/005_class_groups_workflow.sql:2-12`
- Database schema has `sent_to_mentor BOOLEAN DEFAULT false`

### Transition: Admin sends to mentor head (Created → SentToMentor)
**Route**: `POST /classes/send`  
**Handler**: `classesHandler.SendToMentor`  
**Database**: Updates `class_groups.sent_to_mentor = true`, sets `sent_at` timestamp.  
**Effect**: 
- Makes the class visible in the Mentor Head's Learning dashboard. 
- Allows assignment of a mentor and starting the round.
- **In OP Admin Board**: The class **stays visible** throughout its lifecycle.
    - If `sent_to_mentor` is true: it becomes **semi-transparent (opacity: 0.55)** and controls are locked.
    - If `round_status` is 'active': it shows a **Blue "STARTED" badge** and remains locked for editing to maintain sync with the active round. This ensures Ops admins can always see the status of students after they join classes.
**Evidence**:
- `internal/views/classes.html:52, 156-162` (CSS opacity and locked controls)
- `internal/handlers/classes.go:215` - calls `models.SendClassGroupToMentor`

### Related Rule: Send-to-Classes (lead) does NOT join active rounds
**Scope**: Pre-Enrolment "Send to Classes" action (lead → classes board).  
**Behavior**: Assigns the lead to a non-started class group for the same level/days/time (or opens a new group if all are full).  
**Active rounds**: Joining a running class requires the Late Joiner flow; Send-to-Classes should not place students into `round_status = 'active'` groups.  
**Evidence**: `internal/models/repository.go` (`AssignClassGroup`, `MoveStudentBetweenGroups`), `internal/views/pre_enrolment_detail.html` note.

### Transition: Mentor Head assigns mentor (SentToMentor → Assigned)
**Route**: `POST /api/mentor-head/assign-mentor`  
**Handler**: `apiHandler.AssignMentor`  
**Database**: Creates record in `mentor_assignments` table  
**Evidence**:
- `cmd/server/main.go:147-154`
- `internal/handlers/api.go` - AssignMentor function
- `migrations/022_create_mentor_assignments.sql`

### Transition: Mentor Head starts round (RoundNotStarted → RoundInProgress)
**Route**: `POST /api/mentor-head/start-round`  
**Handler**: `apiHandler.StartRound`  
**Database**: Updates `class_groups.round_status = 'IN_PROGRESS'`  
**Prerequisite**: Mentor must be assigned before starting the round.  
**Evidence**:
- `cmd/server/main.go:183-190`
- `internal/handlers/api.go` - StartRound function
- `migrations/027_class_groups_round_status.sql` - Added round_status field

### State: Sessions In Progress
**How**: Mentor marks attendance and completes sessions (1-8)  
**Routes**: 
- `POST /api/attendance` (mark attendance)
- `POST /api/session/complete` (complete session)
- `POST /api/classes/:id/sessions/:n/complete` (complete by number)  
**Database**: 
- `attendance` records created
- `class_sessions.status` updated to 'completed'
- `class_sessions.completed_at` timestamp set  
**Evidence**:
- `cmd/server/main.go:104-112`, `391-406`
- `migrations/016_create_class_sessions.sql`
- `migrations/017_create_attendance.sql`

### Transition: Mentor Head closes round (AllSessionsComplete → RoundClosed)
**Route**: `POST /api/mentor-head/close-round`  
**Handler**: `apiHandler.CloseRound`  
**Database**: Updates `class_groups.round_status = 'CLOSED'`, sets `closed_at` timestamp, sets `closed_by_mentor_user_id`  
**Evidence**:
- `cmd/server/main.go:201-208`
- `internal/handlers/api.go` - CloseRound function
- `migrations/027_class_groups_round_status.sql:5-7` - Added closed_at, closed_by_mentor_user_id
- `migrations/032_add_closed_mentor_user_id.sql`

### Archived Classes Filter (Mentor Head)
**Route**: `GET /api/mentor-head/archive`  
**Filters**: Optional `from` and `to` (YYYY-MM-DD) to filter by `round_closed_at` date (inclusive).  
**Sort**: `sort=oldest|newest` on `round_closed_at`.  
**Evidence**: `internal/handlers/api.go` (`GetMentorHeadArchive`), `internal/models/repository.go` (`GetArchivedClassGroups`).

### Transition: Return to Ops (SentToMentor/Assigned → ReturnedToOps)
**Route**: `POST /api/mentor-head/return-to-ops` or `POST /classes/return`  
**Handler**: `apiHandler.ReturnToOps` or `classesHandler.ReturnFromMentor`  
**Database**: Updates `class_groups.sent_to_mentor = false`, sets `returned_at` timestamp  
**Guard**: Class cannot be returned if `round_status = 'active'` (started rounds must be closed or archived, not returned).  
**Evidence**:
- `cmd/server/main.go:156-163` (mentor-head)
- `cmd/server/main.go:567-581` (admin)
- `internal/handlers/api.go` - ReturnToOps function

### Transition: Mentor Head unassigns mentor (Assigned → ReturnedToOps)
**Route**: `POST /api/mentor-head/unassign`  
**Handler**: `apiHandler.UnassignMentor`  
**Database**: Deletes record from `mentor_assignments`, updates `class_groups.sent_to_mentor = false`  
**Evidence**:
- `cmd/server/main.go:174-181`
- `internal/handlers/api.go` - UnassignMentor function

---

## Key Business Rules

### Rule: One mentor per class
**Evidence**: `migrations/022_create_mentor_assignments.sql:9`  
**Constraint**: `UNIQUE (class_key)` on `mentor_assignments` table

### Rule: Round status progression
**Evidence**: `migrations/027_class_groups_round_status.sql`  
**States**: not_started → active → closed  
**CHECK constraint**: `round_status IN ('not_started', 'active', 'closed')`

### Rule: 8 sessions per class
**Evidence**: `migrations/016_create_class_sessions.sql`  
**Implementation**: When class is created, 8 session records are initialized  
**TODO**: Verify exact session creation logic in handlers

### Rule: Can only close round after all sessions complete
**Evidence**: Handler logic (TODO: verify in `internal/handlers/api.go` - CloseRound)  
**Expected behavior**: Frontend/backend should check all 8 sessions are completed before allowing close.

### Rule: Active Classes Feed Filtering
**Implementation**: 
- **Mentor/Mentor-Head Views**: Classes are visible only when `sent_to_mentor = true` and not closed.
- **Student Success Views**: Should only show classes when **Mentor Head has started the round** (`round_status = 'active'`) **and** `sent_to_mentor = true`.
- **OP Admin Classes Board**: Classes remain visible even after they start (`round_status = 'active'`) with a blue **STARTED** badge, but they are read-only to prevent out-of-sync roster mid-round.
- **OP Admin Archive**: Admin can hide classes from the Ops board **only if** `sent_to_mentor = true` **and** `round_status = 'active'`. Archived classes are reversible via Ops archive view (filterable by class_key and date).
**Evidence**: `models.GetClassGroups`, `models.GetClassGroupsSentToMentor`, `models.GetMentorClasses`, `internal/views/classes.html`

---

## Open Questions

- [ ] **Session creation timing**: When are the 8 `class_sessions` records created? At class creation? At round start? *(check handler code)*
- [ ] **Return to ops conditions**: What are the exact conditions that trigger return to ops? *(check business logic)*
- [ ] **Unassign vs return**: What's the difference between unassigning mentor vs returning class to ops? *(seems overlapping)*
- [ ] **Round restart**: Can a closed round be restarted? *(database allows, but is it exposed in UI?)*
