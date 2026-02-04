# TODO

## User-friendly error messages
- Consider extending banner UX to mentor head / mentor / student success pages (these still use raw http.Error responses).
- API endpoints (`internal/handlers/api.go`): standardize JSON error messages for user-facing actions (mentor head, classes, student success) and keep internal errors in logs only.

## Business workflows to document/implement
- Manager role (see existing description in memory)
- After-round workflow (primarily Student Success)
- Grading workflow (mentor)
- Reports
