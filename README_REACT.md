# React App Setup

This project includes a React app for Mentor and Mentor Head roles, served under `/app/*` routes.

## Quick Start

1. **Build the React app:**
   ```bash
   cd frontend
   npm install
   npm run build
   ```

2. **Start the Go server:**
   ```bash
   go run cmd/server/main.go
   ```

3. **Access the React app:**
   - Mentor: `http://localhost:3000/app/mentor`
   - Mentor Head: `http://localhost:3000/app/mentor-head`
   - Class workspace: `http://localhost:3000/app/mentor/class?class_key=...`

## Development Setup

1. **Install dependencies:**
   ```bash
   cd frontend
   npm install
   ```

2. **Run development server (with hot reload):**
   ```bash
   npm run dev
   ```
   This will start Vite dev server on `http://localhost:5173` with proxy to Go API at `http://localhost:3000`.

3. **Build for production:**
   ```bash
   npm run build
   ```
   This builds the React app and outputs to `frontend/dist/` directory.

## Routes

### React App Routes (SPA)
- `/app/mentor` - Mentor dashboard
- `/app/mentor-head` - Mentor Head dashboard
- `/app/mentor/class?class_key=...` - Mentor class workspace
- `/app/mentor-head/class?class_key=...` - Mentor Head class workspace

### API Routes (JSON)
- `GET /api/me` - Current user info { id, email, name, role }
- `GET /api/mentor/classes` - Mentor's classes
- `GET /api/mentor-head/dashboard` - Mentor Head dashboard data (classes + mentors)
- `GET /api/mentor-head/classes` - Classes grouped by mentor
- `GET /api/class-workspace?class_key=...` - Class workspace data
- `GET /api/student?student_id=...&class_key=...` - Student profile
- `GET /api/notes?student_id=...&class_key=...` - Student notes
- `POST /api/notes` - Create note (body: { student_id, class_key, text })
- `DELETE /api/notes?note_id=...` - Delete note
- `POST /api/mentor-head/assign-mentor` - Assign mentor (body: { class_key, mentor_email })
- `POST /api/mentor-head/start-round` - Start round (body: { class_key })
- `POST /api/mentor-head/close-round` - Close round (body: { class_key })
- `POST /api/mentor-head/return-to-ops` - Return to Operations (body: { class_key })

## Notes

- The React app uses cookie-based authentication (same session as SSR pages)
- All API requests include `credentials: 'include'` to send cookies
- The Go server serves the React build from `frontend/dist/` directory
- React app uses the same sidebar + branding as SSR pages (AppLayout component)
- SSR pages remain unchanged and continue to work at their original routes
- All `class_key` values are passed as query parameters (never path params) to handle values containing "/"
