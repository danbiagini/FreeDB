#!/bin/bash
set -euo pipefail

# Migration: v0.5 → v0.6
# - Rename timestamped containers back to their app names for stable .incus DNS
# - Update the registry with the new container names

echo "=== Migration v0.6: Stable container names ==="

REGISTRY="/etc/freedb/registry.json"
if [ ! -f "$REGISTRY" ]; then
  echo "No registry found, skipping"
  echo ""
  echo "=== Migration v0.6 complete ==="
  exit 0
fi

echo "Checking for containers that need renaming..."

# Parse each app's container_name from the registry
python3 -c "
import json, sys
with open('$REGISTRY') as f:
    reg = json.load(f)
for name, app in reg.get('apps', {}).items():
    cn = app.get('container_name', name)
    if cn != name and cn != '':
        print(f'{name}|{cn}')
" 2>/dev/null | while IFS='|' read -r APP_NAME CONTAINER_NAME; do
  [ -z "$APP_NAME" ] && continue

  echo ""
  echo "  $CONTAINER_NAME -> $APP_NAME"

  # Check if the container exists
  if ! sudo incus info "$CONTAINER_NAME" &>/dev/null; then
    echo "    Container $CONTAINER_NAME not found, skipping"
    continue
  fi

  # Check if target name already exists
  if sudo incus info "$APP_NAME" &>/dev/null; then
    echo "    Container $APP_NAME already exists, skipping"
    continue
  fi

  # Stop, rename, start
  echo -n "    Stopping... "
  sudo incus stop "$CONTAINER_NAME" 2>/dev/null || true
  echo -n "Renaming... "
  if sudo incus rename "$CONTAINER_NAME" "$APP_NAME" 2>/dev/null; then
    echo -n "Starting... "
    sudo incus start "$APP_NAME" 2>/dev/null || true
    echo "Done"

    # Update registry
    python3 -c "
import json
with open('$REGISTRY') as f:
    reg = json.load(f)
app = reg['apps']['$APP_NAME']
app['container_name'] = '$APP_NAME'
with open('$REGISTRY', 'w') as f:
    json.dump(reg, f, indent=2)
" 2>/dev/null || echo "    Warning: could not update registry"
  else
    echo "FAILED"
    echo "    Restarting with original name..."
    sudo incus start "$CONTAINER_NAME" 2>/dev/null || true
  fi
done

echo ""
echo "=== Migration v0.6 complete ==="
echo ""
echo "Containers now use stable names for .incus DNS resolution."
echo "Future deploys will maintain stable names automatically."
