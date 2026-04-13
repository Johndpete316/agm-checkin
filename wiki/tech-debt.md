# Tech Debt and Known Limitations

This page documents all TODOs, FIXMEs, hardcoded assumptions, and known limitations found through static analysis of the codebase.

---

## Backend

### Hardcoded canonical event order

**File:** `internal/service/competitors.go` and `bin/import/main.go`

```go
var eventOrder = []string{"nat-2024", "glr-2025", "nat-2025", "glr-2026"}
```

This list is hardcoded in two places and is used to determine `last_registered_event` during bulk import. Every new event season requires updating this list. There is no database-driven ordering for this purpose.

Similarly, `bin/import/main.go` hardcodes four event constants and four CSV format detection patterns — adding a fifth event season requires new code.

### No bulk import deduplication by competitor identity

**File:** `internal/service/competitors.go`, `BulkImport`

The import service has no unique constraint on `(name_first, name_last, studio)` and no deduplication logic. Running a bulk import on a populated database creates duplicate competitor records. The code contains a comment acknowledging this:

> "The operation is designed for an empty database; re-running on a populated DB will produce duplicate competitors since there is no unique constraint on (name, studio)."

### Backup snapshot tables are never cleaned up

**File:** `internal/service/competitors.go`, `BulkImport`

Before each bulk import, `competitors_backup_<unix>` and `competitor_events_backup_<unix>` tables are created via raw SQL. These tables accumulate indefinitely and are never cleaned up automatically. Manual cleanup is required after a successful import.

### No test coverage

There are no test files (`*_test.go`) anywhere in `agm-checkin-api/`. There is no test infrastructure or CI pipeline to run tests.

### AUTH_PIN stored as plaintext in env

The staff access code is stored in plaintext in the `AUTH_PIN` environment variable (injected via Kubernetes Secret). While Kubernetes Secrets are base64-encoded (not encrypted at rest by default), the comparison uses `crypto/subtle.ConstantTimeCompare` which is appropriate. The plaintext storage is a documentation concern rather than an active exploit.

### Postgres password in values.yaml (non-secret) as fallback

`helm/agm-checkin/values.yaml` contains `postgres.password: test` as a default value. This is committed to the repository. The intent is that `values.secret.yaml` overrides it, but a deploy without the secrets file would use `"test"` as the production database password.

---

## Frontend

### Hardcoded event list in EditCompetitorDialog

**File:** `src/components/EditCompetitorDialog.jsx`

```js
const EVENTS = ['glr-2026', 'nat-2025', 'glr-2025', 'nat-2024']
```

The `lastRegisteredEvent` dropdown in the edit dialog uses this hardcoded list. Adding a new event requires updating this array in the frontend source code and redeploying.

### Hardcoded shirt size list inconsistency

`AddCompetitorDialog` uses generic sizes `['XS', 'S', 'M', 'L', 'XL', 'XXL']`, while `EditCompetitorDialog` uses adult/youth sizes `['Adult XL', 'Adult L', 'Adult M', 'Adult S', 'Youth XL', 'Youth L', 'Youth M', 'Youth S']`. These do not match each other, and neither necessarily matches the values already in the database from historical imports (which include values like `"S"`, `"M"`, `"Adult L"`, etc.).

### Validation dialog duplicated between CheckInPage and CompetitorsPage

The validation dialog (for competitors requiring identity check before check-in) is implemented twice: once inside `CompetitorCard.jsx` (used by CheckInPage) and once inline in `CompetitorsPage.jsx`. The logic is identical but duplicated. A change to the validation flow requires updating both locations.

### Age calculation and DOB formatting duplicated

The utility functions `calculateAge`, `formatDOB`, and `toInputDate` appear in both `CompetitorCard.jsx` and `CompetitorsPage.jsx`. They are identical copies — not imported from a shared utility module.

### No loading state on Competitors page edit/validate operations

When the validation confirm dialog is active on `CompetitorsPage`, the whole-page `checkingIn` state is separate from the dialog `confirming` state. If an error occurs during the confirm sequence, the UI shows the error but the original competitor row may be in a partial update state until the page is refreshed.

### VITE_API_URL is baked at build time

Changing the API domain requires rebuilding and redeploying the frontend image. This is documented but is a deployment friction point — there is no runtime configuration mechanism.

### Column visibility hardcoded event order

**File:** `src/pages/CompetitorsPage.jsx`

```js
const EVENT_ORDER = ['nat-2024', 'glr-2025', 'nat-2025', 'glr-2026']
```

Same hardcoded event ordering as the backend. The event filter on the Competitors page uses this to sort event checkboxes chronologically.

### staff.js inconsistent 401 handling

`src/api/staff.js` throws `Error('unauthorized')` on 401 instead of redirecting to `/login` like the other modules. The `ManageUsersPage` does not handle this error case explicitly — it falls through to the generic `err.message || 'Failed to load users.'` handler. A revoked admin token on the ManageUsers page will show "unauthorized" rather than triggering logout.

---

## Infrastructure

### No automated CI/CD

There are no GitHub Actions workflows or any other automated pipeline. All deploys are triggered manually by a developer with SSH access to the cluster. A mis-run or partial failure of `push-images.fish` could leave the cluster in an inconsistent state.

### No image versioning / tagging

All images are pushed as `agm-api:latest` and `agm-frontend:latest`. There is no semantic versioning, no git-sha tagging, and no rollback capability via image tags. Rolling back requires checking out a previous commit and re-running `push-images.fish`.

### DR script contains placeholder credentials

`scripts/dr-deploy.fish` contains `set AUTH_PIN "FILL_IN"` and `set DB_PASSWORD "FILL_IN"` as literal placeholders. The script will deploy with these placeholder values if not edited before use. There is no validation that these have been updated.

### pgAdmin password cached in PVC

If pgAdmin is first deployed with incorrect credentials, the PVC caches the initial config. Fixing requires deleting the PVC and pod. This is documented in `DEPLOYMENT.md` under Troubleshooting.

### Single postgres replica (no HA)

PostgreSQL runs as a single-replica StatefulSet. There is no replication, no standby, and no automatic failover. A pod failure requires manual intervention and relies on the PVC being reattached to a new pod.

---

## Import Tool

### Teacher email not stored as competitor email

**File:** `bin/import/main.go`

A comment in the GLR 2026 import case notes:

> "Teacher email is intentionally not stored as the competitor email. It will be used for a teachers table in the future."

The teachers table does not exist yet. Teacher email from GLR 2026 source data is discarded.

### Two-digit year parsing may misbehave for dates near 2069

**File:** `bin/import/main.go`, `parseDOBTwoDigit`

The GLR 2025 CSV uses two-digit years (M/D/YY). Go's `"06"` format maps years 00–68 to 2000–2068 and 69–99 to 1969–1999. The import tool adds a check: if the parsed year is greater than 2026, subtract 100 years. This works for current data but will misfire for competitors born after 2026 in future use.

---

## Related Pages

- [Backend Overview](backend/README.md)
- [Frontend Overview](frontend/README.md)
- [Infrastructure as Code](infrastructure/iac.md)
