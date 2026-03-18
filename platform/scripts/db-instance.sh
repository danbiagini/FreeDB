#!/bin/bash
set -euo pipefail

# Get the directory of the currently running script
SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
REPO_ROOT="${SCRIPT_DIR}/../.."
source "${SCRIPT_DIR}/cloud-env.sh"

sudo incus launch images:debian/12/cloud db1

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
sudo incus network forward port add incusbr0 "${HOST_INTERNAL_IP}" tcp 5432 "${DB1_IP}"

sudo incus exec db1 -- sudo -u postgres cp "/etc/postgresql/${PG_VERSION}/main/pg_hba.conf" "/etc/postgresql/${PG_VERSION}/main/pg_hba.conf.bak"
sudo incus exec db1 -- sudo -u postgres sh -c "cat <<EOF >> /etc/postgresql/${PG_VERSION}/main/pg_hba.conf

## FreeDB host system and container connections
host    all             all             10.0.0.1/24             trust

EOF"

# Restart postgres to pick up config changes
sudo incus exec db1 -- systemctl restart postgresql

# setup nightly pg_dump cron job
sudo incus exec db1 -- sudo -u postgres mkdir -p /var/lib/postgresql/backups
sudo incus exec db1 -- sudo -u postgres mkdir -p /var/lib/postgresql/tools

# Push backup script and cron file directly instead of cloning the whole repo
sudo incus file push "${REPO_ROOT}/ops/backup-db.sh" db1/var/lib/postgresql/tools/
sudo incus exec db1 -- chmod +x /var/lib/postgresql/tools/backup-db.sh
sudo incus file push "${REPO_ROOT}/ops/db1.cron" db1/var/lib/postgresql/tools/

# Install cloud CLI inside db1 for backup uploads
sudo incus exec db1 -- sh -c 'echo "Acquire::ForceIPv4 \"true\";" > /etc/apt/apt.conf.d/99force-ipv4'
install_cloud_cli_in_container() {
  case "$CLOUD" in
    gcp)
      sudo incus exec db1 -- sh -c "curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/cloud.google.gpg"
      sudo incus exec db1 -- sh -c "echo 'deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main' | sudo tee -a /etc/apt/sources.list.d/google-cloud-sdk.list"
      sudo incus exec db1 -- sh -c "apt-get update && apt-get install -yq google-cloud-cli"
      ;;
    aws)
      sudo incus exec db1 -- sh -c 'curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "/tmp/awscliv2.zip" && cd /tmp && unzip -qo awscliv2.zip && ./aws/install && rm -rf aws awscliv2.zip'
      ;;
    *)
      echo "Warning: Unknown cloud, skipping cloud CLI install in db1 — backups to cloud storage will not work"
      ;;
  esac
}
install_cloud_cli_in_container

sudo incus exec db1 -- sudo -u postgres crontab /var/lib/postgresql/tools/db1.cron

echo ""
echo "================================================================"
echo "Database setup complete!"
echo "================================================================"
