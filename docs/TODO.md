# TODO

## User-friendly error messages
- Ops Classes board handlers (`internal/handlers/classes.go`): replace remaining generic `http.Error` messages with user-friendly copy and consider banner-based UX instead of raw error pages.
- API endpoints (`internal/handlers/api.go`): standardize JSON error messages for user-facing actions (mentor head, classes, student success) and keep internal errors in logs only.

## Business workflows to document/implement
- Manager role (see existing description in memory)
- After-round workflow (primarily Student Success)
- Grading workflow (mentor)
- Reports
