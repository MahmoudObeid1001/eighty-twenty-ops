# Investigations Summary

**Purpose**: Quick reference of all answered open questions from the agent memory system.

---

## Investigation Files

| File | Questions Answered |
|------|-------------------|
| [`class_lifecycle_answers.md`](class_lifecycle_answers.md) | Sessions creation timing, round reopening, return vs unassign, double-booking |
| [`roles_permissions_answers.md`](roles_permissions_answers.md) | Moderator intent, evaluation access, CO/HR roles, admin restrictions |
| [`attendance_answers.md`](attendance_answers.md) | Auto follow-up, escalation rules, attendance deadlines, grades usage |
| [`complaints_answers.md`](complaints_answers.md) | Category/urgency values, routing logic, reopening resolved, dual-table design |
| [`database_architecture_answers.md`](database_architecture_answers.md) | Soft delete, dual APIs, frontend security, SSR vs React, grades, finance |

---

## Quick Answers

### Class Lifecycle
✅ **Sessions created at**: StartRound (8 weekly sessions)  
✅ **Return vs Unassign**: Return=anytime, Unassign=before round starts only  
✅ **Double-booking**: Database trigger prevents  
✅ **Reopen closed round**: No UI/API (DB allows but not exposed)

### Roles & Permissions
✅ **Moderator 403s**: Intentional - read-only pre-enrolment role  
✅ **Evaluation access**: mentor_head-only (admin excluded for personnel separation)  
✅ **CO/HR roles**: Limited implementations, unclear workflows  
✅ **Admin can't unassign**: Personnel decisions reserved for mentor_head

### Attendance & Absences
✅ **Auto follow-up**: No - manual creation by Student Success  
✅ **Escalation rules**: No automatic thresholds  
✅ **Attendance deadline**: No enforcement  
✅ **Grades**: Table exists, feature not implemented  
✅ **Past attendance edits**: Allowed (no restrictions)

### Complaints
✅ **Category/urgency**: No DB constraints (frontend defines)  
✅ **Routing**: Single-tier (SS → MH, no multi-tier)  
✅ **Reopening**: No UI/API (DB allows but not exposed)  
✅ **Dual-table design**: Code reuse for status tracking

### Database & Architecture
✅ **Soft delete**: Keep for future (low cost)  
✅ **Two complete APIs**: Backward compatibility + flexibility  
✅ **Frontend security**: Backend middleware is real enforcement  
✅ **SSR vs React**: Intentional split (admin=SSR, dashboards=React)  
✅ **Finance docs**: Incomplete (known gap)

---

## Status Updates

### Resolved Questions: **20+**
All major questions from `decisions/open_questions.md` have been investigated and answered with code evidence.

### Remaining TODOs:
- Community Officer complete workflow  
- HR complete workflow
- Finance module expansion
- Grades feature implementation status
- Complaint dropdown values (need frontend check)

---

## Evidence Quality

All answers include:
- ✅ File paths with line numbers
- ✅ Code snippets where relevant
- ✅ Migration references
- ✅ Handler/function names

**No invented behavior** - all conclusions backed by actual code.
