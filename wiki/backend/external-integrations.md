# External Integrations

---

## Cloudflare Tunnel

**Purpose:** Public HTTPS ingress without exposing any inbound ports on the k3s nodes.

### How it works

Two `cloudflared` pods run in the cluster. Each pod makes outbound connections to Cloudflare's edge network and registers as a tunnel connector. Cloudflare routes inbound HTTPS traffic for the configured hostnames through these connectors to the internal Kubernetes services.

The backend itself has no direct awareness of Cloudflare except for one critical detail: IP address extraction.

### CF-Connecting-IP Header

When traffic passes through a Cloudflare Tunnel, the real client IP is included in the `CF-Connecting-IP` request header. The backend's `GetClientIP` function checks this header first:

```go
func GetClientIP(r *http.Request) string {
    if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
        return strings.TrimSpace(ip)
    }
    if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
        return strings.TrimSpace(strings.Split(ip, ",")[0])
    }
    // ... fall back to RemoteAddr
}
```

This IP is used in two places:
1. **IPBlocklist middleware** — looks up the client IP in `ip_blocklists` on every request
2. **Audit log entries** — records the IP alongside every mutating action

Without this header check, all requests would appear to come from the cloudflared pod IP (an internal cluster address), making per-IP brute-force protection nonfunctional.

### Cloudflare Zero Trust — pgAdmin Access

pgAdmin is exposed through the Cloudflare Tunnel at `pgadmin.checkin.reduxit.net`. To restrict access, a Cloudflare Access application should be configured in the Zero Trust dashboard for that hostname. This is not enforced at the application level — it relies entirely on Cloudflare's access policies.

### Configuration

Tunnel routing (hostname → internal service) is configured in the Cloudflare Zero Trust dashboard, not in code. See [Infrastructure Overview](README.md) for the routing table.

The tunnel token is stored as a Kubernetes Secret (`agm-checkin-cloudflared-secret`) and injected as the `TUNNEL_TOKEN` environment variable into cloudflared pods.

---

## rclone (Backup Remote)

**Purpose:** Upload compressed database backups to cloud storage for disaster recovery.

rclone is used by `scripts/backup-db.fish` on the production server. The remote is configured by name (`r2:agm-db-backup/postgres_backups` by default, pointing to Cloudflare R2). The DR script (`scripts/dr-deploy.fish`) uses the same remote to pull the latest backup when standing up the disaster recovery environment.

rclone is not integrated into the application code — it is a shell tool used by operational scripts only.

---

## No Other External Integrations

The application has no integrations with:
- Email services (no transactional email)
- SMS or notification services
- Payment processors
- Third-party analytics
- External identity providers (no OAuth, no SAML)
- External logging services (logs go to stdout/stderr, collected by Kubernetes)

---

## Related Pages

- [Backend Overview](README.md)
- [Middleware & Utilities](functions.md)
- [Infrastructure Overview](../infrastructure/README.md)
- [Disaster Recovery](../infrastructure/disaster-recovery.md)
