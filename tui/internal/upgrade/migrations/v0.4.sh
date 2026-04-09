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
  DEFAULT_BUCKET="${FREEDB_BACKUP_BUCKET:-freedb-backup}"
  read -r -p "   S3/GCS bucket for backups [${DEFAULT_BUCKET}]: " BUCKET_INPUT
  BACKUP_BUCKET="${BUCKET_INPUT:-$DEFAULT_BUCKET}"
  cat > /opt/freedb/backup.env << EOF
export FREEDB_BACKUP_BUCKET=${BACKUP_BUCKET}
export FREEDB_DB_CONTAINER=db1
EOF
  echo "   Created /opt/freedb/backup.env (bucket=${BACKUP_BUCKET})"
else
  # Source existing config for use in post-migration instructions
  . /opt/freedb/backup.env
  BACKUP_BUCKET="${FREEDB_BACKUP_BUCKET:-freedb-backup}"
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
echo ""
echo "NOTE: The EC2 instance needs S3 permissions to upload backups."
echo "If the instance role doesn't have S3 access, add a policy:"
echo ""
echo "  aws iam put-role-policy --role-name <ROLE_NAME> \\"
echo "    --policy-name freedb-backup-s3 \\"
echo "    --policy-document '{"
echo '    "Version": "2012-10-17",'
echo '    "Statement": [{'
echo '      "Effect": "Allow",'
echo '      "Action": ["s3:PutObject", "s3:GetObject", "s3:ListBucket"],'
echo "      \"Resource\": [\"arn:aws:s3:::${BACKUP_BUCKET}\", \"arn:aws:s3:::${BACKUP_BUCKET}/*\"]"
echo "    }]}"
echo ""
echo "To test: sudo bash -c '. /opt/freedb/backup.env && /opt/freedb/backup-db.sh'"
