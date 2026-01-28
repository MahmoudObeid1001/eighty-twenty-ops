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
   Create a `.env` file (or set env vars). Example:
   ```bash
   DATABASE_URL=postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable
   PORT=3000
   SESSION_SECRET=change-this-to-a-random-secret-in-production
   ADMIN_EMAIL=admin@eightytwenty.test
   ADMIN_PASSWORD=admin123
   MODERATOR_EMAIL=moderator@eightytwenty.test
   MODERATOR_PASSWORD=moderator123
   MENTOR_HEAD_EMAIL=mentor_head@eightytwenty.test
   MENTOR_HEAD_PASSWORD=mentor_head123
   MENTOR_EMAIL=mentor@eightytwenty.test
   MENTOR_PASSWORD=mentor123
   COMMUNITY_OFFICER_EMAIL=community_officer@eightytwenty.test
   COMMUNITY_OFFICER_PASSWORD=community_officer123
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
     - **Admin:** `admin@eightytwenty.test` / `admin123` (or `ADMIN_EMAIL` / `ADMIN_PASSWORD`)
     - **Moderator:** `moderator@eightytwenty.test` / `moderator123` (or `MODERATOR_EMAIL` / `MODERATOR_PASSWORD`)
     - **Mentor Head:** `mentor_head@eightytwenty.test` / `mentor_head123` (or `MENTOR_HEAD_EMAIL` / `MENTOR_HEAD_PASSWORD`)
     - **Mentor:** `mentor@eightytwenty.test` / `mentor123` (or `MENTOR_EMAIL` / `MENTOR_PASSWORD`)
     - **Community Officer:** `community_officer@eightytwenty.test` / `community_officer123` (or `COMMUNITY_OFFICER_EMAIL` / `COMMUNITY_OFFICER_PASSWORD`)

### Authentication & Roles

- All routes except `/login` and `/static/*` require authentication
- Sessions are stored in signed cookies (`eighty_twenty_session`)
- Default admin, moderator, mentor_head, mentor, and community_officer users are automatically created on first server start if they don't exist
- After login, users are redirected to `/pre-enrolment`

**Role Permissions:**
- **Admin:** Full access - can create, view, edit, delete leads, update lead status, manage payments/offers/refunds, access Classes, Finance, Mentor Head, Mentor, and Community Officer pages
- **Moderator:** Limited access - can create leads, view leads, and edit **only** basic lead info (name, phone, source, notes) to fix mistakes. Cannot delete leads, change status, see/edit payments/offers/pricing, or access Classes/Finance/Mentor sections
- **Mentor Head:** Access `/mentor-head` - assign mentors to classes, return classes to Operations, cancel/reschedule sessions, close rounds
- **Mentor:** Access `/mentor` and assigned class detail - mark attendance, enter grades, add notes, complete sessions
- **Community Officer:** Access `/community-officer` - submit feedback (sessions 4 & 8), log absence follow-up

### Features

- **Pre-Enrolment Module:**
  - List all leads with status, payment, and next action
  - Create new leads
  - Edit lead details (all sections for admin; basic info only for moderators)
  - Update lead status (admin only)
  - Track placement tests, offers, bookings, payments, scheduling, and shipping
  - Cancel leads with optional refunds (admin only)
  - Send leads to Classes board when ready (admin only)

- **Finance Module (Admin only):**
  - View current cash balance and balance by payment method
  - View transaction ledger with filtering
  - Create refunds for cancelled leads
  - Track expenses and income

- **Classes Board (Admin only):**
  - View students organized by groups
  - Move students between groups
  - Track current round

- **Role-Based Access Control:**
  - Admin: Full access to all features
  - Moderator: Limited to creating leads and editing basic lead information (name, phone, source, notes)
  - Custom access-restricted pages for moderators attempting to access admin-only sections

### Database

The database schema is automatically migrated on server startup. The migration system tracks applied migrations in the `schema_migrations` table.

### Development

- Static files are served from `web/static/`
- Templates are in `internal/views/`
- Handlers are in `internal/handlers/`
- Models and database operations are in `internal/models/`

### Documentation

- `docs/MILESTONE_1_PRE_ENROLMENT_QA_AUDIT.md` — Comprehensive QA audit of Pre-Enrolment flows, invariants, and regression risks
- `docs/BLOCKING_FIXES_DELIVERABLE.md` — Detailed explanation of the 5 blocking issues fixed (double refund, cancel idempotency, payment bounds, etc.)
- `docs/MODERATOR_UX_MANUAL_CHECKLIST.md` — Manual QA checklist for moderator role behavior and permissions

### Recent Updates

**Milestone 1 Hardening (Latest):**
- ✅ Fixed 5 blocking issues from QA audit (double refund, cancel idempotency, payment bounds, error rendering)
- ✅ Implemented moderator role UX + permissions (limited edit mode, access restrictions)
- ✅ Added idempotent refund creation for cancel flow
- ✅ Server-side validation for course payments (offer_final_price, remaining_balance)
- ✅ Shared detail view model helper for consistent error rendering
- ✅ Custom access-restricted pages for Classes/Finance (moderator-friendly 403)
