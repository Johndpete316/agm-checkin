# Pages

All pages are in `src/pages/`. They are mounted by React Router via the route definitions in `src/App.jsx`.

---

## LoginPage

**Route:** `/login`  
**File:** `src/pages/LoginPage.jsx`  
**Auth:** None (redirects to `/home` if already authenticated)

A two-step login form:

**Step 1 — Access code:** A password-type input field labeled "Access code". No hints about format or length. On submit, advances to step 2 (no API call).

**Step 2 — Name:** First name and last name fields. On submit, calls `POST /api/auth/token`. On success, calls `login()` from `AuthContext` and navigates to `/home`. On failure, handles three error cases:
- `invalid_auth` → returns to step 1 with "Incorrect access code."
- `blocked` → disables the form and shows "Access denied."
- Other → generic error message

**Internal state:** `step` (1 or 2), `code`, `firstName`, `lastName`, `error`, `blocked`, `loading`

**API calls:** `requestToken(code, firstName, lastName)` from `src/api/auth.js`

---

## CheckInPage

**Route:** `/home`  
**File:** `src/pages/CheckInPage.jsx`  
**Auth:** Protected (Bearer token required)

The primary working view for registration staff. Provides a debounced search-driven competitor lookup and one-click check-in.

**UI intention:** Staff type a competitor's name. Results appear below after 300ms of no typing. Each result is a `CompetitorCard` showing the competitor's details and a "Check In" button.

**Data flow:**
- On `search` change (with 300ms debounce): calls `GET /api/competitors?search=<term>`
- On "Check In" click: calls `PATCH /api/competitors/{id}/checkin`
- On competitor validation/DOB update (within `CompetitorCard`): calls `PATCH /api/competitors/{id}/dob` and/or `PATCH /api/competitors/{id}/validate`

**Internal state:** `search`, `competitors` (array), `loading`, `error`, `checkingIn` (competitor ID or null)

**Key components used:** [`CompetitorCard`](components.md#competitorcard)

**Role behavior:** Registration users only see competitors registered for the current event (filtered by the backend). Admins see all competitors.

---

## CompetitorsPage

**Route:** `/competitors`  
**File:** `src/pages/CompetitorsPage.jsx`  
**Auth:** Protected

A full roster view with sorting, filtering, and (for admins) edit and add capabilities.

**UI intention:** Shows all competitors in a responsive layout — card list on mobile, sortable/filterable table on desktop. Staff can check in competitors from this page. Admins can add new competitors or edit existing ones.

**Data fetched on mount:** `GET /api/competitors` (no search param — loads all)

**Actions:**
- Sort by any column (client-side)
- Filter by event (checkbox group showing events found in data, persisted in component state)
- Toggle column visibility (persisted in `localStorage` under `agm_competitors_columns`)
- Check In (same validation flow as CheckInPage — handles `requiresValidation` flag)
- Edit competitor (admin only) → `PATCH /api/competitors/{id}` via `EditCompetitorDialog`
- Add competitor (admin only) → `POST /api/competitors` via `AddCompetitorDialog`

**Available columns (desktop table):** Name, Event, Studio, Teacher, Shirt, DOB/Age, Email, Validated, Status, Check-In Time, Note. Note and Check-In Time are hidden by default.

**Internal state:** `competitors`, `loading`, `error`, `order`, `orderBy`, `checkingIn`, `validateTarget`, `editedDOB`, `confirming`, `dialogError`, `editTarget`, `addOpen`, `visibleColumns`, `columnsAnchor`, `eventFilter`

**Key components used:** [`EditCompetitorDialog`](components.md#editcompetitordialog), [`AddCompetitorDialog`](components.md#addcompetitordialog), inline validation dialog (duplicated from CompetitorCard logic)

**API calls:**
- `getCompetitors()` on mount
- `checkInCompetitor(id)` on check-in
- `updateCompetitorDOB(id, dob)` during validation
- `validateCompetitor(id)` during validation
- `updateCompetitor(id, data)` via EditCompetitorDialog
- `createCompetitor(data)` via AddCompetitorDialog

---

## StatsPage

**Route:** `/stats`  
**File:** `src/pages/StatsPage.jsx`  
**Auth:** Protected

Live statistics dashboard for the current event.

**UI intention:** At-a-glance view of check-in progress, shirt inventory, and studio breakdown. Updates on navigation to the page (re-fetches on mount).

**Data fetched on mount (parallel):**
- `GET /api/competitors` (all competitors with check-in records)
- `GET /api/events/current`

**Computed stats (client-side, scoped to competitors with a non-null `currentCheckIn`):**
- Total registered for current event
- Checked in count and percentage
- Remaining count
- Validation pending count (requires validation, not yet validated, not yet checked in)

**Charts:**
- **Check-In Status** — donut/pie chart (Recharts `PieChart`): Checked In vs. Remaining. Colors: `#1565C0` (blue) and `#90A4AE` (grey).
- **Check-Ins by Day** — bar chart (Recharts `BarChart`): groups check-in timestamps by local date string.
- **T-Shirt Inventory** — stacked bar chart per shirt size: "Handed Out" (checked-in competitors) and "Remaining" (registered but not yet checked in). Sizes ordered by `SHIRT_SIZE_ORDER = ['YXS', 'YS', 'YM', 'YL', 'XS', 'S', 'M', 'L', 'XL', 'XXL', 'XXXL']`.
- **Top Studios** — horizontal bar chart of the top 10 studios by registered competitor count.

**Internal state:** `competitors`, `currentEvent`, `loading`, `error`

**Key components used:** `StatCard` (local), `CustomTooltip` (local), Recharts components

---

## EventsPage

**Route:** `/events`  
**File:** `src/pages/EventsPage.jsx`  
**Auth:** Admin

Management of competition events.

**UI intention:** Lists all events in a table (sorted newest first). Staff can see which event is current, change the current event, and create new events.

**Data fetched on mount:** `GET /api/events`

**Actions:**
- **Set as current** — calls `PATCH /api/events/{id}/current`. Optimistically updates the table (sets `isCurrent` on the updated event, clears it on all others).
- **New Event** — opens a dialog. On save, calls `POST /api/events`. Fields: Event ID (slug), Name, Start Date, End Date.

**Internal state:** `events`, `loading`, `error`, `creating`, `settingCurrent`, `dialogOpen`, `form`, `formError`, `saving`

**API calls:** `listEvents()`, `createEvent(data)`, `setCurrentEvent(id)`

---

## ManageUsersPage

**Route:** `/manage-users`  
**File:** `src/pages/ManageUsersPage.jsx`  
**Auth:** Admin

Staff token management.

**UI intention:** Table of all logged-in staff. Admins can change a staff member's role inline or revoke their access with a confirmation dialog. The current user's row shows a "you" chip and cannot be modified.

**Data fetched on mount:** `GET /api/staff`

**Actions:**
- **Role change** — inline `Select` dropdown per row. On change, calls `PATCH /api/staff/{id}/role`. Updates the local list optimistically.
- **Revoke** — `BlockIcon` button opens a confirmation dialog. On confirm, calls `DELETE /api/staff/{id}`. Removes the row from the local list.

**Self-protection:** `isSelf(s)` checks if `s.firstName + s.lastName` matches the authenticated user's name. If true, the revoke button is disabled and role changes are blocked at the service level.

**Internal state:** `staff`, `loading`, `error`, `revokeTarget`, `revoking`

**API calls:** `listStaff(token)`, `updateStaffRole(token, id, role)`, `revokeStaff(token, id)`

---

## AuditPage

**Route:** `/audit`  
**File:** `src/pages/AuditPage.jsx`  
**Auth:** Admin

Audit log viewer.

**UI intention:** Scrollable table of the most recent audit events with color-coded action chips and detail key-value display. Supports filtering by action type (dropdown) and actor name (debounced text input, 400ms).

**Data fetched on mount and on filter change:** `GET /api/audit?action=&actor=&limit=200`

**Columns:** Time, Actor, Action (colored chip), Entity (name + type), Detail (key-value badges), IP

**Action color coding:**
| Color | Actions |
|---|---|
| Red (error) | `competitor.deleted`, `staff.revoked` |
| Orange (warning) | `staff.role_updated`, `event.set_current` |
| Green (success) | `competitor.checked_in`, `competitor.validated` |
| Blue (primary) | `competitor.created`, `event.created` |
| Grey (default) | `competitor.updated`, `competitor.dob_updated` |

**Internal state:** `logs`, `loading`, `error`, `actionFilter`, `actorFilter`, `actorInput`

**API calls:** `listAuditLogs({ action, actor, limit: 200 })`

---

## ImportPage

**Route:** `/import`  
**File:** `src/pages/ImportPage.jsx`  
**Auth:** Admin

Bulk competitor import via normalized CSV file.

**UI intention:** A collapsible "File format & merge rules" section explains the required CSV schema, shows an example row, and describes the auto-fill and conflict-resolution behaviors. Below that is a drag-and-drop (or click-to-select) upload area. After file selection, a preview of the first 5 data rows is shown. Staff can confirm to import or cancel. After a successful import the result card shows counts and any warnings. If the import produced field conflicts, a conflict-resolution table appears above the drop zone and must be resolved before navigating away.

**Data flow:**
1. File selected (drag or input) → client-side CSV parse for preview (no API call)
2. "Import N competitors" button clicked → `POST /api/competitors/import` (multipart form)
3. Success → shows `ImportResult` summary: competitorsCreated, competitorsMatched, fieldsUpdated, eventsCreated, eventEntriesAdded, errors
4. If `result.fieldConflicts` is non-empty → conflict resolution table appears (see below)

**Conflict resolution flow:**
After import, any field where both the existing database record and the import file had different non-blank values is listed as a conflict. The table shows the competitor name, the conflicting field (email, studio, teacher, shirt size, or date of birth), the existing value, and the import value. Staff clicks **Keep existing** (no API call) or **Use import** (calls `PATCH /api/competitors/{id}/dob` for date of birth, or `PATCH /api/competitors/{id}` for all other fields). Once resolved, the conflict row is removed from the list. The list is ephemeral — navigating away loses it.

**Fields auto-filled (no conflict prompt):** If the database has a blank value and the import has a value, it is filled in silently. Applies to: email, studio, teacher, shirt size, date of birth.

**Fields never overwritten on existing records:** `requires_validation`, `validated`, `note`, `last_registered_event`, and all check-in records. Event registrations are added for missing events but existing registrations are never removed.

**Ambiguous name handling:** If more than one competitor in the database shares the same (first, last) name, the import row is skipped and the competitor is listed under warnings.

**Internal state:** `file`, `preview` (headers + rows + totalRows), `dragOver`, `loading`, `result`, `error`, `conflicts` (unresolved FieldConflict array), `resolvingId` (competitorId+field key for the in-flight save)

**API calls:**
- `importCompetitors(file)` from `src/api/competitors.js`
- `updateCompetitorDOB(id, date)` (conflict resolution, DOB field)
- `getCompetitor(id)` + `updateCompetitor(id, data)` (conflict resolution, other fields)

**CSV schema** (the accordion on the page spells this out):

| Column | Format | Notes |
|---|---|---|
| `first_name` | string | Required |
| `last_name` | string | Required |
| `studio` | string | Blank if unknown |
| `teacher` | string | Display name, e.g. `"Smith, Jane"` |
| `email` | string | Student/parent email only |
| `shirt_size` | string | `Adult XL/L/M/S` or `Youth XL/L/M/S` |
| `date_of_birth` | `YYYY-MM-DD` or blank | |
| `requires_validation` | `true`/`false` | Whether ID check needed at check-in |
| `validated` | `true`/`false` | Whether ID verified in a prior event |
| `events` | pipe-separated IDs | e.g. `nat-2024\|glr-2025\|glr-2026` |

---

## Related Pages

- [Components](components.md)
- [State Management](state.md)
- [API Client](api-client.md)
- [API Reference](../backend/api.md)
