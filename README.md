# Eighty Twenty Operations

## Running the Application

### Prerequisites
- Go 1.21 or later
- Docker and Docker Compose

### Setup

1. **Start PostgreSQL database:**
   ```bash
   docker-compose up -d
   ```

2. **Set environment variables (optional):**
   Create a `.env` file based on `.env.example`:
   ```bash
   DATABASE_URL=postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable
   PORT=3000
   SESSION_SECRET=change-this-to-a-random-secret-in-production
   ADMIN_EMAIL=admin@eightytwenty.test
   ADMIN_PASSWORD=admin123
   ```

3. **Install Go dependencies:**
   ```bash
   go mod download
   ```

4. **Run the server:**
   ```bash
   go run cmd/server/main.go
   ```

5. **Access the application:**
   - URL: http://localhost:3000
   - Default logins:
     - **Admin:** `admin@eightytwenty.test` / `admin123` (or values from `ADMIN_EMAIL` / `ADMIN_PASSWORD` env vars)
     - **Moderator:** `moderator@eightytwenty.test` / `moderator123` (or values from `MODERATOR_EMAIL` / `MODERATOR_PASSWORD` env vars)

### Authentication & Roles

- All routes except `/login` and `/static/*` require authentication
- Sessions are stored in signed cookies (`eighty_twenty_session`)
- Default admin and moderator users are automatically created on first server start if they don't exist
- After login, users are redirected to `/pre-enrolment`

**Role Permissions:**
- **Admin:** Full access - can create, view, edit, and update lead status
- **Moderator:** Create-only - can create leads and view details (read-only), but cannot edit or update status

### Features

- **Pre-Enrolment Module:**
  - List all leads with status, payment, and next action
  - Create new leads
  - Edit lead details (all sections)
  - Update lead status
  - Track placement tests, offers, bookings, payments, scheduling, and shipping

### Database

The database schema is automatically migrated on server startup. The migration system tracks applied migrations in the `schema_migrations` table.

### Development

- Static files are served from `web/static/`
- Templates are in `internal/views/`
- Handlers are in `internal/handlers/`
- Models and database operations are in `internal/models/`
