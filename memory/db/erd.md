# Database ERD

**Purpose**: Visual map of core database tables and relationships.

**Evidence Sources**:  
- `migrations/001_init.sql` - Core tables (users, leads, placement_tests, etc.)
- `migrations/004_classes_board.sql` - Settings table
- `migrations/005_class_groups_workflow.sql` - class_groups table
- `migrations/022_create_mentor_assignments.sql` - mentor_assignments table
- `migrations/029_create_followups_table.sql` - followups table
- `migrations/033_add_complaints_to_followups.sql` - Complaint fields

---

## Entity Relationship Diagram

```mermaid
erDiagram
    users ||--o{ leads : "created_by"
    users ||--o{ mentor_assignments : "mentor_user_id"
    users ||--o{ mentor_assignments : "created_by"
    users ||--o{ followups : "created_by"
    users ||--o{ followups : "deleted_by"
    users ||--o{ late_joiner_notifications : "user_id"
    users ||--o{ feedback_collected_uploads : "uploaded_by"
    users ||--o{ mentor_testimonials : "mentor_id"
    users ||--o{ mentor_testimonials : "created_by_user_id"
    
    leads ||--o| placement_tests : "lead_id"
    leads ||--o| offers : "lead_id"
    leads ||--o| bookings : "lead_id"
    leads ||--o| payments : "lead_id"
    leads ||--o| scheduling : "lead_id"
    leads ||--o| shipping : "lead_id"
    leads ||--o{ followups : "lead_id"
    leads ||--o{ late_joiner_notifications : "lead_id"
    leads ||--o{ feedback_collected_uploads : "lead_id"
    
    class_groups ||--o| mentor_assignments : "class_key"
    class_groups ||--o{ class_sessions : "class_key"
    class_groups ||--o{ followups : "class_key"
    class_groups ||--o{ late_joiner_notifications : "class_key"
    class_groups ||--o{ feedback_collected_uploads : "class_key"
    class_groups ||--o{ mentor_testimonials : "class_key"
    
    class_sessions ||--o{ attendance : "session_id"
    class_sessions ||--o{ session_performance : "class_session_id"
    attendance }o--|| leads : "lead_id"
    session_performance }o--|| leads : "lead_id"
    
    leads ||--o{ grades : "lead_id"
    class_groups ||--o{ grades : "class_key"
    
    leads ||--o{ student_notes : "lead_id"
    class_groups ||--o{ student_notes : "class_key"
    users ||--o{ student_notes : "created_by"
    
    class_groups ||--o{ community_officer_feedback : "class_key"
    users ||--o{ community_officer_feedback : "created_by"
    
    followups ||--o{ followup_case_notes : "followup_id"
    feedback_collected_uploads {
        uuid id PK
        uuid lead_id FK
        text class_key FK
        int session_number
        text file_name
        text file_url
        text mime_type
        int size_bytes
        text note
        uuid uploaded_by_user_id FK
        timestamp uploaded_at
    }

    mentor_testimonials {
        uuid id PK
        uuid mentor_id FK
        text class_key FK
        text testimonial_text
        uuid created_by_user_id FK
        timestamp created_at
        timestamp updated_at
    }
    
    class_sessions {
        uuid id PK
        text class_key FK
        int session_number
        date scheduled_date
        time scheduled_time
        text status
        timestamp completed_at
    }
    
    users {
        uuid id PK
        text email UK
        text full_name
        text phone
        text password_hash
        text role "CHECK(admin|moderator|mentor_head|mentor|community_officer|hr|student_success)"
        timestamp created_at
    }
    
    leads {
        uuid id PK
        text full_name
        text phone UK
        text source
        text notes
        text status "lead_created→in_classes"
        uuid created_by_user_id FK
        timestamp created_at
        timestamp updated_at
    }
    
    placement_tests {
        uuid id PK
        uuid lead_id FK "UNIQUE"
        date test_date
        time test_time
        text test_type "online|live"
        int assigned_level "1-8"
        text test_notes
        uuid run_by_user_id FK
        timestamp updated_at
    }
    
    offers {
        uuid id PK
        uuid lead_id FK "UNIQUE"
        int bundle_levels "1-4"
        int base_price
        int discount_value
        text discount_type "amount|percent"
        int final_price
        timestamp updated_at
    }
    
    bookings {
        uuid id PK
        uuid lead_id FK "UNIQUE"
        text book_format "pdf|printed"
        text address
        text city
        text delivery_notes
        timestamp updated_at
    }
    
    payments {
        uuid id PK
        uuid lead_id FK "UNIQUE"
        text payment_type "full|deposit"
        int amount_paid
        int remaining_balance
        date payment_date
        timestamp updated_at
    }
    
    scheduling {
        uuid id PK
        uuid lead_id FK "UNIQUE"
        text expected_round
        text class_days
        time class_time
        date start_date
        time start_time
        int class_group_index
        timestamp updated_at
    }
    
    shipping {
        uuid id PK
        uuid lead_id FK "UNIQUE"
        text shipment_status "pending|sent|delivered"
        date shipment_date
        timestamp updated_at
    }
    
    class_groups {
        text class_key PK
        int level
        text class_days
        text class_time
        int class_number
        bool sent_to_mentor
        text round_status "NOT_STARTED|IN_PROGRESS|CLOSED"
        timestamp sent_at
        timestamp returned_at
        timestamp closed_at
        uuid closed_by_mentor_user_id FK
        timestamp updated_at
    }
    
    mentor_assignments {
        uuid id PK
        uuid mentor_user_id FK
        text class_key FK "UNIQUE"
        timestamp assigned_at
        uuid created_by_user_id FK
    }
    
    attendance {
        uuid id PK
        uuid session_id FK
        uuid lead_id FK
        text status "present|absent_excused|absent_unexcused"
        timestamp marked_at
        uuid marked_by_user_id FK
    }

    session_performance {
        uuid id PK
        uuid class_session_id FK
        uuid lead_id FK
        bool task_completed
        int participation_score "1..5"
        timestamp created_at
        timestamp updated_at
    }
    
    followups {
        uuid id PK
        text class_key FK
        uuid lead_id FK
        int session_number "nullable for complaints"
        text note
        text status "NOT_CONTACTED|CONTACTED|RESOLVED"
        text type "absence_escalation|complaint"
        text category "nullable, complaint category"
        text urgency "nullable, complaint urgency"
        text student_phone "nullable"
        text complaint_text "nullable"
        uuid created_by FK
        timestamp created_at
        timestamp updated_at
        timestamp deleted_at "soft delete"
        uuid deleted_by_user_id FK
        text delete_reason
    }
    
    followup_case_notes {
        uuid id PK
        uuid followup_id FK
        text note_text
        text note_type "status_update|resolution"
        text new_status "nullable"
        text resolution_note "nullable"
        uuid created_by_user_id FK
        timestamp created_at
    }
    
    settings {
        text key PK
        text value
        timestamp updated_at
    }

    late_joiner_notifications {
        uuid id PK
        uuid lead_id FK
        text class_key FK
        uuid user_id FK
        int joined_at_session_number
        timestamp acknowledged_at
        timestamp created_at
    }
    
    grades {
        uuid id PK
        uuid lead_id FK
        text class_key FK
        int session_number "always 8"
        text grade "A|B|C|F"
        text notes
        uuid created_by_user_id FK
        timestamp created_at
        timestamp updated_at
    }
    
    student_notes {
        uuid id PK
        uuid lead_id FK
        text class_key FK "nullable"
        int session_number "nullable"
        text note_text
        bool is_private
        uuid created_by_user_id FK
        timestamp created_at
        timestamp updated_at
    }
    
    community_officer_feedback {
        uuid id PK
        text class_key FK
        int session_number "4 or 8"
        text feedback_text
        uuid created_by_user_id FK
        timestamp created_at
        timestamp updated_at
    }
```

---

## Migration Evidence

| Table | Created In | Modified In |
|-------|------------|-------------|
| `users` | `001_init.sql:4-10` | `014_add_mentor_roles.sql`, `023_add_hr_role.sql`, `028_add_student_success_role.sql`, `035_add_manager_role.sql`, `056_add_user_profile_fields_for_mentors.sql` (added `full_name`, `phone`, mentor required-profile check), `060_reactivate_manager_and_force_password_change.sql` (added `must_change_password`, active manager role constraint) |
| `leads` | `001_init.sql:13-26` | `004_classes_board.sql` (added `in_classes` status), `013_add_paused_status.sql` (added `paused`) |
| `placement_tests` | `001_init.sql:29-39` | `003_assigned_level_1_to_8.sql` (changed level constraint 1-4 → 1-8) |
| `offers` | `001_init.sql:42-51` | - |
| `bookings` | `001_init.sql:54-62` | - |
| `payments` | `001_init.sql:65-73` | - |
| `scheduling` | `001_init.sql:76-85` | `004_classes_board.sql:9` (added `class_group_index`) |
| `shipping` | `001_init.sql:88-94` | - |
| `settings` | `004_classes_board.sql:12-16` | - |
| `class_groups` | `005_class_groups_workflow.sql:2-12` | `027_class_groups_round_status.sql` (added round_status, closed_at, closed_by_mentor_user_id), `032_add_closed_mentor_user_id.sql` |
| `class_sessions` | `016_create_class_sessions.sql` | - |
| `attendance` | `017_create_attendance.sql` | `030_refine_attendance_and_followups.sql` |
| `session_performance` | `058_create_session_performance.sql` | - |
| `mentor_assignments` | `022_create_mentor_assignments.sql:3-10` | - |
| `followups` | `029_create_followups_table.sql:2-13` | `030_refine_attendance_and_followups.sql`, `033_add_complaints_to_followups.sql` (added type, category, urgency, student_phone, complaint_text, deleted_at, deleted_by_user_id, delete_reason), `036_make_followups_nullable_for_complaints.sql` |
| `followup_case_notes` | `034_create_followup_case_notes.sql` | - |
| `late_joiner_notifications` | `044_late_joiner_notifications.sql` | - |
| `grades` | `018_create_grades.sql` | - |
| `student_notes` | `019_create_student_notes.sql` | - |
| `community_officer_feedback` | `020_create_community_officer_feedback.sql` | - |
| `mentor_testimonials` | `057_create_mentor_testimonials.sql` | - |

---

## Key Constraints & Indexes

### Foreign Keys
- All `*_by_user_id` fields → `users(id)`
- Most tables have `lead_id FK → leads(id) ON DELETE CASCADE`
- `mentor_assignments.mentor_user_id` → `users(id) ON DELETE CASCADE`
- `mentor_assignments.class_key` → `class_groups(class_key) ON DELETE CASCADE`
- `attendance.session_id` → `class_sessions(id)`
- `session_performance.class_session_id` → `class_sessions(id)`
- `session_performance.lead_id` → `leads(id)`
- `followups.lead_id` → `leads(id) ON DELETE CASCADE`
- `followup_case_notes.followup_id` → `followups(id) ON DELETE CASCADE`

### Unique Constraints
- UNIQUE per lead: `placement_tests.lead_id`, `offers.lead_id`, `bookings.lead_id`, `payments.lead_id`, `scheduling.lead_id`, `shipping.lead_id`
- UNIQUE: `mentor_assignments(class_key)` - One mentor per class
- UNIQUE: `followups(class_key, lead_id, session_number)` - One follow-up per absence instance
- UNIQUE: `grades(lead_id, class_key, session_number)` - One grade per student per class per session
- UNIQUE: `attendance(session_id, lead_id)` - One attendance record per student per session
- UNIQUE: `session_performance(class_session_id, lead_id)` - One performance row per student per session

### Check Constraints
**Evidence**: Inline in CREATE TABLE statements

- `users.role` must be one of: admin, moderator, mentor_head, mentor, hr, student_success, manager
- `users.must_change_password` controls forced first-login password setup for manager-provisioned accounts
- `users_mentor_profile_required`: when `role='mentor'`, both `full_name` and `phone` must be non-empty
- `leads.status` has extensive pipeline states (lead_created → in_classes)
- `placement_tests.assigned_level` must be 1-8 *(changed from 1-4)*
- `placement_tests.test_type` must be online or live
- `offers.discount_type` must be amount or percent
- `bookings.book_format` must be pdf or printed
- `payments.payment_type` must be full or deposit
- `shipping.shipment_status` must be pending, sent, or delivered
- `attendance.status` must be present, absent_excused, or absent_unexcused
- `class_groups.round_status` must be NOT_STARTED, IN_PROGRESS, or CLOSED
- `followups.status` must be NOT_CONTACTED, CONTACTED, or RESOLVED
- `followups.type` must be absence_escalation or complaint
- `grades.grade` must be A, B, C, or F
- `community_officer_feedback.session_number` must be 4 or 8

### Performance Indexes
**Evidence**: `001_init.sql:97-100`, other migrations

- `idx_leads_status ON leads(status)`
- `idx_leads_phone ON leads(phone)`
- `idx_leads_created_at ON leads(created_at)`
- `idx_users_email ON users(email)`
- `idx_scheduling_class_group ON scheduling(class_days, class_time, class_group_index)`
- `idx_class_groups_key ON class_groups(class_key)`
- `idx_mentor_assignments_mentor_user_id ON mentor_assignments(mentor_user_id)`
- `idx_mentor_assignments_class_key ON mentor_assignments(class_key)`
- `idx_followups_class_key ON followups(class_key)`
- `idx_followups_lead_id ON followups(lead_id)`
- `idx_followups_type_status ON followups(type, status)`
- `idx_followups_deleted_at ON followups(deleted_at)`
- `idx_followups_student_phone ON followups(student_phone)`

---

## Notes

### Soft Delete Pattern
**Evidence**: `033_add_complaints_to_followups.sql:14-16`

`followups` table supports soft deletion via:
- `deleted_at` timestamp
- `deleted_by_user_id` UUID
- `delete_reason` TEXT

This was added for manager role (now removed) but the schema remains.

### Dual-Purpose followups Table
**Evidence**: `033_add_complaints_to_followups.sql`

The `followups` table serves two purposes:
1. **Absence escalations** (`type = 'absence_escalation'`) - Student Success workflow
2. **Complaints** (`type = 'complaint'`) - Complaint workflow

Complaint-specific fields (category, urgency, student_phone, complaint_text) are nullable and only used for complaint records.

### Session Numbering
**Evidence**: `036_make_followups_nullable_for_complaints.sql`

- Absence escalations: `session_number` is NOT NULL (tracks which session student missed)
- Complaints: `session_number` is NULL (not tied to specific session)
