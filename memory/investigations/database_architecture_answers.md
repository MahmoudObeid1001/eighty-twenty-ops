# Database & Architecture Investigations - ANSWERED

## Q: Soft delete fields - should they be removed?

### Answer: **No - keep for future extensibility**

**Context**: `followups` table has soft delete fields (deleted_at, deleted_by_user_id, delete_reason)  
**Origin**: Added for manager role's delete complaint feature (now removed)

**Reasons to keep**:
1. **Low cost**: 3 nullable columns, minimal storage overhead
2. **Future use**: May want soft delete for data retention/audit
3. **Migration risk**: Removing columns requres migration + potential data loss
4. **Schema stability**: Better to have unused fields than to add/remove repeatedly

**Conclusion**: Keep fields. Cost is minimal, benefit of having them available outweighs removal effort.

---

## Q: Session completion - why two APIs?

### Answer: **Different use cases / legacy compatibility**

**Evidence**: `cmd/server/main.go:109-112`, `391-406`

### API 1: `/api/session/complete` (POST)
**Payload**: `{ session_id }`  
**Use case**: Complete by session UUID  
**Frontend**: Likely older implementation

### API 2: `/api/classes/:id/sessions/:n/complete` (POST)
**Payload**: Path params only  
**Use case**: Complete by class_key + session_number  
**Frontend**: More REST-ful design  
**Evidence**: `apiHandler.CompleteSessionByNumber`

**Why both**:
- **Backward compatibility**: Older code uses session_id
- **Different contexts**: Some UI has session_id readily available, others have class_key + number
- **No harm**: Both call same underlying logic

**Conclusion**: Multiple APIs for same action is fine for flexibility. No need to remove either unless consolidating.

---

## Q: AppLayout role checks vs middleware - what prevents manual navigation?

### Answer: **Backend middleware is the real enforcement**

**Evidence**:
- `frontend/src/components/AppLayout.tsx` - Client-side nav rendering
- `cmd/server/main.go` - RequireAuth + RequireAnyRole middleware

**Security model**:
1. **Frontend** (AppLayout): Hides nav links for unauthorized routes (UX only)
2. **Backend** (Middleware): Enforces access with 403 responses (security)

**User tries manual navigation**:
```
User types /app/admin/mentors in browser
→ React router renders component
→ Component makes API call
→ Backend middleware checks role
→ Returns 403 if unauthorized
→ Frontend shows error
```

**Conclusion**: Client-side checks are UX nice-to-have. Backend is the real security boundary. This is correct security architecture.

---

## Q: React app vs SSR templates - migration strategy?

### Answer: **No migration - intentional dual architecture**

**Evidence**:
- `cmd/server/main.go` - Both SSR routes (`/pre-enrolment`, `/classes`) and React catch-all (`/app/*`)
- Different user workflows use different approaches

**Current split**:
- **SSR templates**: Admin workflows (pre-enrolment, classes board, finance)
- **React SPA**: Role-specific dashboards (mentor, mentor_head, student_success, etc.)

**Reasoning** (inferred):
- **SSR**: Traditional server-rendered pages for admin operations
- **React**: Interactive dashboards for role-based workflows

**Conclusion**: No migration planned. System intentionally supports both. New features should match pattern of their workflow type (admin ops = SSR, dashboards = React).

---

## Q: Grades table - how is it used?

### Answer: **Planned feature, not yet implemented**

**Evidence**:
- `migrations/018_create_grades.sql` - Table exists
- No API endpoints found in `cmd/server/main.go`
- No frontend components reference grades

**Status**: Database schema ready but feature inactive.

**TODO**: Either implement grade tracking or document as "future feature".

---

## Q: Finance tables - incomplete documentation?

### Answer: **Yes - finance module needs expansion**

**Evidence**:
- `migrations/007_finance_tracking.sql`
- `migrations/008_finance_ledger_sync.sql`
- These were not fully documented in ERD

**TODO**: Expand memory documentation with finance-specific tables and workflows.

**Status**: Known gap in current memory system - finance module less documented than other features.
