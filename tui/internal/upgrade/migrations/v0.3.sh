#!/bin/bash
set -euo pipefail

# Migration: v0.2 → v0.3
# - HTTPS redirect in Traefik
# - Remove 8080 from network forward
# - PostgreSQL: trust → scram-sha-256 with password generation
#
# This script is self-contained (embedded in the freedb binary).
# It patches existing configs in-place rather than pushing files from the repo.

echo "=== Migration v0.3: Security hardening ==="

# ---------------------------------------------------------------
# 1. HTTPS redirect — patch traefik.toml in proxy1
# ---------------------------------------------------------------
echo ""
echo "1. Updating Traefik config (HTTPS redirect)..."

if sudo incus info proxy1 &>/dev/null; then
  # Check if redirect is already configured
  if sudo incus exec proxy1 -- grep -q "redirections" /etc/traefik/traefik.toml 2>/dev/null; then
    echo "   Already configured — skipping"
  else
    # Add redirect config after the web entrypoint address line
    sudo incus exec proxy1 -- sed -i '/\[entryPoints.web\]/{n;s|address = ":80"|address = ":80"\n    [entryPoints.web.http.redirections.entryPoint]\n      to = "websecure"\n      scheme = "https"|;}' /etc/traefik/traefik.toml
    sudo incus exec proxy1 -- systemctl restart traefik
    echo "   Done — HTTP now redirects to HTTPS"
  fi
else
  echo "   Skipped — proxy1 not running"
fi

# ---------------------------------------------------------------
# 2. Remove 8080 from network forward
# ---------------------------------------------------------------
echo ""
echo "2. Removing dashboard port (8080) from network forward..."

# Detect host internal IP
HOST_IP=""
if curl -sf -m 2 -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/ >/dev/null 2>&1; then
  HOST_IP=$(curl -sf -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip 2>/dev/null || echo "")
elif TOKEN=$(curl -sf -m 2 -X PUT -H "X-aws-ec2-metadata-token-ttl-seconds: 10" http://169.254.169.254/latest/api/token 2>/dev/null); then
  HOST_IP=$(curl -sf -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/local-ipv4 2>/dev/null || echo "")
fi
if [ -z "$HOST_IP" ]; then
  HOST_IP=$(ip -4 route get 1.0.0.0 2>/dev/null | grep -oP 'src \K\S+' || echo "")
fi

if [ -n "$HOST_IP" ]; then
  # Parse forward entries to find any that include 8080
  FORWARD_OUTPUT=$(sudo incus network forward show incusbr0 "$HOST_IP" 2>/dev/null || echo "")

  # Find the listen_port value and target_address for entries containing 8080
  LISTEN_PORT=""
  PROXY1_IP=""
  PREV_LINE=""
  while IFS= read -r line; do
    if echo "$line" | grep -q "listen_port:"; then
      PREV_LINE=$(echo "$line" | sed 's/.*listen_port: *//' | tr -d '"')
    fi
    if echo "$line" | grep -q "target_address:" && echo "$PREV_LINE" | grep -q "8080"; then
      LISTEN_PORT="$PREV_LINE"
      PROXY1_IP=$(echo "$line" | sed 's/.*target_address: *//')
      break
    fi
  done <<< "$FORWARD_OUTPUT"

  if [ -n "$PROXY1_IP" ] && [ -n "$LISTEN_PORT" ]; then
    # Remove the existing entry
    sudo incus network forward port remove incusbr0 "$HOST_IP" tcp "$LISTEN_PORT" 2>/dev/null || true

    # Re-add without 8080
    NEW_PORTS=$(echo "$LISTEN_PORT" | sed 's/,8080//;s/8080,//;s/8080//')
    if [ -n "$NEW_PORTS" ]; then
      sudo incus network forward port add incusbr0 "$HOST_IP" tcp "$NEW_PORTS" "$PROXY1_IP" 2>/dev/null || true
      echo "   Done — removed 8080, keeping ${NEW_PORTS} -> ${PROXY1_IP}"
    else
      echo "   Done — removed port forward entirely (was only 8080)"
    fi
  else
    echo "   Skipped — port 8080 not found in forwards"
  fi
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
if sudo incus exec db1 -- grep -q "10\.0\.0\.1/24.*scram-sha-256" "$PG_HBA" 2>/dev/null; then
  echo "   Already using scram-sha-256 — skipping"
else
  # Generate passwords for all app database users
  REGISTRY="/etc/freedb/registry.json"
  if [ -f "$REGISTRY" ]; then
    echo "   Generating passwords for existing database users..."

    DB1_IP="db1.incus"

    # Process each app with a database
    python3 -c "
import json
with open('$REGISTRY') as f:
    reg = json.load(f)
for name, app in reg.get('apps', {}).items():
    if app.get('has_db') and app.get('db_name'):
        container = app.get('container_name', name)
        db_env_var = app.get('db_env_var', 'DATABASE_URL')
        print(f\"{name}|{app['db_name']}|{container}|{db_env_var}\")
" 2>/dev/null | while IFS='|' read -r APP_NAME DB_NAME CONTAINER_NAME DB_ENV_VAR; do
      [ -z "$APP_NAME" ] && continue

      # Generate password
      PASSWORD=$(openssl rand -hex 12)

      echo "   Setting password for user ${DB_NAME}..."
      sudo incus exec db1 -- sudo -u postgres psql -c \
        "ALTER USER \"${DB_NAME}\" WITH PASSWORD '${PASSWORD}'" 2>/dev/null || true

      # Update DATABASE_URL in the container
      if sudo incus info "$CONTAINER_NAME" &>/dev/null; then
        NEW_URL="postgresql://${DB_NAME}:${PASSWORD}@${DB1_IP}:5432/${DB_NAME}?sslmode=disable"
        echo "   Updating ${DB_ENV_VAR} on container ${CONTAINER_NAME}..."
        sudo incus config set "$CONTAINER_NAME" "environment.${DB_ENV_VAR}" "${NEW_URL}" 2>/dev/null || true
      fi
    done
  fi

  # Update pg_hba.conf
  echo "   Switching pg_hba.conf from trust to scram-sha-256..."
  sudo incus exec db1 -- sudo -u postgres sed -i \
    's/10\.0\.0\.1\/24             trust/10.0.0.1\/24             scram-sha-256/' "$PG_HBA"

  # Restart postgres
  sudo incus exec db1 -- systemctl restart postgresql
  echo "   Done — PostgreSQL now requires password authentication"

  # Restart app containers to pick up new DATABASE_URL
  if [ -f "$REGISTRY" ]; then
    echo "   Restarting app containers with databases..."
    python3 -c "
import json
with open('$REGISTRY') as f:
    reg = json.load(f)
for name, app in reg.get('apps', {}).items():
    if app.get('has_db'):
        print(app.get('container_name', name))
" 2>/dev/null | while read -r CONTAINER_NAME; do
      [ -z "$CONTAINER_NAME" ] && continue
      if sudo incus info "$CONTAINER_NAME" &>/dev/null; then
        echo "   Restarting ${CONTAINER_NAME}..."
        sudo incus restart "$CONTAINER_NAME" 2>/dev/null || true
      fi
    done
  fi
fi

echo ""
echo "=== Migration v0.3 complete ==="
