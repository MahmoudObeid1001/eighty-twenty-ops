# Online Server Access

This file records the current production access details discovered from the live server on 2026-04-03.

## Server

- Host: `school-system-1`
- Service: `eighty-twenty-ops.service`
- Binary: `/opt/eighty-twenty-ops/bin/eighty-twenty-ops`
- Working directory: `/opt/eighty-twenty-ops`

## Database

- `DATABASE_URL=postgres://postgres:ChangeMeStrong123@localhost:5432/eighty_twenty_ops?sslmode=disable`

## SSH

Example:

```bash
ssh root@school-system-1
```

If DNS alias is not configured on your machine, use the server IP instead.

## Service Inspection

Check service status:

```bash
systemctl status eighty-twenty-ops.service --no-pager
```

Print service file:

```bash
sed -n '1,120p' /etc/systemd/system/eighty-twenty-ops.service
```

## Database Access

Connect with `psql`:

```bash
psql 'postgres://postgres:ChangeMeStrong123@localhost:5432/eighty_twenty_ops?sslmode=disable'
```

Run a one-shot query:

```bash
psql 'postgres://postgres:ChangeMeStrong123@localhost:5432/eighty_twenty_ops?sslmode=disable' -F $'\t' -Atqc "SELECT now();"
```

## Useful Queries

List active users:

```bash
psql 'postgres://postgres:ChangeMeStrong123@localhost:5432/eighty_twenty_ops?sslmode=disable' -F $'\t' -Atqc "SELECT email, role, COALESCE(full_name,''), COALESCE(phone,''), COALESCE(is_active,true) FROM users WHERE COALESCE(is_active,true)=true ORDER BY role, email;"
```

List all mentors:

```bash
psql 'postgres://postgres:ChangeMeStrong123@localhost:5432/eighty_twenty_ops?sslmode=disable' -F $'\t' -Atqc "SELECT email, role, COALESCE(full_name,''), COALESCE(phone,''), COALESCE(is_active,true) FROM users WHERE role = 'mentor' ORDER BY is_active DESC, email;"
```

List mentor assignments:

```bash
psql 'postgres://postgres:ChangeMeStrong123@localhost:5432/eighty_twenty_ops?sslmode=disable' -F $'\t' -Atqc "
SELECT u.email, COALESCE(u.full_name,''), ma.class_key
FROM mentor_assignments ma
JOIN users u ON u.id = ma.mentor_user_id
WHERE COALESCE(u.is_active,true)=true
ORDER BY u.email, ma.class_key;
"
```

List classes with mentor and round status:

```bash
psql 'postgres://postgres:ChangeMeStrong123@localhost:5432/eighty_twenty_ops?sslmode=disable' -F $'\t' -Atqc "
SELECT cg.class_key, cg.round_status, COALESCE(u.email,''), COALESCE(u.full_name,'')
FROM class_groups cg
LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
LEFT JOIN users u ON u.id = ma.mentor_user_id
ORDER BY cg.class_key;
"
```

## Notes

- Current active users observed on 2026-04-03:
  - `admin@eightytwenty.test`
  - `hr@eightytwenty.test`
  - `manager@eightytwenty.test`
  - `mentor@eightytwenty.test`
  - `mentor_head@eightytwenty.test`
  - `moderator@eightytwenty.test`
  - `student_success@eightytwenty.test`
- Only one active mentor account was present at the time of inspection: `mentor@eightytwenty.test`

## Security Follow-Up

- The production database password is stored in the systemd unit file.
- Rotate this password after the migration work is complete.
- Replace default `.test` production users and passwords with real managed credentials.
