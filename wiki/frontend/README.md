# Frontend Overview

The frontend is a React 18 single-page application (SPA) built with Vite 5. It communicates with the backend API via fetch-based API client modules and renders using Material UI (MUI) v6 components.

---

## Tech Stack

| Library | Version | Purpose |
|---|---|---|
| React | 18.3.1 | UI framework |
| Vite | 5.4.0 | Build tool and dev server |
| MUI (Material UI) | 6.1.0 | Component library |
| React Router | 6.26.0 | Client-side routing |
| Recharts | 2.13.0 | Charts on the Stats page |
| @fontsource/montserrat | 5.1.0 | Self-hosted Montserrat font |

---

## Application Shell Structure

```
main.jsx
└── App (ColorModeContext.Provider + ThemeProvider + AuthProvider + BrowserRouter)
    └── AppLayout
        ├── NavBar (rendered only when authenticated)
        └── Routes
            ├── /login          → LoginPage (no auth)
            ├── /home           → ProtectedRoute → CheckInPage
            ├── /competitors    → ProtectedRoute → CompetitorsPage
            ├── /stats          → ProtectedRoute → StatsPage
            ├── /events         → AdminRoute → EventsPage
            ├── /audit          → AdminRoute → AuditPage
            ├── /manage-users   → AdminRoute → ManageUsersPage
            ├── /import         → AdminRoute → ImportPage
            └── *               → Navigate to /home
```

**`ProtectedRoute`:** Redirects to `/login` if `token` is null.  
**`AdminRoute`:** Redirects to `/login` if no token; redirects to `/home` if token exists but `isAdmin` is false.  
**`AppLayout`:** Renders `NavBar` only when authenticated. Applies padding to the main content area when authenticated.

---

## Auth State Flow

1. User visits any route → `ProtectedRoute` or `AdminRoute` checks `token` from `AuthContext`
2. If no token → redirect to `/login`
3. `LoginPage` step 1: user enters access code → local state only (no API call yet)
4. `LoginPage` step 2: user enters name → calls `POST /api/auth/token`
5. On success: `login(token, firstName, lastName, role)` writes to `localStorage` (`agm_token`, `agm_staff`) and sets `auth` state
6. Redirect to `/home`
7. On every page mount and on `visibilitychange` (tab focus): `AuthContext` calls `GET /api/auth/me` to sync role
8. If any API call returns 401: `apiFetch` clears `localStorage` and hard-redirects to `/login`
9. On logout: `logout()` clears `localStorage`, sets `auth` to null → `ProtectedRoute` redirects to `/login`

---

## Theme and Styling

**Theme file:** `src/theme.js`  
**Function:** `buildTheme(mode: 'light' | 'dark') => MUI theme`

| Setting | Value |
|---|---|
| Primary color | `#1565C0` (deep blue) |
| Secondary color | `#00897B` (teal) |
| Font | Montserrat (weights 400, 500, 600, 700 self-hosted via @fontsource) |
| Light mode background | `#F0F4F8` |
| Dark mode background | `#0e1117` |
| Dark mode paper | `#1a1f2e` |

Color mode (light/dark) is toggled by the `ColorModeContext` in `App.jsx`. The current mode is persisted in `localStorage` under `colorMode`. On first load, the system preference (`prefers-color-scheme`) is checked.

---

## Responsive Design

The application uses MUI's `xs`/`md` breakpoints for responsive layout. The primary breakpoint is `md` (960px):

- **NavBar:** Below `md`, shows a hamburger icon; clicking it opens a right-side `Drawer`. At `md` and above, shows inline navigation buttons.
- **CompetitorsPage:** Below `md`, renders `Paper` cards (one per competitor). At `md` and above, renders a sortable `Table`.
- **Check-In page:** Single-column layout with a constrained max-width of 680px, works well on mobile.
- **Stats page:** Uses MUI `Grid` for responsive chart layout.

---

## Key Design Decisions

- **Server-side search:** The Check-In page does not load all competitors on mount. It sends debounced search queries to the server (300ms debounce) and displays only matching results.
- **Client-side filtering/sorting:** The Competitors page loads all competitors once and handles filtering and sorting in the browser.
- **VITE_API_URL baked at build time:** The API base URL is injected at Docker build time via `--build-arg`. Changing the API domain requires a frontend rebuild and redeploy.
- **Column visibility persistence:** The Competitors page persists column visibility choices in `localStorage` under `agm_competitors_columns`.

---

## Related Pages

- [Pages](pages.md)
- [Components](components.md)
- [State Management](state.md)
- [API Client](api-client.md)
