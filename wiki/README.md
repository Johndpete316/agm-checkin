# AGM Check-In — Wiki

AGM Check-In is a web application used by registration staff at the AGM Music Competition to manage competitor check-in. It stores competitor records, tracks check-in status per event, validates minor competitors' identities, and provides administrators with event management, staff management, audit logging, and bulk data import capabilities. The system is designed to expand beyond check-in into broader competition data services.

---

## Tech Stack

| Layer | Technology |
|---|---|
| **Frontend** | React 18, Vite 5, MUI v6 (Material UI), React Router v6, Recharts |
| **Backend** | Go 1.22, chi router v5, GORM v1 (PostgreSQL driver) |
| **Database** | PostgreSQL 16 (GORM AutoMigrate for schema) |
| **Container runtime** | Docker (build), containerd via k3s (production), microk8s (DR) |
| **Orchestration** | k3s (production), microk8s (disaster recovery) |
| **Packaging** | Helm 3 |
| **Network ingress** | Cloudflare Tunnel (cloudflared) — no open inbound ports |
| **Backup / DR remote** | rclone (configured against Cloudflare R2 or any compatible remote) |
| **Static serving** | nginx (inside frontend container) |
| **Font** | Montserrat (self-hosted via @fontsource) |
| **Theme** | Primary `#1565C0` (blue), secondary `#00897B` (teal); light and dark modes |

---

## Wiki Sections

### Infrastructure
- [Infrastructure Overview](infrastructure/README.md) — Architecture diagram, k3s cluster, Cloudflare Tunnel
- [Infrastructure as Code](infrastructure/iac.md) — Helm chart structure, secrets, values
- [CI/CD](infrastructure/cicd.md) — Deploy workflow, image distribution, no registry approach
- [Environments](infrastructure/environments.md) — Production and local development
- [Disaster Recovery](infrastructure/disaster-recovery.md) — Backup strategy, DR runbook, restore-to-production

### Backend
- [Backend Overview](backend/README.md) — Architecture, request lifecycle, auth model
- [API Reference](backend/api.md) — Every route: method, auth, params, response shapes
- [Service Layer](backend/services.md) — All service files, methods, inputs, outputs
- [Middleware & Utilities](backend/functions.md) — IPBlocklist, RequireToken, RequireAdmin, helpers
- [Database](backend/database.md) — All models, fields, constraints, ERD, relationships
- [External Integrations](backend/external-integrations.md) — Cloudflare Tunnel IP headers

### Frontend
- [Frontend Overview](frontend/README.md) — App structure, auth flow, responsive design
- [Pages](frontend/pages.md) — Every page: route, data, components, auth requirements
- [Components](frontend/components.md) — Every reusable component: props, state, callbacks
- [State Management](frontend/state.md) — AuthContext, ColorModeContext, localStorage
- [API Client](frontend/api-client.md) — All fetch modules, auth injection, 401 handling

### Other
- [Tech Debt & Known Limitations](tech-debt.md)
