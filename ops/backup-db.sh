#!/bin/bash
set -euo pipefail

# FreeDB database backup script
# Runs on the HOST, not inside the db1 container.
# Uses incus exec to run pg_dumpall/pg_dump inside db1 and pipes output to the host.
#
# PARAMETERS
#   $1 - database name (optional — if omitted, runs pg_dumpall for full backup)
#
# ENVIRONMENT
#   FREEDB_BACKUP_BUCKET - cloud storage bucket name (default: freedb-backup)
#   FREEDB_DB_CONTAINER  - incus container name (default: db1)

BACKUP_DIRECTORY="/var/lib/freedb/backups"
BACKUP_BUCKET="${FREEDB_BACKUP_BUCKET:-freedb-backup}"
DB_CONTAINER="${FREEDB_DB_CONTAINER:-db1}"
CURRENT_DATE=$(date "+%Y%m%d")
HOSTNAME=$(hostname)

# Ensure backup directory exists
mkdir -p "$BACKUP_DIRECTORY"

# Run the dump inside db1, pipe to host
if [ -z "${1:-}" ]; then
  # Full backup using pg_dumpall (includes users/roles)
  fileName="pg_dumpall_${CURRENT_DATE}.sql.gz"
  sudo incus exec "$DB_CONTAINER" -- sudo -u postgres pg_dumpall | gzip > "$BACKUP_DIRECTORY/$fileName"
else
  # Single database backup
  fileName="${1}_${CURRENT_DATE}.sql.gz"
  sudo incus exec "$DB_CONTAINER" -- sudo -u postgres pg_dump "$1" | gzip > "$BACKUP_DIRECTORY/$fileName"
fi

# Verify the backup was created
if [ ! -f "$BACKUP_DIRECTORY/$fileName" ] || [ ! -s "$BACKUP_DIRECTORY/$fileName" ]; then
  echo "Backup failed: $fileName is missing or empty"
  exit 1
fi

echo "Backup created: $BACKUP_DIRECTORY/$fileName ($(du -h "$BACKUP_DIRECTORY/$fileName" | cut -f1))"

# Upload to cloud storage using the host's cloud CLI
if command -v gcloud &>/dev/null; then
  gcloud storage cp "$BACKUP_DIRECTORY/$fileName" "gs://${BACKUP_BUCKET}/${HOSTNAME}/$fileName"
  echo "Uploaded to gs://${BACKUP_BUCKET}/${HOSTNAME}/$fileName"
elif command -v aws &>/dev/null; then
  aws s3 cp "$BACKUP_DIRECTORY/$fileName" "s3://${BACKUP_BUCKET}/${HOSTNAME}/$fileName"
  echo "Uploaded to s3://${BACKUP_BUCKET}/${HOSTNAME}/$fileName"
else
  echo "Warning: No cloud CLI found — backup saved locally only"
fi

# Delete local backups older than 30 days
find "$BACKUP_DIRECTORY" -type f -mtime +30 -delete
