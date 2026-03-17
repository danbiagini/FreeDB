#!/bin/bash
set -euo pipefail

# Phase 2: Run after ZFS install and incus admin init
# Requires incus daemon to be running with storage pool configured

echo "Configuring incus post-init..."

# Add artifact registry remote
if sudo incus remote list | grep -q gcr; then
  echo "Remote 'gcr' already exists, skipping"
else
  sudo incus remote add gcr https://us-central1-docker.pkg.dev --protocol=oci
fi

# Setup DNS for incus containers
SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
sudo cp "${SCRIPT_DIR}/../config/incus-dns.service" /etc/systemd/system/incus-dns-incusbr0.service
sudo systemctl daemon-reload
sudo systemctl enable incus-dns-incusbr0.service
sudo systemctl start incus-dns-incusbr0.service

# Setup deploy helper
sudo -u incus mkdir -p /home/incus/deploy
sudo -u incus cp "${SCRIPT_DIR}/../../apps/deploy-container.sh" /home/incus/deploy/
sudo -u incus chmod +x /home/incus/deploy/deploy-container.sh

echo ""
echo "================================================================"
echo "Incus setup complete!"
echo ""
echo "Next steps:"
echo "  ./platform/scripts/traefik-instance.sh"
echo "  ./platform/scripts/db-instance.sh"
echo "================================================================"
