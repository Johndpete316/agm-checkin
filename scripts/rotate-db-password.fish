#!/usr/bin/env fish

# Rotates the postgres password.
#
# Usage:
#   ./rotate-db-password.fish --generate     # make up a strong one
#   ./rotate-db-password.fish                # prompt for one
#   ./rotate-db-password.fish --dry-run      # show the plan, change nothing
#
# Order matters and is not obvious. POSTGRES_PASSWORD in the StatefulSet only
# does anything when it initialises an empty PGDATA, so editing values and
# redeploying does NOT change the password of an existing database — it only
# changes what the clients try to authenticate with. Change one without the
# other and every pod fails to connect. So: ALTER USER first, then values, then
# deploy, in one go.
#
# There is a short window between the ALTER and the rollout finishing where
# already-open connections keep working but new ones are rejected. Run this when
# a few seconds of connection errors is acceptable, not mid-event.

set -l DRY_RUN 0
set -l GENERATE 0

for arg in $argv
    switch $arg
        case --dry-run
            set DRY_RUN 1
        case --generate
            set GENERATE 1
        case --help -h
            echo "Usage: ./rotate-db-password.fish [--generate] [--dry-run]"
            exit 0
        case '*'
            echo "Unknown flag: $arg"
            exit 1
    end
end

set -l RELEASE   agm-checkin
set -l CHART_DIR (dirname (status --current-filename))/../helm/agm-checkin
set -l SECRETS   $CHART_DIR/values.secret.yaml

if not test -f $SECRETS
    echo "Secrets file not found: $SECRETS"
    exit 1
end

# --- work out the new password -------------------------------------------------

set -l NEW_PW
if test $GENERATE -eq 1
    # Alphanumeric only: this string gets embedded in a libpq DSN and passed
    # through a SQL string literal, so avoid quotes, backslashes and spaces.
    set NEW_PW (openssl rand -base64 48 | tr -dc 'A-Za-z0-9' | head -c 32)
else
    read -s -P "New postgres password: " NEW_PW
    echo
    read -s -P "Confirm: " NEW_PW_CONFIRM
    echo
    if test "$NEW_PW" != "$NEW_PW_CONFIRM"
        echo "Passwords do not match."
        exit 1
    end
end

if test -z "$NEW_PW"
    echo "Empty password, refusing."
    exit 1
end

if string match -qr '[^A-Za-z0-9._~-]' -- "$NEW_PW"
    echo "Password contains characters that need escaping in a libpq DSN or SQL literal."
    echo "Stick to letters, digits and . _ ~ -"
    exit 1
end

# --- locate the running postgres pod -------------------------------------------

set -l PG_POD (kubectl get pod -l app=agm-postgres -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
if test -z "$PG_POD"
    if test $DRY_RUN -eq 1
        # Let --dry-run work away from the cluster; it changes nothing either way.
        set PG_POD "(none found — cluster unreachable?)"
    else
        echo "Could not find a running postgres pod (label app=agm-postgres)."
        exit 1
    end
end

set -l PG_USER (grep -A5 '^postgres:' $CHART_DIR/values.yaml | grep '^\s*user:' | head -1 | string replace -r '^\s*user:\s*' '' | string trim -c '"')
if test -z "$PG_USER"
    set PG_USER postgres
end

echo "Release:  $RELEASE"
echo "Pod:      $PG_POD"
echo "DB user:  $PG_USER"
echo "Secrets:  $SECRETS"
if test $GENERATE -eq 1
    echo "Password: (generated, 32 chars — will be written to the secrets file)"
end

if test $DRY_RUN -eq 1
    echo
    echo "--dry-run: would ALTER USER, rewrite the secrets file, helm upgrade, restart api."
    exit 0
end

echo
read -P "This changes the live database password. Continue? [y/N] " CONFIRM
if test "$CONFIRM" != y -a "$CONFIRM" != Y
    echo "Aborted."
    exit 1
end

# --- 1. change it in the database ----------------------------------------------

echo "==> ALTER USER on $PG_POD"
# Piped via stdin so the password never appears in the pod's argv.
echo "ALTER USER \"$PG_USER\" WITH PASSWORD '$NEW_PW';" \
    | kubectl exec -i $PG_POD -- psql -U $PG_USER -d postgres -v ON_ERROR_STOP=1 -q -f -
or begin
    echo "ALTER USER failed. Nothing else has changed; the old password is still in effect."
    exit 1
end

# --- 2. update the secrets file -------------------------------------------------

set -l BACKUP $SECRETS.bak.(date -u +%Y%m%dT%H%M%SZ)
cp $SECRETS $BACKUP
echo "==> Backed up secrets file to $BACKUP"

# Replace `password:` only inside the top-level `postgres:` block — pgadmin has a
# password key too, and a blanket substitution would clobber it.
awk -v newpw="$NEW_PW" '
    /^[a-zA-Z_-]+:/ { in_pg = ($0 ~ /^postgres:/) }
    in_pg && /^[[:space:]]+password:/ {
        match($0, /^[[:space:]]+/)
        print substr($0, 1, RLENGTH) "password: \"" newpw "\""
        next
    }
    { print }
' $SECRETS > $SECRETS.tmp
and mv $SECRETS.tmp $SECRETS
or begin
    echo "Rewriting the secrets file failed. The DATABASE password has already changed."
    echo "Restore with: cp $BACKUP $SECRETS, then set postgres.password by hand and deploy."
    exit 1
end

if not grep -q "$NEW_PW" $SECRETS
    echo "The new password is not in $SECRETS after rewriting — check the postgres: block."
    echo "The database password HAS changed. Fix the file by hand, then run helm upgrade."
    exit 1
end
echo "==> Updated postgres.password in the secrets file"

# --- 3. redeploy ---------------------------------------------------------------

echo "==> helm upgrade"
helm upgrade --install $RELEASE $CHART_DIR -f $SECRETS
or begin
    echo "Helm upgrade failed. The database password has changed and pods still hold the old one."
    echo "Re-run: helm upgrade --install $RELEASE $CHART_DIR -f $SECRETS"
    exit 1
end

echo "==> Restarting api and cf-sync consumers"
kubectl rollout restart deployment/$RELEASE-api
kubectl rollout status deployment/$RELEASE-api --timeout=120s
or begin
    echo "API did not become ready. Check: kubectl logs -l app=agm-api --tail=50"
    exit 1
end

echo
echo "Done. Verify with:"
echo "  kubectl exec deploy/$RELEASE-api -- wget -qO- localhost:8080/health"
echo
echo "The old password is still in $BACKUP — delete it once you are satisfied:"
echo "  rm $BACKUP"
