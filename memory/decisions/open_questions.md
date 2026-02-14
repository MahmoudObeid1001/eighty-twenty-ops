# Open Questions

**Purpose**: Document inconsistencies, TODOs, and areas needing clarification.

**Rule**: If evidence cannot be found or behavior is unclear, document it here.

---

## Architecture & Design

### Q: React app vs SSR templates - migration strategy?
**Observation**: System has both:
- SSR templates (pre-enrolment, classes, finance) at `/`
- React SPA at `/app/*`  
**Questions**:
- Is there a migration plan to move everything to React?
- Why keep both approaches?
- Which is preferred for new features?  
**Evidence**: 
- `cmd/server/main.go` shows both SSR routes and React app catch-all
- `frontend/src/App.tsx` shows React routes

---

## Roles & Permissions

### Q: Moderator role - what's the actual intent?
**Observation**: Moderator allowed by middleware but gets 403 in handlers  
**Affected routes**: `/classes`, `/finance`, pre-enrolment updates  
**Questions**:
- Is moderator meant to be read-only?
- Should middleware reject moderator, or should handlers allow it?
- Is this a bug or intentional design?  
**Evidence**: 
- `cmd/server/main.go:512-517` - classes route allows moderator
- Handler logic (TODO: confirm) rejects moderator  
**Next steps**: Check `internal/handlers/classes.go`, `finance.go`, `pre_enrolment.go` for role checks

### Q: Admin vs Mentor Head - evaluation access?
**Observation**: Only mentor_head can access `/api/mentor-head/evaluations`  
**Questions**:
- Is this intentional that admin cannot view/edit evaluations?
- Or should admin have access like other mentor_head endpoints?  
**Evidence**: `cmd/server/main.go:263` - RequireAnyRole(["mentor_head"]) excludes admin

### Q: Community Officer & HR roles - what do they actually do?
**Observation**: Roles exist in DB and routes defined, but functionality unclear  
**Questions**:
- What is community_officer's actual workflow?
- What is hr's actual workflow?
- Are these roles actively used?  
**Evidence**: 
- `migrations/014_add_mentor_roles.sql` - roles added
- Routes exist but no workflow diagrams
- RBAC matrix has empty cells

---

## Class Lifecycle

### Q: When are 8 sessions created?
**Observation**: Each class needs 8 `class_sessions` records  
**Questions**:
- Created at class creation time?
- Created when round starts?
- Created when mentor is assigned?  
**Evidence**: `migrations/016_create_class_sessions.sql` shows table but not creation trigger  
**Next steps**: Check class creation handlers and StartRound handler

### Q: Can a closed round be reopened?
**Observation**: Database allows `round_status` to change from CLOSED back to IN_PROGRESS  
**Questions**:
- Is there a UI/API to reopen a closed round?
- Under what conditions would you reopen?  
**Evidence**: No CHECK constraint prevents status reversal  
**Next steps**: Check frontend and API handlers

### Q: Return to ops vs unassign mentor - what's the difference?
**Observation**: Two similar-sounding operations:
- `/api/mentor-head/return-to-ops` - returns class to ops
- `/api/mentor-head/unassign` - unassigns mentor  
**Questions**:
- Do both do the same thing?
- When would you use one vs the other?
- What's the state difference?  
**Evidence**: `cmd/server/main.go:156-181` - both routes exist  
**Next steps**: Check handler implementations

---

## Attendance & Absences

### Q: Automatic follow-up creation?
**Observation**: Student Success can create follow-ups from absence feed  
**Questions**:
- Is follow-up created automatically when student marked absent_unexcused?
- Or does SS manually create it?  
**Evidence**: 
- `migrations/029_create_followups_table.sql` - no trigger defined
- API has CreateFollowUp endpoint suggesting manual  
**Next steps**: Check MarkAttendance handler for side effects

### Q: Escalation thresholds?
**Observation**: StudentSuccess tracks absences and creates follow-ups  
**Questions**:
- Are there escalation rules (e.g., 3+ absences → alert)?
- Is urgency/priority calculated automatically?  
**Evidence**: No CHECK constraints or triggers visible in migrations  
**Next steps**: Check business logic in handlers

## Complaints

### Q: Complaint category/urgency values?
**Observation**: Fields exist but no constraints  
**Questions**:
- What are valid category values?
- What are valid urgency values?  
**Evidence**: `migrations/033_add_complaints_to_followups.sql:8-9` - TEXT fields, no CHECK  
**Next steps**: Check frontend dropdown options

### Q: Complaint routing logic?
**Observation**: Student Success creates complaints, Mentor Head handles  
**Questions**:
- How does SS decide if complaint goes to Mentor Head vs escalated elsewhere?
- Are there different complaint types that route differently?  
**Evidence**: Only one workflow documented  
**Next steps**: Check business documentation or stakeholder input

### Q: Can resolved complaints be reopened?
**Observation**: Status can be RESOLVED  
**Questions**:
- Can status change from RESOLVED back to CONTACTED?
- Is there a UI for this?  
**Evidence**: No CHECK constraint prevents status reversal  
**Next steps**: Check frontend UI and handler validation

---

## Database & Schema

### Q: Soft delete - why exists if unused?
**Observation**: `followups` table has soft delete fields (deleted_at, etc.)  
**Question**: Should this feature be removed since manager role is gone?  
**Evidence**: `migrations/033_add_complaints_to_followups.sql:14-16`  
**Impact**: Schema bloat for unused feature

### Q: Grades table - how is it used?
**Observation**: `migrations/018_create_grades.sql` exists  
**Questions**:
- What grades are tracked?
- Who enters grades?
- Where are they displayed?  
**Evidence**: Migration exists but no workflow describes usage  
**Next steps**: Check ClassWorkspace UI and API

### Q: Finance tables - incomplete documentation?
**Observation**: Finance migrations exist (007_finance_tracking, 008_finance_ledger_sync)  
**Questions**:
- What tables were added?
- What is the finance workflow?  
**Evidence**: Migrations exist but not fully documented in ERD  
**Next steps**: Review finance migrations and expand documentation

---

## Frontend/Backend Sync

### Q: Session completion - two APIs?
**Observation**: Two endpoints for session completion:
- `POST /api/session/complete`
- `POST /api/classes/:id/sessions/:n/complete`  
**Questions**:
- Why two endpoints for same action?
- When is each used?
- Do they do the same thing?  
**Evidence**: `cmd/server/main.go:109-112`, `391-406`  
**Next steps**: Check which frontend uses which

### Q: AppLayout role checks vs middleware?
**Observation**: Frontend `AppLayout.tsx` has role-based nav rendering  
**Question**: What prevents user from manually navigating to forbidden routes?  
**Evidence**: 
- Frontend does client-side role checks
- Backend has middleware checks  
**Assumption**: Backend middleware is the real enforcement  
**Next steps**: Confirm React router doesn't bypass auth

### Q: Mentor Evaluations scope mismatch (mentor-level vs class-level)?
**Observation**: Mentor Evaluations page (`/app/mentor-head/evaluations`) currently stores one manual evaluation row per mentor (`mentor_evaluations.mentor_id UNIQUE`) and shows assigned class count from all assignments, not active-only classes.
**User Impact**:
- Mentor card can show class count higher than active workload (e.g., includes closed classes).
- Evaluation remains visible even when all classes are closed.
- Metrics are partly manual and not aligned with Student Success compliance data.
**Questions**:
- Should evaluations be class-scoped (mentor + class_key) and shown only for active classes?
- Which metrics remain manual and which must be auto-computed from `mentor_session_checks`?
**Evidence**:
- `internal/db/migrations/025_create_mentor_evaluations.sql` (UNIQUE mentor_id)
- `internal/models/repository.go` (`GetAssignedMentors`, `UpsertMentorEvaluation`)
- `internal/handlers/api.go` (`GetMentorEvaluations`, `UpdateMentorEvaluation`)
- `frontend/src/pages/MentorEvaluations.tsx`

---

## Migration Gaps

### Q: Missing migrations for some tables?
**Observation**: Tables like `students`, `mentors` are referenced but no CREATE TABLE found  
**Questions**:
- Do these tables exist?
- Are they views?
- Are leads used as students?  
**Evidence**: API handlers reference students but schema unclear  
**Next steps**: Check if `leads` table serves as students

---

## Performance & Scalability

### Q: Index strategy for large tables?
**Observation**: Some indexes defined, but no comprehensive strategy documented  
**Questions**:
- Are there performance issues with large classes?
- What tables grow unbounded?
- Query optimization done?  
**Evidence**: Migrations show some indexes but not comprehensive  
**Next steps**: Performance profiling

---

## Action Items

For each open question above:
1. Investigate code/migrations for evidence
2. If evidence found: move to `business_rules.md` with citation
3. If evidence not found but behavior confirmed: add to business_rules as "TODO: verify"
4. If truly unclear: keep here and flag for stakeholder clarification
