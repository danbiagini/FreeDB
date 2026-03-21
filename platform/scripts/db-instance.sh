#!/bin/bash
set -euo pipefail

# Get the directory of the currently running script
SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
REPO_ROOT="${SCRIPT_DIR}/../.."
source "${SCRIPT_DIR}/cloud-env.sh"

if sudo incus info db1 &>/dev/null; then
  echo "db1 already exists, skipping launch"
else
  sudo incus launch images:debian/12/cloud db1
fi

# Force apt to use IPv4 (IPv6 is disabled on the incus bridge but containers may still try it)
sudo incus exec db1 -- sh -c 'echo "Acquire::ForceIPv4 \"true\";" > /etc/apt/apt.conf.d/99force-ipv4'
sudo incus exec db1 -- apt update
sudo incus exec db1 -- apt install -yq postgresql curl cron

# setup the postgres user with dot files
sudo incus exec db1 -- sudo -u postgres cp /etc/skel/.* /var/lib/postgresql/

# Detect PostgreSQL version installed in the container
PG_VERSION=$(sudo incus exec db1 -- sh -c "ls /etc/postgresql/ | head -1")
echo "Detected PostgreSQL version: $PG_VERSION"

# add pg_hba entry for client and accept tcp connections
sudo incus exec db1 -- sudo -u postgres sed -r -i.BAK "/#listen_addresses/a\listen_addresses = '*'" "/etc/postgresql/${PG_VERSION}/main/postgresql.conf"

# add incus network forward to send postgres traffic from host to instance
# auto-detect the host's internal IP and the container's IP
HOST_INTERNAL_IP=$(get_internal_ip)
DB1_IP=$(sudo incus query '/1.0/instances/db1?recursion=1' | jq -r '.state.network.eth0.addresses[] | select(.family == "inet") | .address')

echo "Setting up network forward: ${HOST_INTERNAL_IP} -> ${DB1_IP}:5432"
sudo incus network forward create incusbr0 "${HOST_INTERNAL_IP}" || echo "Forward already exists, continuing"
sudo incus network forward port remove incusbr0 "${HOST_INTERNAL_IP}" tcp 5432 2>/dev/null || true
sudo incus network forward port add incusbr0 "${HOST_INTERNAL_IP}" tcp 5432 "${DB1_IP}"

sudo incus exec db1 -- sudo -u postgres cp "/etc/postgresql/${PG_VERSION}/main/pg_hba.conf" "/etc/postgresql/${PG_VERSION}/main/pg_hba.conf.bak"
sudo incus exec db1 -- sudo -u postgres sh -c "cat <<EOF >> /etc/postgresql/${PG_VERSION}/main/pg_hba.conf

## FreeDB host system and container connections
host    all             all             10.0.0.1/24             trust

EOF"

# Restart postgres to pick up config changes
sudo incus exec db1 -- systemctl restart postgresql

# Setup nightly backup — runs on the HOST (not inside db1)
# The host has cloud CLI with working IAM credentials for uploading to cloud storage
sudo mkdir -p /opt/freedb /var/lib/freedb/backups
sudo cp "${REPO_ROOT}/ops/backup-db.sh" /opt/freedb/backup-db.sh
sudo chmod +x /opt/freedb/backup-db.sh

# Write backup config with environment-specific values
BACKUP_BUCKET="${FREEDB_BACKUP_BUCKET:-freedb-backup}"
sudo tee /opt/freedb/backup.env > /dev/null << EOF
FREEDB_BACKUP_BUCKET=${BACKUP_BUCKET}
FREEDB_DB_CONTAINER=db1
EOF
echo "Backup config: bucket=${BACKUP_BUCKET}, container=db1"

# Install host-side cron for nightly backups (sources config before running)
CRON_LINE="0 3 * * * . /opt/freedb/backup.env && /opt/freedb/backup-db.sh 2>&1 | logger -t freedb-backup"
EXISTING=$(sudo crontab -l 2>/dev/null | grep -v freedb-backup || true)
echo "${EXISTING:+$EXISTING
}${CRON_LINE}" | sudo crontab -
echo "Backup cron installed (runs nightly at 3am on the host)"

echo ""
echo "================================================================"
echo "Database setup complete!"
echo "================================================================"
