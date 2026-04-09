#!/bin/bash
set -euo pipefail

# Migration: v0.3 → v0.4
# - Install backup script with status tracking
# - Create backup directory and config if missing
# - Install cron job for nightly backups
#
# Note: The actual backup script is embedded in the freedb binary.
# This migration script calls freedb to install it.

echo "=== Migration v0.4: Backup status tracking ==="

echo "1. Creating backup directories..."
mkdir -p /opt/freedb /var/lib/freedb/backups

echo "2. Installing backup script..."
freedb install-backup-script 2>/dev/null || {
  echo "   Warning: Could not install backup script via freedb."
  echo "   Manually copy ops/backup-db.sh to /opt/freedb/backup-db.sh"
}

echo "3. Writing backup config..."
if [ ! -f /opt/freedb/backup.env ]; then
  BACKUP_BUCKET="${FREEDB_BACKUP_BUCKET:-freedb-backup}"
  cat > /opt/freedb/backup.env << EOF
FREEDB_BACKUP_BUCKET=${BACKUP_BUCKET}
FREEDB_DB_CONTAINER=db1
EOF
  echo "   Created /opt/freedb/backup.env (bucket=${BACKUP_BUCKET})"
else
  echo "   /opt/freedb/backup.env already exists, skipping"
fi

echo "4. Installing backup cron job..."
CRON_LINE="0 3 * * * . /opt/freedb/backup.env && /opt/freedb/backup-db.sh 2>&1 | logger -t freedb-backup"
if crontab -l 2>/dev/null | grep -q freedb-backup; then
  echo "   Backup cron already installed, skipping"
else
  EXISTING=$(crontab -l 2>/dev/null || true)
  echo "${EXISTING:+$EXISTING
}${CRON_LINE}" | crontab -
  echo "   Backup cron installed (nightly at 3am)"
fi

echo ""
echo "=== Migration v0.4 complete ==="
