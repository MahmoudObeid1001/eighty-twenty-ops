# HR Mentor Profile Data Flow

**Purpose**: Define required mentor identity fields when HR creates mentors.

**Status**: Implemented.

---

## Rule (Implemented)

- Mentor records must include:
  - `full_name`
  - `email`
  - `phone`
- HR create form must require all three fields.
- Backend must validate all three fields and reject missing values.

## Data Storage (Implemented)

- Persist mentor identity on `users` table.
- For existing mentors missing profile values:
  - backfill `full_name` from email local part
  - backfill generated phone numbers

## Enforcement (Implemented)

- Application-level validation in `HRHandler.MentorsCreate`.
- Database-level constraint for `role = 'mentor'` rows:
  - `full_name` and `phone` must be non-empty.

## Implementation References

- Migration:
  - `internal/db/migrations/056_add_user_profile_fields_for_mentors.sql`
- HR handler:
  - `internal/handlers/hr.go`
- HR form:
  - `internal/views/hr_mentors.html`
- User repository:
  - `internal/models/repository.go`
