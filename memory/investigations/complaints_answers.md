# Complaints Workflow Investigations - ANSWERED

## Q: Complaint category/urgency values?

### Answer: **No database constraints - frontend defines values**

**Evidence**:
- `migrations/033_add_complaints_to_followups.sql:8-9` - TEXT fields with no CHECK constraint
- Frontend would define dropdown options

**Inference from design**: Complaint creation happens via Student Success but exact UI dropdown values need frontend investigation.

**Common patterns** (typical complaint systems):
- **Categories**: mentor, content, technical, admin, scheduling, student_behavior, other
- **Urgency**: low, medium, high, critical

**TODO**: Check `StudentSuccessComplaintForm` component for actual dropdowns *(file may not exist yet)*

---

## Q: Complaint routing logic?

### Answer: **Single routing: SS creates → MH handles (no escalation routing found)**

**Evidence**: `cmd/server/main.go:344-375`

**Current workflow**:
1. Student Success creates complaint via `POST /api/student-success/complaints`
2. Complaint stored in `followups` table with `type = 'complaint'`
3. Mentor Head views all complaints via `GET /api/mentor-head/complaints`
4. Mentor Head handles (update status / resolve)

**No multi-tier routing**: All complaints go to Mentor Head. No code found for:
- Escalating to different departments
- Routing based on category/urgency
- Auto-assignment logic

**Conclusion**: Simple single-tier workflow. All complaints handled by current design does not include complex routing.

---

## Q: Can resolved complaints be reopened?

### Answer: **No database constraint prevents it, but no UI/API exists**

**Evidence**:
- No CHECK constraint prevents `status` reversal from RESOLVED
- No API endpoint to change status from RESOLVED back to CONTACTED *(checked api.go)*
- Frontend `MentorHeadComplaints.tsx` doesn't show actions for resolved complaints

**Conclusion**: Technically possible in DB but not exposed in UI. To reopen would need manual DB update or new feature.

---

## Q: Why separate complaint type vs new table?

### Answer: **Code reuse - shares status tracking and case notes**

**Evidence**: `migrations/033_add_complaints_to_followups.sql`

**Shared functionality**:
- Both use `status` (NOT_CONTACTED, CONTACTED, RESOLVED)
- Both use `followup_case_notes` for audit trail
- Both need created_by, created_at tracking

**Difference**:
- Absence escalations: tied to `session_number`, automatic from attendance
- Complaints: `session_number = NULL`, manual creation

**Design benefit**: DRY principle - reuse existing follow-up workflow infrastructure instead of duplicating tables/handlers.
