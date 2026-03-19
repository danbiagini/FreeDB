#!/bin/bash
set -euo pipefail

# Get the directory of the currently running script
SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source "${SCRIPT_DIR}/cloud-env.sh"

# Construct the full path to the traefik.toml file
CONFIG_DIR="${SCRIPT_DIR}/../config"
TRAEFIK_CONFIG_PATH="${CONFIG_DIR}/traefik.toml"

TRAEFIK_VERSION="${TRAEFIK_VERSION:-3.6.10}"

if sudo incus info proxy1 &>/dev/null; then
  echo "proxy1 already exists, skipping launch"
else
  sudo incus launch images:debian/12/cloud proxy1
fi

# Force apt to use IPv4 (IPv6 is disabled on the incus bridge but containers may still try it)
sudo incus exec proxy1 -- sh -c 'echo "Acquire::ForceIPv4 \"true\";" > /etc/apt/apt.conf.d/99force-ipv4'
sudo incus exec proxy1 -- apt update
sudo incus exec proxy1 -- apt install -yq git curl

sudo incus exec proxy1 -- sudo adduser --system --group --shell /bin/bash --home /home/traefik traefik
sudo incus exec proxy1 -- sudo usermod -aG sudo traefik

# setup the traefik user with bash and PATH after the gcloud install
sudo incus exec proxy1 -- sudo -u traefik cp /etc/skel/.* /home/traefik/


# check version of traefik
echo "Installing traefik version ${TRAEFIK_VERSION} (override with TRAEFIK_VERSION env var)"
echo "Check for updates at https://github.com/traefik/traefik/releases"
sudo incus exec proxy1 -- sudo -u traefik -i sh -c "curl -L 'https://github.com/traefik/traefik/releases/download/v${TRAEFIK_VERSION}/traefik_v${TRAEFIK_VERSION}_linux_amd64.tar.gz' > traefik_v${TRAEFIK_VERSION}.tar.gz"
sudo incus exec proxy1 -- sudo -u traefik -i tar -xzvf "traefik_v${TRAEFIK_VERSION}.tar.gz"

sudo incus exec proxy1 -- sudo cp /home/traefik/traefik /usr/local/bin/
sudo incus exec proxy1 -- sudo chown root:root /usr/local/bin/traefik
sudo incus exec proxy1 -- sudo chmod 755 /usr/local/bin/traefik
sudo incus exec proxy1 -- sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/traefik

sudo incus exec proxy1 -- sudo mkdir -p /etc/traefik/acme /etc/traefik/manual /etc/traefik/plugins-storage /etc/gcp-credentials

sudo incus exec proxy1 -- sudo chown -R root:root /etc/traefik
sudo incus exec proxy1 -- sudo chown -R traefik:traefik /etc/traefik/acme
sudo incus exec proxy1 -- sudo chown -R traefik:traefik /etc/traefik/manual
sudo incus exec proxy1 -- sudo chown -R traefik:traefik /etc/traefik/plugins-storage

# Push base traefik config
sudo incus file push "$TRAEFIK_CONFIG_PATH" proxy1/etc/traefik/

# If cloud-saver credentials exist, append the plugin config
CLOUD_SAVER_CONFIG="${CONFIG_DIR}/traefik-cloud-saver.toml"
if sudo incus exec proxy1 -- test -f /etc/gcp-credentials/service_account.json; then
  echo "GCP credentials found, enabling cloud-saver plugin"
  sudo incus exec proxy1 -- sh -c "cat >> /etc/traefik/traefik.toml" < "$CLOUD_SAVER_CONFIG"
else
  echo "No GCP credentials at /etc/gcp-credentials/service_account.json — cloud-saver plugin disabled"
  echo "To enable later, place credentials and re-run this script"
fi

sudo incus exec proxy1 -- sudo chown root:root /etc/traefik/traefik.toml
sudo incus exec proxy1 -- sudo chmod 644 /etc/traefik/traefik.toml

sudo incus file push "${CONFIG_DIR}/traefik.service" proxy1/etc/systemd/system/

sudo incus exec proxy1 -- sudo chown root:root /etc/systemd/system/traefik.service
sudo incus exec proxy1 -- sudo chmod 644 /etc/systemd/system/traefik.service
sudo incus exec proxy1 -- sudo systemctl daemon-reload
sudo incus exec proxy1 -- sudo systemctl enable traefik.service
sudo incus exec proxy1 -- sudo systemctl start traefik.service

# Auto-detect host internal IP and proxy container IP for network forwarding
HOST_INTERNAL_IP=$(get_internal_ip)
PROXY1_IP=$(sudo incus query '/1.0/instances/proxy1?recursion=1' | jq -r '.state.network.eth0.addresses[] | select(.family == "inet") | .address')

echo "Setting up network forward: ${HOST_INTERNAL_IP} -> ${PROXY1_IP}:80,443,8080"
sudo incus network forward create incusbr0 "${HOST_INTERNAL_IP}" || echo "Forward already exists, continuing"
sudo incus network forward port remove incusbr0 "${HOST_INTERNAL_IP}" tcp 80 2>/dev/null || true
sudo incus network forward port remove incusbr0 "${HOST_INTERNAL_IP}" tcp 443 2>/dev/null || true
sudo incus network forward port remove incusbr0 "${HOST_INTERNAL_IP}" tcp 8080 2>/dev/null || true
sudo incus network forward port add incusbr0 "${HOST_INTERNAL_IP}" tcp 80,443,8080 "${PROXY1_IP}"

echo ""
echo "================================================================"
echo "Traefik setup complete!"
echo "================================================================"
