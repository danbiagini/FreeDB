#!/bin/bash
set -euo pipefail

# This script will backup the postgresql database
# and store it in a specified directory

# PARAMETERS
# $1 database name (if none specified run pg_dumpall)

# CONSTANTS
BACKUP_DIRECTORY="/var/lib/postgresql/backups"
BACKUP_BUCKET="${FREEDB_BACKUP_BUCKET:-freedb-backup}"
CURRENT_DATE=$(date "+%Y%m%d")

# !!! Important pg_dump command does not export users/groups tables
# still need to maintain a pg_dumpall for full disaster recovery !!!

if [ -z "${1:-}" ]; then
  # No database specified, do a full backup using pg_dumpall
  fileName=pg_dumpall_$CURRENT_DATE.sql.gz
  pg_dumpall | gzip - > "$BACKUP_DIRECTORY/$fileName"
else
  # Database named (command line argument) use pg_dump for targeted backup
  fileName=${1}_$CURRENT_DATE.sql.gz
  pg_dump "$1" | gzip - > "$BACKUP_DIRECTORY/$fileName"
fi

# Upload to cloud storage
if [ -f "$BACKUP_DIRECTORY/$fileName" ]; then
  # Try gcloud first, then aws, then warn
  if command -v gcloud &>/dev/null; then
    gcloud storage cp "$BACKUP_DIRECTORY/$fileName" "gs://${BACKUP_BUCKET}/$(hostname)/$fileName"
  elif command -v aws &>/dev/null; then
    aws s3 cp "$BACKUP_DIRECTORY/$fileName" "s3://${BACKUP_BUCKET}/$(hostname)/$fileName"
  else
    echo "Warning: No cloud CLI found, backup saved locally only: $BACKUP_DIRECTORY/$fileName"
  fi
  echo "Backup completed successfully: $fileName"

  # Delete local backups older than 1 month
  find "$BACKUP_DIRECTORY" -type f -mtime +30 -delete
else
  echo "Backup failed"
  exit 1
fi
