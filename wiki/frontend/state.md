# State Management

The application has two global React contexts: `AuthContext` and `ColorModeContext`. There is no Redux, Zustand, or other state management library. Page-level data is fetched directly in each page component and stored in local state.

---

## AuthContext

**File:** `src/context/AuthContext.jsx`  
**Provider:** `AuthProvider` wraps the entire app in `App.jsx`

### Purpose

Manages authentication state: the Bearer token, staff info (name and role), and derived `isAdmin` flag. Persists state to `localStorage` so users stay logged in across page refreshes.

### localStorage keys

| Key | Value | Description |
|---|---|---|
| `agm_token` | `string` | The raw Bearer token hex string |
| `agm_staff` | `JSON string` | `{ firstName, lastName, role }` |

### Context value shape

```js
{
  token: string | null,        // Bearer token, or null if not logged in
  staff: {                     // null if not logged in
    firstName: string,
    lastName: string,
    role: "registration" | "admin"
  } | null,
  isAdmin: boolean,            // derived: staff?.role === "admin"
  login: (token, firstName, lastName, role) => void,
  logout: () => void
}
```

### `login(token, firstName, lastName, role)`

1. Builds `staff = { firstName, lastName, role }`
2. Writes `agm_token` and `agm_staff` to `localStorage`
3. Sets `auth` state to `{ token, staff }`

### `logout()`

1. Removes `agm_token` and `agm_staff` from `localStorage`
2. Sets `auth` state to `null`

### Role sync (useEffect)

On mount and whenever the page becomes visible (`visibilitychange` event fires with `document.visibilityState === 'visible'`), the context runs `syncRole()`:

1. Reads current auth from `localStorage` (not React state — avoids stale closure)
2. Calls `GET /api/auth/me` with the stored token
3. If `401`: clears both localStorage keys and sets `auth` to `null` (forced logout — token has been revoked)
4. If `200`: updates `staff.role` in localStorage and in React state

This ensures that an admin who has their role changed (or token revoked) by another admin is updated within seconds of switching back to the tab.

### `useAuth()` hook

```js
import { useAuth } from '../context/AuthContext'
const { token, staff, isAdmin, login, logout } = useAuth()
```

Available in any component inside `AuthProvider`.

### BASE_URL

`AuthContext` uses `import.meta.env.VITE_API_URL || 'http://localhost:8080'` for the `syncRole` call. This is the same base URL used by the API client modules.

---

## ColorModeContext

**File:** `src/App.jsx`  
**Provider:** `ColorModeContext.Provider` wraps the entire app

### Purpose

Manages the light/dark color mode toggle. Persists the mode to `localStorage` under `colorMode`.

### Context value shape

```js
{
  toggle: () => void,     // toggles between "light" and "dark"
  mode: "light" | "dark"
}
```

### Initial mode

On first load, reads `localStorage.getItem('colorMode')`. If `'light'` or `'dark'`, uses that value. Otherwise, checks `window.matchMedia('(prefers-color-scheme: dark)')` and uses the system preference.

### `useColorMode()` hook

```js
import { useColorMode } from '../App'
const { mode, toggle } = useColorMode()
```

Used by `NavBar` to show the appropriate icon and call `toggle`.

### Theme integration

`App.jsx` passes `mode` to `buildTheme(mode)` in a `useMemo` call. The MUI `ThemeProvider` re-renders with the new theme whenever `mode` changes.

---

## Page-level State

Each page manages its own data-fetching state (competitors, events, staff, audit logs) in local `useState` hooks. Data is fetched in `useEffect` on mount (with `useCallback` for stable fetch function references). There is no global cache or shared server state.

The Competitors page persists column visibility to `localStorage` under `agm_competitors_columns` (array of visible column keys), independent of both contexts.

---

## Related Pages

- [Frontend Overview](README.md)
- [Components](components.md)
- [API Client](api-client.md)
