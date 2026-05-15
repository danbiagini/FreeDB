#!/bin/bash
set -euo pipefail

# Migration: v0.7 → v1.0
# - Populate `domains` array from legacy `domain` string in registry.json
#   so the multi-domain feature has a single authoritative field.

echo "=== Migration v1.0: Multi-domain registry schema ==="

REGISTRY="/etc/freedb/registry.json"

if [ ! -f "$REGISTRY" ]; then
  echo "No registry found — nothing to migrate"
  echo ""
  echo "=== Migration v1.0 complete ==="
  exit 0
fi

echo "1. Migrating $REGISTRY..."

python3 - <<'EOF'
import json, sys

with open("/etc/freedb/registry.json") as f:
    reg = json.load(f)

changed = 0
for name, app in reg.get("apps", {}).items():
    if app.get("domain") and not app.get("domains"):
        app["domains"] = [app["domain"]]
        changed += 1

if changed:
    with open("/etc/freedb/registry.json", "w") as f:
        json.dump(reg, f, indent=2)
        f.write("\n")
    print(f"   Migrated {changed} app(s)")
else:
    print("   Already up to date — no changes needed")
EOF

echo ""
echo "=== Migration v1.0 complete ==="
