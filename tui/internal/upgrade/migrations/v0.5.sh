#!/bin/bash
set -euo pipefail

# Migration: v0.4 → v0.5
# - Update backup script to per-database backups

echo "=== Migration v0.5: Per-database backups ==="

echo "1. Updating backup script..."
freedb install-backup-script 2>/dev/null || {
  echo "   Warning: Could not install backup script via freedb."
  echo "   Manually copy ops/backup-db.sh to /opt/freedb/backup-db.sh"
}

echo ""
echo "=== Migration v0.5 complete ==="
echo ""
echo "Backups now create individual files per database instead of a single pg_dumpall."
echo "Old backups will be cleaned up automatically after 30 days."
echo ""
echo "To test: sudo bash -c '. /opt/freedb/backup.env && /opt/freedb/backup-db.sh'"
