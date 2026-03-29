#!/bin/bash
set -euo pipefail

# Migration: v0.2 → v0.3
# - HTTPS redirect in Traefik
# - Remove 8080 from network forward
# - PostgreSQL: trust → scram-sha-256 with password generation

SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
REPO_ROOT="${SCRIPT_DIR}/../.."

echo "=== Migration v0.3: Security hardening ==="

# ---------------------------------------------------------------
# 1. HTTPS redirect — push updated traefik.toml
# ---------------------------------------------------------------
echo ""
echo "1. Updating Traefik config (HTTPS redirect)..."

if sudo incus info proxy1 &>/dev/null; then
  sudo incus file push "${REPO_ROOT}/platform/config/traefik.toml" proxy1/etc/traefik/
  sudo incus exec proxy1 -- systemctl restart traefik
  echo "   Done — HTTP now redirects to HTTPS"
else
  echo "   Skipped — proxy1 not running"
fi

# ---------------------------------------------------------------
# 2. Remove 8080 from network forward
# ---------------------------------------------------------------
echo ""
echo "2. Removing dashboard port (8080) from network forward..."

# Detect host internal IP
if command -v curl &>/dev/null; then
  HOST_IP=$(curl -sf -m 2 -H "Metadata-Flavor: Google" \
    http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip 2>/dev/null || \
    (TOKEN=$(curl -sf -m 2 -X PUT -H "X-aws-ec2-metadata-token-ttl-seconds: 10" \
      http://169.254.169.254/latest/api/token 2>/dev/null) && \
    curl -sf -H "X-aws-ec2-metadata-token: $TOKEN" \
      http://169.254.169.254/latest/meta-data/local-ipv4 2>/dev/null) || \
    ip -4 route get 1.0.0.0 2>/dev/null | grep -oP 'src \K\S+' || echo "")
fi

if [ -n "${HOST_IP:-}" ]; then
  sudo incus network forward port remove incusbr0 "$HOST_IP" tcp 8080 2>/dev/null && \
    echo "   Done — port 8080 removed from forward" || \
    echo "   Skipped — port 8080 was not forwarded"
else
  echo "   Skipped — could not detect host IP"
fi

echo "   Dashboard accessible via: ssh -L 8080:proxy1.incus:8080 <host>"

# ---------------------------------------------------------------
# 3. PostgreSQL: trust → scram-sha-256
# ---------------------------------------------------------------
echo ""
echo "3. Upgrading PostgreSQL authentication..."

if ! sudo incus info db1 &>/dev/null; then
  echo "   Skipped — db1 not running"
  exit 0
fi

# Detect PG version
PG_VERSION=$(sudo incus exec db1 -- sh -c "ls /etc/postgresql/ | head -1")
PG_HBA="/etc/postgresql/${PG_VERSION}/main/pg_hba.conf"

# Check if already migrated
if sudo incus exec db1 -- grep -q "scram-sha-256" "$PG_HBA" 2>/dev/null; then
  echo "   Already using scram-sha-256 — skipping"
else
  # Generate passwords for all app database users
  REGISTRY="/etc/freedb/registry.json"
  if [ -f "$REGISTRY" ]; then
    echo "   Generating passwords for existing database users..."

    # Get list of apps with databases
    APPS_WITH_DB=$(python3 -c "
import json, sys
with open('$REGISTRY') as f:
    reg = json.load(f)
for name, app in reg.get('apps', {}).items():
    if app.get('has_db') and app.get('db_name'):
        container = app.get('container_name', name)
        print(f\"{name}|{app['db_name']}|{container}\")
" 2>/dev/null || echo "")

    DB1_IP=$(sudo incus query '/1.0/instances/db1?recursion=1' 2>/dev/null | \
      python3 -c "import sys,json; print([a['address'] for a in json.load(sys.stdin)['state']['network']['eth0']['addresses'] if a['family']=='inet'][0])" 2>/dev/null || echo "db1.incus")

    while IFS='|' read -r APP_NAME DB_NAME CONTAINER_NAME; do
      [ -z "$APP_NAME" ] && continue

      # Generate password
      PASSWORD=$(openssl rand -hex 12)

      echo "   Setting password for user ${DB_NAME}..."
      sudo incus exec db1 -- sudo -u postgres psql -c \
        "ALTER USER ${DB_NAME} WITH PASSWORD '${PASSWORD}'" 2>/dev/null || true

      # Update DATABASE_URL in the container
      if sudo incus info "$CONTAINER_NAME" &>/dev/null; then
        # Find the current DATABASE_URL env var name
        DB_ENV_VAR=$(python3 -c "
import json
with open('$REGISTRY') as f:
    reg = json.load(f)
app = reg.get('apps', {}).get('$APP_NAME', {})
print(app.get('db_env_var', 'DATABASE_URL'))
" 2>/dev/null || echo "DATABASE_URL")

        NEW_URL="postgresql://${DB_NAME}:${PASSWORD}@${DB1_IP}:5432/${DB_NAME}?sslmode=disable"
        echo "   Updating ${DB_ENV_VAR} on container ${CONTAINER_NAME}..."
        sudo incus config set "$CONTAINER_NAME" "environment.${DB_ENV_VAR}=${NEW_URL}" 2>/dev/null || true
      fi
    done <<< "$APPS_WITH_DB"
  fi

  # Update pg_hba.conf
  echo "   Switching pg_hba.conf from trust to scram-sha-256..."
  sudo incus exec db1 -- sudo -u postgres sed -i \
    's/10\.0\.0\.1\/24             trust/10.0.0.1\/24             scram-sha-256/' "$PG_HBA"

  # Restart postgres
  sudo incus exec db1 -- systemctl restart postgresql
  echo "   Done — PostgreSQL now requires password authentication"

  # Restart app containers to pick up new DATABASE_URL
  if [ -f "$REGISTRY" ] && [ -n "${APPS_WITH_DB:-}" ]; then
    echo "   Restarting app containers..."
    while IFS='|' read -r APP_NAME DB_NAME CONTAINER_NAME; do
      [ -z "$CONTAINER_NAME" ] && continue
      if sudo incus info "$CONTAINER_NAME" &>/dev/null; then
        echo "   Restarting ${CONTAINER_NAME}..."
        sudo incus restart "$CONTAINER_NAME" 2>/dev/null || true
      fi
    done <<< "$APPS_WITH_DB"
  fi
fi

echo ""
echo "=== Migration v0.3 complete ==="
