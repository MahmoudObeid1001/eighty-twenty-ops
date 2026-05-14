# Online DB Connection

## Confirmed online access pattern

For the `eighty-twenty-ops` production server, the direct working connection is:

```bash
PGPASSWORD='ChangeMeStrong123' psql -h localhost -U postgres -d eighty_twenty_ops
```

Notes:

- `localhost` here means the Postgres instance running on the same online server.
- The local docker-compose default password `postgres` did **not** work on the online server.
- The production password that worked was `ChangeMeStrong123`.

## Verified lead lookup command

Use this to find when a lead was added:

```bash
PGPASSWORD='ChangeMeStrong123' psql -h localhost -U postgres -d eighty_twenty_ops -F $'\t' -Atqc "
SELECT
  l.id,
  l.full_name,
  l.phone,
  l.status,
  l.created_at,
  l.updated_at
FROM leads l
WHERE regexp_replace(COALESCE(l.phone,''), '[^0-9]', '', 'g') IN ('1148075102', '01148075102')
ORDER BY l.created_at DESC;
"
```

## Verified class enrollment lookup command

Use this to see whether and when the lead was added to a class:

```bash
PGPASSWORD='ChangeMeStrong123' psql -h localhost -U postgres -d eighty_twenty_ops -F $'\t' -Atqc "
SELECT
  l.id,
  l.full_name,
  l.phone,
  ce.class_key,
  ce.created_at AS class_added_at,
  ce.outcome,
  ce.final_grade
FROM leads l
LEFT JOIN class_enrollments ce ON ce.lead_id = l.id
WHERE regexp_replace(COALESCE(l.phone,''), '[^0-9]', '', 'g') IN ('1148075102', '01148075102')
ORDER BY ce.created_at DESC NULLS LAST;
"
```
