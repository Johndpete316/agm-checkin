# Competitor Import Guide

## Philosophy

Historical imports are a two-stage process by design:

1. **Normalize first, commit later.** A Go script transforms raw event CSVs into a single clean, human-readable CSV. That file is the artifact — review it, spot-check it, share it with event staff, make manual corrections — before a single row hits the database.

2. **Import through the UI.** Admins upload the normalized CSV through the Import Data page. The backend takes a Postgres snapshot before writing anything, so the import is always reversible.

This keeps the data pipeline transparent and repeatable. When new historical event data arrives, the same two steps apply regardless of the source format.

---

## Stage 1 — Generate the Normalized CSV

From the `agm-checkin-api/` directory, run the normalization script against your raw CSV files. Files can be passed in any order — the script detects which event each file belongs to from its headers.

```bash
cd agm-checkin-api

go run ./bin/import \
  ../scripts/agm_data_dump/glr-2026.csv \
  ../scripts/agm_data_dump/glr-2025.csv \
  ../scripts/agm_data_dump/nat-2025.csv \
  ../scripts/agm_data_dump/nat-2024.csv \
  > normalized_import.csv
```

Progress and warnings are printed to stderr; only the CSV goes to stdout.

### What the script does

- **Deduplicates** across all sheets by full name (first + last). One person competing across multiple years becomes one row.
- **Merges fields** — DOBs, studio/teacher, and student emails are filled in from whichever sheet has them.
- **Preserves full event history** — a competitor who attended nat-2024, glr-2025, and glr-2026 gets `events=nat-2024|glr-2025|glr-2026`.
- **Sets validation flags** — if a competitor was marked verified in any prior year, `validated=true` and `requires_validation=false`. Anyone brand new or never previously verified gets `requires_validation=true`.
- **Normalizes shirt sizes** to shorthand: `Adult XL`, `Adult L`, `Adult M`, `Adult S`, `Youth XL`, `Youth L`, `Youth M`, `Youth S`.
- **Keeps teacher emails out of the student email field.** Teacher contact data is preserved in the `teacher` column for future use when a teachers table is added.

### Review before importing

Open `normalized_import.csv` in a spreadsheet and spot-check:

- Known competitors appear with the right DOB, studio, and event history
- `requires_validation` and `validated` flags look correct
- No obvious merge errors (two different people with the same name are rare but possible — fix manually if found)
- Shirt sizes look right

This is also the file to share with event staff as a template for submitting future historical data — the column headers define exactly what the system expects.

---

## Stage 2 — Import via the Admin UI

1. Deploy the latest backend to your target environment (local or production).
2. Log in as an admin and navigate to **Import Data** in the nav.
3. Drag-and-drop or click to select `normalized_import.csv`.
4. Review the 5-row preview table — confirm it's the right file.
5. Click **Import N competitors**.
6. The result card shows competitors created, stub events created, and event registrations added.

### What happens on the backend

Before writing any data, the API creates snapshot tables:

```sql
competitors_backup_<unix_timestamp>
competitor_events_backup_<unix_timestamp>
```

These are plain Postgres table copies. If something goes wrong, an admin with DB access can restore from them directly.

Stub event records (no dates) are auto-created for any event IDs referenced in the CSV that don't already exist. Fill in the dates later via the Events page.

---

## Rolling Back an Import

If you need to undo an import, connect to the database and restore from the backup tables created at import time:

```sql
-- Find the backup timestamp
SELECT table_name FROM information_schema.tables
WHERE table_name LIKE 'competitors_backup_%'
ORDER BY table_name DESC;

-- Restore (replace <ts> with the actual timestamp)
TRUNCATE competitors CASCADE;
INSERT INTO competitors SELECT * FROM competitors_backup_<ts>;

TRUNCATE competitor_events;
INSERT INTO competitor_events SELECT * FROM competitor_events_backup_<ts>;
```

---

## Normalized CSV Schema

This is the format the import endpoint expects. Use it as a template when preparing historical data from future events.

| Column | Format | Notes |
|--------|--------|-------|
| `first_name` | string | |
| `last_name` | string | |
| `studio` | string | empty if unknown |
| `teacher` | string | teacher's display name, e.g. `"Emshwiller, Michael"` |
| `email` | string | student or parent email only — no teacher emails |
| `shirt_size` | string | `Adult XL / L / M / S` or `Youth XL / L / M / S` |
| `date_of_birth` | `YYYY-MM-DD` or empty | empty if unknown |
| `requires_validation` | `true` / `false` | whether ID check is needed at check-in |
| `validated` | `true` / `false` | whether ID has already been verified |
| `events` | pipe-separated event IDs | e.g. `nat-2024\|glr-2025\|glr-2026` |

### Known event IDs

| ID | Event |
|----|-------|
| `nat-2024` | Nationals 2024 |
| `glr-2025` | GLR 2025 |
| `nat-2025` | Nationals 2025 |
| `glr-2026` | GLR 2026 (current) |

New events follow the same slug pattern: `{prefix}-{year}`.

---

## Adding Data from a New Event

When a new event's registration data arrives as a raw CSV:

1. Add the file to `scripts/agm_data_dump/` (or wherever you keep raw data).
2. Add a detection rule to `bin/import/main.go` → `detectCSVType()` for the new file's headers.
3. Add a parsing case to `processFile()` mapping the new file's columns to the normalized fields.
4. Re-run the script with all files (including the new one) to regenerate the normalized CSV.
5. Review and import as normal.
