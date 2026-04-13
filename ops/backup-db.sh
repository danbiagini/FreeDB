#!/bin/bash
set -euo pipefail

# FreeDB database backup script
# Runs on the HOST, not inside the db1 container.
# Uses incus exec to run pg_dump inside db1 and pipes output to the host.
# Writes status to /var/lib/freedb/backup-status.json for TUI display.
#
# MODES
#   No args    — back up all non-system databases + roles
#   $1 = name  — back up a single database
#
# ENVIRONMENT
#   FREEDB_BACKUP_BUCKET - cloud storage bucket name (default: freedb-backup)
#   FREEDB_DB_CONTAINER  - incus container name (default: db1)

BACKUP_DIRECTORY="/var/lib/freedb/backups"
STATUS_FILE="/var/lib/freedb/backup-status.json"
BACKUP_BUCKET="${FREEDB_BACKUP_BUCKET:-freedb-backup}"
DB_CONTAINER="${FREEDB_DB_CONTAINER:-db1}"
CURRENT_DATE=$(date -u "+%Y%m%d_%H%M%SZ")
HOSTNAME=$(hostname)
START_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

mkdir -p "$BACKUP_DIRECTORY"

# Upload a file to cloud storage. Sets CLOUD_STATUS and CLOUD_ERROR.
upload_to_cloud() {
  local file="$1"
  local name
  name=$(basename "$file")
  CLOUD_STATUS="none"
  CLOUD_ERROR=""
  CLOUD_URL=""

  if command -v gcloud &>/dev/null; then
    CLOUD_URL="gs://${BACKUP_BUCKET}/${HOSTNAME}/$name"
    if gcloud storage cp "$file" "$CLOUD_URL" 2>/dev/null; then
      CLOUD_STATUS="uploaded"
    else
      CLOUD_STATUS="failed"
      CLOUD_ERROR="gcloud upload failed"
      CLOUD_URL=""
    fi
  elif command -v aws &>/dev/null; then
    CLOUD_URL="s3://${BACKUP_BUCKET}/${HOSTNAME}/$name"
    if aws s3 cp "$file" "$CLOUD_URL" 2>/dev/null; then
      CLOUD_STATUS="uploaded"
    else
      CLOUD_STATUS="failed"
      CLOUD_ERROR="aws s3 upload failed"
      CLOUD_URL=""
    fi
  else
    CLOUD_STATUS="skipped"
    CLOUD_ERROR="no cloud CLI found"
  fi
}

# Back up a single database (or roles). Appends result to RESULTS array.
backup_one() {
  local db_name="$1"
  local dump_cmd="$2"
  local fileName="${db_name}_${CURRENT_DATE}.sql.gz"
  local status="success"
  local cloud_status="none"
  local cloud_error=""
  local file_size=0

  echo -n "Backing up ${db_name}... "

  if ! sudo incus exec "$DB_CONTAINER" -- $dump_cmd < /dev/null | gzip > "$BACKUP_DIRECTORY/$fileName" 2>/dev/null; then
    echo "FAILED"
    RESULTS+=("{\"database\":\"${db_name}\",\"status\":\"failed\",\"file\":\"${fileName}\",\"size_bytes\":0,\"cloud_upload\":\"none\",\"error\":\"dump failed\"}")
    return 1
  fi

  if [ ! -s "$BACKUP_DIRECTORY/$fileName" ]; then
    echo "FAILED (empty)"
    RESULTS+=("{\"database\":\"${db_name}\",\"status\":\"failed\",\"file\":\"${fileName}\",\"size_bytes\":0,\"cloud_upload\":\"none\",\"error\":\"backup file empty\"}")
    return 1
  fi

  file_size=$(stat -c%s "$BACKUP_DIRECTORY/$fileName" 2>/dev/null || stat -f%z "$BACKUP_DIRECTORY/$fileName" 2>/dev/null || echo "0")

  upload_to_cloud "$BACKUP_DIRECTORY/$fileName"
  cloud_status="$CLOUD_STATUS"
  cloud_error="$CLOUD_ERROR"
  local cloud_url="$CLOUD_URL"

  local size_human
  size_human=$(du -h "$BACKUP_DIRECTORY/$fileName" | cut -f1)
  echo "OK (${size_human}, ${cloud_status})"

  RESULTS+=("{\"database\":\"${db_name}\",\"status\":\"success\",\"file\":\"${fileName}\",\"size_bytes\":${file_size},\"cloud_upload\":\"${cloud_status}\",\"cloud_url\":\"${cloud_url}\",\"error\":\"${cloud_error}\"}")
  return 0
}

# Write status file from RESULTS array
write_status() {
  local entries=""
  for i in "${!RESULTS[@]}"; do
    if [ "$i" -gt 0 ]; then
      entries="${entries},"
    fi
    entries="${entries}
    ${RESULTS[$i]}"
  done

  cat > "$STATUS_FILE" << STATUSEOF
{
  "timestamp": "${START_TIME}",
  "bucket": "${BACKUP_BUCKET}",
  "databases": [${entries}
  ]
}
STATUSEOF
}

# Collect results
RESULTS=()
FAILURES=0

if [ -n "${1:-}" ]; then
  # Single database mode
  backup_one "$1" "sudo -u postgres pg_dump $1" || FAILURES=$((FAILURES + 1))
else
  # Full backup mode: roles + all user databases

  # 1. Dump roles (users/passwords)
  backup_one "roles" "sudo -u postgres pg_dumpall --roles-only" || FAILURES=$((FAILURES + 1))

  # 2. List non-system databases and back up each
  DB_LIST=$(sudo incus exec "$DB_CONTAINER" -- sudo -u postgres psql -At -c \
    "SELECT datname FROM pg_database WHERE datistemplate = false AND datname NOT IN ('postgres')" 2>/dev/null || echo "")

  if [ -z "$DB_LIST" ]; then
    echo "Warning: No user databases found"
  else
    while IFS= read -r db; do
      [ -z "$db" ] && continue
      backup_one "$db" "sudo -u postgres pg_dump $db" || FAILURES=$((FAILURES + 1))
    done <<< "$DB_LIST"
  fi
fi

write_status

# Delete local backups older than 30 days
find "$BACKUP_DIRECTORY" -type f -name "*.sql.gz" -mtime +30 -delete

if [ "$FAILURES" -gt 0 ]; then
  echo "Backup completed with $FAILURES failure(s)"
  exit 1
fi

echo "All backups completed successfully"
