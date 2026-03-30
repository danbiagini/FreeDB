#!/bin/bash
set -euo pipefail

# Migration: v0.3 → v0.4
# - Update backup script with status tracking
#
# Note: The actual backup script is embedded in the freedb binary.
# This migration script calls freedb to install it.

echo "=== Migration v0.4: Backup status tracking ==="

echo "1. Updating backup script..."
# The freedb binary has the latest backup-db.sh embedded.
# Write it to /opt/freedb/ via the upgrade system.
freedb install-backup-script 2>/dev/null || {
  echo "   Warning: Could not install backup script via freedb."
  echo "   Manually copy ops/backup-db.sh to /opt/freedb/backup-db.sh"
}

echo ""
echo "=== Migration v0.4 complete ==="
