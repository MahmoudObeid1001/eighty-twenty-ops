# Current Round Import CSV Spec

Use these CSV files to migrate an already-started manual round into the system.

This import is designed for:
- current working mentors
- active classes that already exist in the system
- current students already attached to those classes
- attendance backfill for sessions that already happened

This import is **not** for creating new class groups from scratch. Each imported row must point to an existing `class_groups.class_key`.

## Files

Place these files under:

- `data/import/current_round/classes.csv`
- `data/import/current_round/mentors.csv`
- `data/import/current_round/students.csv`
- `data/import/current_round/attendance.csv`

## General Rules

- CSV must be UTF-8.
- First row must be headers exactly as documented below.
- Empty cell means `NULL` unless the field is required.
- Use one phone format consistently across all files.
- `class_key` must match an existing class exactly.
- Do not change header names.

## 1. `classes.csv`

Purpose: map each existing class to its current mentor and current manual round position.

### Required columns

- `class_key`
- `level`
- `class_days`
- `class_time`
- `class_number`
- `mentor_email`
- `current_session_number`

### Column rules

- `class_key`: existing key from `class_groups.class_key`
- `level`: integer `1..8`
- `class_days`: exact DB value, for example `Sun/Wed`
- `class_time`: exact DB value in `HH:MM` 24-hour format, for example `07:30`
- `class_number`: integer group number, usually `1`, `2`, ...
- `mentor_email`: email of mentor to assign to this class
- `current_session_number`: integer `1..8`

### Example

```csv
class_key,level,class_days,class_time,class_number,mentor_email,current_session_number
L1|Sun/Wed|07:30|1,1,Sun/Wed,07:30,1,mentor1@example.com,3
L2|Sat/Tues|10:00|1,2,Sat/Tues,10:00,1,mentor2@example.com,2
```

## 2. `mentors.csv`

Purpose: create or match mentor users before class assignment.

### Required columns

- `email`
- `full_name`
- `phone`

### Optional columns

- `is_active`

### Column rules

- `email`: unique mentor identity
- `full_name`: required for mentor users
- `phone`: required for mentor users
- `is_active`: `true` or `false`; default `true` if empty

### Example

```csv
email,full_name,phone,is_active
mentor1@example.com,Sara Ahmed,01012345678,true
mentor2@example.com,Omar Ali,01023456789,true
```

## 3. `students.csv`

Purpose: create or update students, attach them to an existing live class, and define whether they were in the class from session 1 or joined later.

### Required columns

- `student_external_id`
- `full_name`
- `phone`
- `class_key`
- `level`
- `class_days`
- `class_time`
- `class_number`
- `roster_start_session`

### Optional columns

- `status`
- `is_returning`
- `levels_purchased_total`
- `levels_consumed`
- `source`
- `notes`

### Column rules

- `student_external_id`: your manual-sheet stable identifier; required for reconciliation only, not stored as system PK
- `full_name`: required
- `phone`: required and should be unique per student
- `class_key`: existing class key
- `level`: integer `1..8`; must match the class level
- `class_days`: exact class days value
- `class_time`: exact class time value in `HH:MM`
- `class_number`: integer class group number
- `roster_start_session`:
  - `1` means student was part of the class from the start
  - `2` means student joined as a late joiner at session 2
- `status`: default `in_classes`; leave empty unless we agree otherwise
- `is_returning`: `true` or `false`; default `false`
- `levels_purchased_total`: integer, optional
- `levels_consumed`: integer, optional
- `source`: optional free text
- `notes`: optional free text

### Important constraint

For this first importer version, `roster_start_session` must be either:

- `1`
- `2`

That matches current late-join support in the codebase.

### Example

```csv
student_external_id,full_name,phone,class_key,level,class_days,class_time,class_number,roster_start_session,status,is_returning,levels_purchased_total,levels_consumed,source,notes
S-001,Mohamed Tarek,01030000001,L1|Sun/Wed|07:30|1,1,Sun/Wed,07:30,1,1,in_classes,false,,,manual sheet,
S-002,Nour Hassan,01030000002,L1|Sun/Wed|07:30|1,1,Sun/Wed,07:30,1,2,in_classes,false,,,manual sheet,Joined after session 1
```

## 4. `attendance.csv`

Purpose: backfill attendance for sessions that already happened.

### Required columns

- `class_key`
- `session_number`
- `student_external_id`
- `phone`
- `status`

### Optional columns

- `notes`

### Column rules

- `class_key`: existing class key
- `session_number`: integer `1..8`
- `student_external_id`: must match `students.csv`
- `phone`: must match `students.csv`
- `status`: one of:
  - `PRESENT`
  - `ABSENT`
  - `LATE`
  - `EXCUSED`
  - `N/A`
  - empty string
- `notes`: optional

### Attendance policy

- For students with `roster_start_session = 1`, provide attendance for every completed session.
- For students with `roster_start_session = 2`, session 1 should normally be `N/A`.
- Do not invent attendance for unknown cases. Leave those rows out and we will review them before import.

### Example

```csv
class_key,session_number,student_external_id,phone,status,notes
L1|Sun/Wed|07:30|1,1,S-001,01030000001,PRESENT,
L1|Sun/Wed|07:30|1,1,S-002,01030000002,N/A,Joined at session 2
L1|Sun/Wed|07:30|1,2,S-001,01030000001,PRESENT,
L1|Sun/Wed|07:30|1,2,S-002,01030000002,PRESENT,
```

## Import Order

The importer will use this order:

1. `mentors.csv`
2. `classes.csv`
3. `students.csv`
4. `attendance.csv`

## Validation We Will Enforce

- mentor email in `classes.csv` must exist in `mentors.csv` or already exist in `users`
- each `class_key` must already exist
- `level`, `class_days`, `class_time`, `class_number` must match the existing class
- each student phone must be unique within the import
- student class fields must match the class row exactly
- `roster_start_session` must be `1` or `2`
- attendance rows must refer to an imported student
- attendance `session_number` must be less than `current_session_number` for that class, or equal if we explicitly decide to import current-session data too

## What You Should Fill First

Minimum data to start safely:

1. `mentors.csv`
2. `classes.csv`
3. `students.csv`

Then fill `attendance.csv` only for sessions you trust.

## Notes

- If you have students who actually joined at session `3+`, flag them separately before import. The current system late-join logic only supports session `2`.
- If you are unsure about a student's past attendance, leave that row out rather than guessing.
