#!/bin/bash
set -euo pipefail

# FreeDB Release Test Script
# Guides you through the full release test, automating what it can
# and prompting for manual verification where needed.
#
# Usage: sudo ./release-test.sh

PASS=0
FAIL=0
SKIP=0
TEST_DB="releasetest_$$"

green()   { echo -e "\033[32m  PASS: $1\033[0m"; PASS=$((PASS + 1)); }
red()     { echo -e "\033[31m  FAIL: $1\033[0m"; FAIL=$((FAIL + 1)); }
yellow()  { echo -e "\033[33m  SKIP: $1\033[0m"; SKIP=$((SKIP + 1)); }
section() { echo ""; echo "========================================"; echo "  $1"; echo "========================================"; }

check() {
  local desc="$1"
  shift
  if "$@" > /dev/null 2>&1; then
    green "$desc"
  else
    red "$desc"
  fi
}

# Prompt for manual verification. Returns 0 if user confirms.
manual() {
  local desc="$1"
  echo ""
  echo -e "  \033[36m>> $desc\033[0m"
  read -r -p "     Did it work? [y/n/s(kip)] " answer
  case "$answer" in
    y|Y) green "$desc" ;;
    s|S) yellow "$desc" ;;
    *)   red "$desc" ;;
  esac
}

# Prompt user to do something, then verify
prompt_action() {
  local instruction="$1"
  local desc="$2"
  echo ""
  echo -e "  \033[36m>> $instruction\033[0m"
  read -r -p "     Press enter when ready..."
  manual "$desc"
}

echo ""
echo "FreeDB Release Test"
echo "==================="
echo ""
echo "Binary: $(freedb --version)"
echo "Version file: $(cat /etc/freedb/version 2>/dev/null || echo 'not found')"
echo ""
read -r -p "Press enter to begin..."

# ─── HEALTH CHECK ───────────────────────────────────────────────
section "Health Check"
check "freedb check passes" freedb check

# ─── APP LIST ───────────────────────────────────────────────────
section "App List"
check "freedb list runs" freedb list
echo ""
freedb list 2>/dev/null | head -20

# ─── CONTAINER STATUS ──────────────────────────────────────────
section "Container Status"
check "containers are running" incus list --format csv -c ns | grep -q RUNNING
echo ""
incus list --format csv -c ns 2>/dev/null

# Check for timestamped names
TIMESTAMPED=$(incus list --format csv -c n 2>/dev/null | grep -E '-[0-9]{4}-[0-9]{4}$' || true)
if [ -n "$TIMESTAMPED" ]; then
  red "found timestamped containers (v0.6 rename migration may not have run)"
  echo "$TIMESTAMPED" | sed 's/^/    /'
else
  green "all containers have stable names"
fi

# ─── DASHBOARD TUI ─────────────────────────────────────────────
section "Dashboard TUI"
prompt_action "Run: sudo freedb (then press q to exit)" "TUI dashboard launches"
manual "Resource summary line shows Mem, CPU, and Disk usage"

# ─── DATABASE MANAGEMENT ──────────────────────────────────────
section "Database Management"

# Automated: create and drop via psql
if incus exec db1 -- sudo -u postgres createdb -O postgres "$TEST_DB" 2>/dev/null; then
  green "create database via psql ($TEST_DB)"
  incus exec db1 -- sudo -u postgres dropdb "$TEST_DB" 2>/dev/null
  green "drop database via psql ($TEST_DB)"
else
  red "create database via psql"
fi

# Manual: TUI database management
prompt_action "In TUI: press [D] for databases, then [a] to create 'testdb', type the name and press enter" "TUI database creation works (name input accepts keystrokes)"
prompt_action "Select the test database and press [d], then [y] to drop it" "TUI database drop works"

# ─── BACKUP ────────────────────────────────────────────────────
section "Backup"
check "backup.env exists" test -f /opt/freedb/backup.env
check "backup script executable" test -x /opt/freedb/backup-db.sh

echo ""
echo "  Running backup..."
if bash -c '. /opt/freedb/backup.env && /opt/freedb/backup-db.sh' 2>&1; then
  green "backup completed"
else
  red "backup completed"
fi

check "status file exists" test -f /var/lib/freedb/backup-status.json
check "status has databases array" python3 -c "
import json
with open('/var/lib/freedb/backup-status.json') as f:
    d = json.load(f)
assert 'databases' in d and len(d['databases']) > 0
"

# Check timestamps in filenames
LATEST_BACKUP=$(ls -t /var/lib/freedb/backups/*.sql.gz 2>/dev/null | head -1)
if echo "$LATEST_BACKUP" | grep -qE '_[0-9]{8}_[0-9]{6}Z\.sql\.gz'; then
  green "backup filename has UTC timestamp"
else
  red "backup filename missing UTC timestamp: $LATEST_BACKUP"
fi

# Cloud upload
CLOUD_STATUS=$(python3 -c "
import json
with open('/var/lib/freedb/backup-status.json') as f:
    d = json.load(f)
statuses = set(db['cloud_upload'] for db in d['databases'])
print(','.join(statuses))
" 2>/dev/null || echo "unknown")
echo "  Cloud upload status: $CLOUD_STATUS"
if echo "$CLOUD_STATUS" | grep -q "uploaded"; then
  green "cloud upload succeeded"
else
  red "cloud upload: $CLOUD_STATUS"
fi

# Check cloud URL in status
check "status has cloud_url" python3 -c "
import json
with open('/var/lib/freedb/backup-status.json') as f:
    d = json.load(f)
urls = [db.get('cloud_url','') for db in d['databases'] if db['status']=='success']
assert any(u.startswith('s3://') or u.startswith('gs://') for u in urls)
"

# ─── BACKUP STATUS IN TUI ─────────────────────────────────────
section "Backup Status in TUI"
prompt_action "In TUI: press [D] for databases. Check the LAST BACKUP column shows dates and sizes." "Per-database backup status displays correctly"
prompt_action "Select a database — do you see Local and Cloud backup paths?" "Backup detail shows on selection"

# ─── RESTORE ───────────────────────────────────────────────────
section "Restore"

# CLI restore - list
echo ""
echo "  Available backups:"
for db in $(incus exec db1 -- sudo -u postgres psql -At -c "SELECT datname FROM pg_database WHERE datistemplate = false AND datname != 'postgres'" 2>/dev/null); do
  BACKUPS=$(ls /var/lib/freedb/backups/${db}_*.sql.gz 2>/dev/null | wc -l)
  echo "    $db: $BACKUPS backup(s)"
done

check "freedb restore lists backups" bash -c "freedb restore $(incus exec db1 -- sudo -u postgres psql -At -c \"SELECT datname FROM pg_database WHERE datistemplate = false AND datname NOT IN ('postgres') LIMIT 1\" 2>/dev/null) 2>&1 | grep -q 'Available backups\|sql.gz'"

# TUI restore
prompt_action "In TUI: [D] -> select a database -> [r] -> select a backup -> confirm restore" "TUI restore completes and app container restarts"

# ─── ACME EMAIL ────────────────────────────────────────────────
section "ACME Email"
ACME_OUT=$(freedb acme-email 2>&1 || true)
echo "  Current: $ACME_OUT"
check "acme-email get works" test -n "$ACME_OUT"

prompt_action "Run: sudo freedb acme-email test@example.com (then check with: sudo freedb acme-email)" "ACME email get/set works"

# ─── REGISTRY AUTH ─────────────────────────────────────────────
section "Registry Auth"
check "auth script exists" test -x /usr/local/bin/freedb-registry-auth.sh
check "auth env file exists" test -f /opt/freedb/registry-auth.env

if [ -f /home/incus/.config/containers/auth.json ]; then
  UPDATED=$(python3 -c "
import json
with open('/home/incus/.config/containers/auth.json') as f:
    print(json.load(f).get('_updated', 'none'))
" 2>/dev/null || echo "error")
  echo "  Auth last updated: $UPDATED"
  check "auth.json has _updated timestamp" test "$UPDATED" != "none"
else
  red "auth.json not found"
fi

# ─── DEPLOY WITH IMAGE CACHE CLEANUP ──────────────────────────
section "Deploy & Image Cache Cleanup"
prompt_action "Deploy an app update from TUI: [enter] on an app -> [u] -> enter tag -> [y]" "Deploy succeeds with container rename"

IMAGE_COUNT=$(incus image list --format csv -c f 2>/dev/null | wc -l)
echo "  Cached images after deploy: $IMAGE_COUNT"
if [ "$IMAGE_COUNT" -le 5 ]; then
  green "image cache cleaned up ($IMAGE_COUNT remaining)"
else
  red "too many cached images ($IMAGE_COUNT) — cleanup may not be working"
fi

manual "Container name is the app name (not timestamped) in incus list"

# ─── SYSTEM CONTAINER ─────────────────────────────────────────
section "System Container (no Traefik)"
prompt_action "In TUI: [a] -> name: 'test-sys' -> image: 'ubuntu/24.04/cloud' -> Expose via Traefik: n -> DB: n" "System container created without Traefik"
prompt_action "Run: sudo freedb destroy test-sys --yes" "System container cleaned up"

# ─── MULTI-DOMAIN ─────────────────────────────────────────────
section "Multi-Domain Support"

# Migration: existing apps should have domains array
check "registry uses domains array" python3 -c "
import json
with open('/etc/freedb/registry.json') as f:
    reg = json.load(f)
apps = reg.get('apps', {})
assert len(apps) > 0, 'no apps in registry'
for name, app in apps.items():
    assert 'domains' in app and len(app['domains']) > 0, f'{name} missing domains array'
"

# Traefik route files use v3 || syntax for multi-domain apps
python3 - <<'EOF'
import json, os, re, sys
with open('/etc/freedb/registry.json') as f:
    reg = json.load(f)
bad = []
for name, app in reg.get('apps', {}).items():
    if len(app.get('domains', [])) > 1:
        route = f'/etc/traefik/manual/{name}.yaml'
        if os.path.exists(route):
            content = open(route).read()
            # v2 syntax: Host(`a`, `b`) — comma inside Host()
            if re.search(r'Host\(`[^`]+`,', content):
                bad.append(name)
if bad:
    print(f"FAIL: v2 Host() syntax found in routes for: {', '.join(bad)}", file=sys.stderr)
    sys.exit(1)
EOF
if [ $? -eq 0 ]; then
  green "multi-domain route files use Traefik v3 || syntax"
else
  red "multi-domain route files use Traefik v3 || syntax"
fi

# freedb status shows Domains (plural) for multi-domain apps
MULTI_APP=$(python3 -c "
import json
with open('/etc/freedb/registry.json') as f:
    reg = json.load(f)
for name, app in reg.get('apps', {}).items():
    if len(app.get('domains', [])) > 1:
        print(name)
        break
" 2>/dev/null || echo "")

if [ -n "$MULTI_APP" ]; then
  check "freedb status shows Domains label for multi-domain app" \
    bash -c "freedb status '$MULTI_APP' 2>&1 | grep -q '^Domains:'"
  check "Traefik router enabled for multi-domain app" \
    bash -c "incus exec proxy1 -- curl -s http://localhost:8080/api/http/routers/${MULTI_APP}-router@file 2>/dev/null | python3 -c \"import sys,json; d=json.load(sys.stdin); sys.exit(0 if d.get('status')=='enabled' else 1)\""
else
  yellow "no multi-domain apps to verify (deploy one to test)"
fi

# TUI: add app with comma-separated domains
prompt_action "In TUI: [a] -> name: 'test-multi' -> any image -> Expose: y -> Domain: 'test1.example.com, test2.example.com' -> Port: 8080 -> TLS: n -> DB: n -> deploy" \
  "Add app accepts comma-separated domains"

if incus info test-multi &>/dev/null 2>&1; then
  check "test-multi Traefik route uses || syntax" \
    bash -c "incus exec proxy1 -- cat /etc/traefik/manual/test-multi.yaml 2>/dev/null | grep -q '||'"
  check "freedb status shows both domains" \
    bash -c "freedb status test-multi 2>&1 | grep -q 'test2.example.com'"
  prompt_action "In TUI: [enter] on test-multi -> [o] -> [a] -> type 'test3.example.com' -> check TLS warning appears if TLS enabled -> [n] to cancel" \
    "Domain editor TLS warning appears correctly"
  prompt_action "In TUI: [enter] on test-multi -> [o] -> [d] on a domain -> [y] to confirm" \
    "Domain editor remove works"
  check "test-multi route updated after domain removal" \
    bash -c "incus exec proxy1 -- curl -s http://localhost:8080/api/http/routers/test-multi-router@file | python3 -c \"import sys,json; d=json.load(sys.stdin); sys.exit(0 if d.get('status')=='enabled' else 1)\""
  prompt_action "Run: sudo freedb destroy test-multi --yes" "test-multi cleaned up"
else
  yellow "test-multi not found — skipping domain editor checks"
fi

# ─── UPGRADE SYSTEM ───────────────────────────────────────────
section "Upgrade System"
check "version file exists" test -f /etc/freedb/version
echo "  Installed version: $(cat /etc/freedb/version)"
freedb upgrade --dry-run 2>&1 | head -10

# ─── SUMMARY ──────────────────────────────────────────────────
section "Results"
echo ""
echo "  Passed:  $PASS"
echo "  Failed:  $FAIL"
echo "  Skipped: $SKIP"
echo ""

if [ "$FAIL" -gt 0 ]; then
  echo -e "  \033[31mRELEASE TEST FAILED\033[0m"
  exit 1
else
  echo -e "  \033[32mALL TESTS PASSED\033[0m"
  exit 0
fi
