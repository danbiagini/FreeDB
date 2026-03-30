#!/bin/bash
set -euo pipefail

# FreeDB database backup script
# Runs on the HOST, not inside the db1 container.
# Uses incus exec to run pg_dumpall/pg_dump inside db1 and pipes output to the host.
# Writes status to /var/lib/freedb/backup-status.json for TUI display.
#
# PARAMETERS
#   $1 - database name (optional — if omitted, runs pg_dumpall for full backup)
#
# ENVIRONMENT
#   FREEDB_BACKUP_BUCKET - cloud storage bucket name (default: freedb-backup)
#   FREEDB_DB_CONTAINER  - incus container name (default: db1)

BACKUP_DIRECTORY="/var/lib/freedb/backups"
STATUS_FILE="/var/lib/freedb/backup-status.json"
BACKUP_BUCKET="${FREEDB_BACKUP_BUCKET:-freedb-backup}"
DB_CONTAINER="${FREEDB_DB_CONTAINER:-db1}"
CURRENT_DATE=$(date "+%Y%m%d")
HOSTNAME=$(hostname)
DB_NAME="${1:-pg_dumpall}"
START_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Ensure backup directory exists
mkdir -p "$BACKUP_DIRECTORY"

write_status() {
  local status="$1"
  local file="${2:-}"
  local size="${3:-}"
  local cloud="${4:-}"
  local error="${5:-}"

  cat > "$STATUS_FILE" << STATUSEOF
{
  "database": "${DB_NAME}",
  "status": "${status}",
  "timestamp": "${START_TIME}",
  "file": "${file}",
  "size_bytes": ${size:-0},
  "cloud_upload": "${cloud}",
  "error": "${error}",
  "bucket": "${BACKUP_BUCKET}"
}
STATUSEOF
}

# Run the dump inside db1, pipe to host
if [ -z "${1:-}" ]; then
  # Full backup using pg_dumpall (includes users/roles)
  fileName="pg_dumpall_${CURRENT_DATE}.sql.gz"
  if ! sudo incus exec "$DB_CONTAINER" -- sudo -u postgres pg_dumpall | gzip > "$BACKUP_DIRECTORY/$fileName" 2>/dev/null; then
    write_status "failed" "$fileName" "0" "none" "pg_dumpall failed"
    echo "Backup failed: pg_dumpall error"
    exit 1
  fi
else
  # Single database backup
  fileName="${1}_${CURRENT_DATE}.sql.gz"
  if ! sudo incus exec "$DB_CONTAINER" -- sudo -u postgres pg_dump "$1" | gzip > "$BACKUP_DIRECTORY/$fileName" 2>/dev/null; then
    write_status "failed" "$fileName" "0" "none" "pg_dump failed for $1"
    echo "Backup failed: pg_dump error for $1"
    exit 1
  fi
fi

# Verify the backup was created
if [ ! -f "$BACKUP_DIRECTORY/$fileName" ] || [ ! -s "$BACKUP_DIRECTORY/$fileName" ]; then
  write_status "failed" "$fileName" "0" "none" "backup file missing or empty"
  echo "Backup failed: $fileName is missing or empty"
  exit 1
fi

FILE_SIZE=$(stat -c%s "$BACKUP_DIRECTORY/$fileName" 2>/dev/null || stat -f%z "$BACKUP_DIRECTORY/$fileName" 2>/dev/null || echo "0")
echo "Backup created: $BACKUP_DIRECTORY/$fileName ($(du -h "$BACKUP_DIRECTORY/$fileName" | cut -f1))"

# Upload to cloud storage using the host's cloud CLI
CLOUD_STATUS="none"
CLOUD_ERROR=""
if command -v gcloud &>/dev/null; then
  if gcloud storage cp "$BACKUP_DIRECTORY/$fileName" "gs://${BACKUP_BUCKET}/${HOSTNAME}/$fileName" 2>/dev/null; then
    CLOUD_STATUS="uploaded"
    echo "Uploaded to gs://${BACKUP_BUCKET}/${HOSTNAME}/$fileName"
  else
    CLOUD_STATUS="failed"
    CLOUD_ERROR="gcloud upload failed"
    echo "Warning: Cloud upload failed"
  fi
elif command -v aws &>/dev/null; then
  if aws s3 cp "$BACKUP_DIRECTORY/$fileName" "s3://${BACKUP_BUCKET}/${HOSTNAME}/$fileName" 2>/dev/null; then
    CLOUD_STATUS="uploaded"
    echo "Uploaded to s3://${BACKUP_BUCKET}/${HOSTNAME}/$fileName"
  else
    CLOUD_STATUS="failed"
    CLOUD_ERROR="aws s3 upload failed"
    echo "Warning: Cloud upload failed"
  fi
else
  CLOUD_STATUS="skipped"
  CLOUD_ERROR="no cloud CLI found"
  echo "Warning: No cloud CLI found — backup saved locally only"
fi

# Write final status
write_status "success" "$fileName" "$FILE_SIZE" "$CLOUD_STATUS" "$CLOUD_ERROR"

# Delete local backups older than 30 days
find "$BACKUP_DIRECTORY" -type f -name "*.sql.gz" -mtime +30 -delete
