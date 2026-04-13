# Components

Reusable components live in `src/components/`. Each is a single-file React component.

---

## NavBar

**File:** `src/components/NavBar.jsx`  
**Used by:** `AppLayout` in `App.jsx` (rendered only when authenticated)

**Purpose:** Application navigation bar with responsive hamburger/drawer on mobile and inline links on desktop.

### Props

None. Reads all data from context.

### Internal state

| State | Type | Description |
|---|---|---|
| `drawerOpen` | `boolean` | Whether the mobile drawer is open |

### Context consumed

- `useColorMode()` from `App.jsx` — `mode` (current color mode), `toggle` (function)
- `useAuth()` from `AuthContext` — `staff` (name display), `isAdmin` (show admin links), `logout`
- `useLocation()` from React Router — for active link highlighting
- `useNavigate()` from React Router — for post-logout navigation

### Navigation links

| Role | Visible links |
|---|---|
| Registration | Check In, Competitors, Stats |
| Admin | Check In, Competitors, Stats, Events, Manage Users, Audit Log, Import Data |

### Layout

- **Desktop (md+):** MUI `AppBar` with inline `Button` components. Active route has `fontWeight: 700` and a white bottom border. Shows staff name. Provides dark/light mode toggle icon and sign-out icon.
- **Mobile (xs–sm):** Hamburger `MenuIcon` on the right side of the toolbar. Opens a right-anchor `Drawer` containing the staff name, nav links as `ListItemButton`, and mode toggle + sign-out options.

### AGM Logo

Displays `src/assets/agm-125th-logo.png` at 36px height in the toolbar.

---

## CompetitorCard

**File:** `src/components/CompetitorCard.jsx`  
**Used by:** `CheckInPage`

**Purpose:** Card UI for a single competitor result on the Check-In page. Shows key details and a Check In button. Handles the identity validation dialog for competitors who require validation.

### Props

| Prop | Type | Description |
|---|---|---|
| `competitor` | `CompetitorWithCheckIn` | The competitor object including `currentCheckIn` |
| `onCheckIn` | `(id: string) => void` | Called when check-in is confirmed |
| `onUpdate` | `(updated: Competitor) => void` | Called after DOB or validation updates |
| `loading` | `boolean` | Whether this competitor is currently being checked in |

### Internal state

| State | Type | Description |
|---|---|---|
| `dialogOpen` | `boolean` | Whether the validation dialog is showing |
| `editedDOB` | `string` | YYYY-MM-DD value in the DOB input |
| `originalDOB` | `string` | DOB at time dialog was opened (for change detection) |
| `confirming` | `boolean` | Whether the validation confirm API calls are in progress |
| `dialogError` | `string` | Error message inside the validation dialog |

### Behavior

**No validation required:** "Check In" button calls `onCheckIn(competitor.id)` directly.

**Validation required (`requiresValidation && !validated`):** "Check In" button opens the validation dialog. The dialog shows the competitor's studio and teacher for identity confirmation and an editable date-of-birth field. On confirm:
1. If `editedDOB !== originalDOB`: calls `updateCompetitorDOB(id, editedDOB)` → calls `onUpdate(updated)`
2. Calls `validateCompetitor(id)` → calls `onUpdate(validated)`
3. Closes dialog and calls `onCheckIn(id)`

### Display data

- Name, validation chip (warning amber / success green), check-in status chip (success / default)
- Age (calculated from DOB), formatted DOB, shirt size (highlighted group)
- Studio, teacher, email
- Note (shown as MUI Alert if present)
- Check-in timestamp and staff name (shown if checked in)

### Helper functions (module-level)

| Function | Description |
|---|---|
| `calculateAge(dob)` | Returns integer age from ISO date string; null if DOB is missing or before 1900 |
| `formatDOB(dob)` | Formats DOB as "Mar 15, 2005" using UTC; null if invalid |
| `toInputDate(dob)` | Formats DOB as "YYYY-MM-DD" for date input; empty string if invalid |

---

## EditCompetitorDialog

**File:** `src/components/EditCompetitorDialog.jsx`  
**Used by:** `CompetitorsPage`

**Purpose:** Admin-only dialog for editing all fields of an existing competitor, including viewing their event history.

### Props

| Prop | Type | Description |
|---|---|---|
| `competitor` | `CompetitorWithCheckIn \| null` | The competitor to edit; dialog is open when non-null |
| `onClose` | `() => void` | Called when dialog is dismissed without saving |
| `onSaved` | `(updated: Competitor) => void` | Called with the updated competitor after save |

### Internal state

| State | Type | Description |
|---|---|---|
| `form` | `object` | Controlled form values for all editable fields |
| `saving` | `boolean` | Whether save API call is in progress |
| `error` | `string` | Error message |
| `eventHistory` | `CompetitorEventWithEvent[]` | Loaded event history (non-critical, loaded async) |

### Editable fields

`nameFirst`, `nameLast`, `dateOfBirth` (date input), `email`, `studio`, `teacher`, `shirtSize` (select from hardcoded list), `lastRegisteredEvent` (select from hardcoded list `['glr-2026', 'nat-2025', 'glr-2025', 'nat-2024']`), `note` (multiline text), `requiresValidation` (switch), `validated` (switch)

### Hardcoded shirt sizes

`['Adult XL', 'Adult L', 'Adult M', 'Adult S', 'Youth XL', 'Youth L', 'Youth M', 'Youth S']`

### Event history section

Appears below the form fields if `eventHistory.length > 0`. Fetched via `getCompetitorEvents(competitor.id)` in a `useEffect` when `competitor` changes. Shows a table with columns: Event, Checked In (icon), Check-In Time, By.

### API calls

On save: `updateCompetitor(competitor.id, payload)` where payload is the full competitor object with form values merged in. DOB is serialized as `"YYYY-MM-DDT00:00:00Z"`.

---

## AddCompetitorDialog

**File:** `src/components/AddCompetitorDialog.jsx`  
**Used by:** `CompetitorsPage`

**Purpose:** Admin-only dialog for creating a new competitor record.

### Props

| Prop | Type | Description |
|---|---|---|
| `open` | `boolean` | Whether the dialog is open |
| `onClose` | `() => void` | Called to close without saving |
| `onCreated` | `(created: Competitor) => void` | Called with the created competitor on success |

### Internal state

| State | Type | Description |
|---|---|---|
| `form` | `object` | Controlled form state |
| `saving` | `boolean` | API call in progress |
| `error` | `string` | Error message |

### Fields

`nameFirst` (required), `nameLast` (required), `dateOfBirth` (date input, optional), `email`, `studio`, `teacher`, `shirtSize` (select from `['XS', 'S', 'M', 'L', 'XL', 'XXL']`), `lastRegisteredEvent` (text input, auto-filled from current event), `requiresValidation` (switch)

### Auto-fill behavior

On open, calls `getCurrentEvent()` to pre-populate `lastRegisteredEvent` with the current event ID. Non-blocking — form still works if the call fails.

### Validation behavior

If `requiresValidation` is toggled off, `validated` is set to `true` in the payload. If on, `validated` is `false`.

If no DOB is provided, `dateOfBirth` defaults to `"0001-01-01T00:00:00Z"` in the payload.

### API calls

On save: `createCompetitor(payload)` via `src/api/competitors.js`.

---

## Related Pages

- [Pages](pages.md)
- [State Management](state.md)
- [API Client](api-client.md)
