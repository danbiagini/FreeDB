#!/bin/bash
set -euo pipefail

# Optional: provide a service account key file for artifact registry auth
# If not provided, uses the instance's access token from the metadata server
KEY_FILE="${1:-}"

SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
source "${SCRIPT_DIR}/cloud-env.sh"

# ============================================================================
# Install packages
# ============================================================================

# Zabbly package repo (more recent incus version)
# https://github.com/zabbly/incus
sudo curl -fsSL https://pkgs.zabbly.com/key.asc -o /etc/apt/keyrings/zabbly.asc
sudo sh -c 'cat <<EOF > /etc/apt/sources.list.d/zabbly-incus-stable.sources
Enabled: yes
Types: deb
URIs: https://pkgs.zabbly.com/incus/stable
Suites: $(. /etc/os-release && echo ${VERSION_CODENAME})
Components: main
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/zabbly.asc

EOF'

# Add contrib component for ZFS packages
sudo sed -r -i'.BAK' 's/^Components(.*)$/Components\1 contrib/g' /etc/apt/sources.list.d/debian.sources

sudo apt-get update
sudo apt-get install -yq incus postgresql-client cron jq skopeo umoci

# Install ZFS non-interactively (pre-accept the license prompt)
echo "zfs-dkms zfs-dkms/note-incompatible-licenses note true" | sudo debconf-set-selections
# Use generic linux-headers for the current kernel (works across clouds)
KERNEL_HEADERS="linux-headers-$(uname -r)"
if ! apt-cache show "$KERNEL_HEADERS" &>/dev/null; then
  KERNEL_HEADERS="linux-headers-cloud-amd64"
fi
sudo DEBIAN_FRONTEND=noninteractive apt-get install -yq "$KERNEL_HEADERS" zfsutils-linux zfs-dkms zfs-zed

# Check if ZFS module can load — if not, a reboot is needed for the new kernel
if ! sudo modprobe zfs 2>/dev/null; then
  echo ""
  echo "================================================================"
  echo "ZFS kernel module requires a reboot (installed for a newer kernel"
  echo "than currently running). Rebooting now..."
  echo ""
  echo "After reboot, re-run the installer to continue setup."
  echo "================================================================"
  sudo reboot
  # Wait for reboot to take effect — prevent script from continuing
  sleep 300
  exit 1
fi

# ============================================================================
# Configure incus user
# ============================================================================

if ! id incus &>/dev/null; then
  sudo adduser --system --shell /bin/bash --home /home/incus incus
else
  echo "User 'incus' already exists, skipping creation"
fi
sudo mkdir -p /home/incus
sudo chown incus:incus /home/incus
sudo usermod -aG incus-admin incus
sudo usermod -aG sudo incus
sudo -u incus cp /etc/skel/.* /home/incus/ 2>/dev/null || true

# ============================================================================
# Setup artifact registry auth for incus/skopeo
# ============================================================================

sudo -u incus mkdir -p /home/incus/.config/containers

# Always ensure a valid auth.json exists (required by skopeo even for public registries)
sudo -u incus tee /home/incus/.config/containers/auth.json > /dev/null << 'EOF'
{
  "auths": {}
}
EOF

if [ -n "$KEY_FILE" ] && [ -f "$KEY_FILE" ]; then
  echo "Using service account key from $KEY_FILE"
  AUTH_STRING=$(echo -n "_json_key:$(cat "$KEY_FILE")" | base64 -w0)
  sudo -u incus tee /home/incus/.config/containers/auth.json > /dev/null << EOF
{
  "auths": {
    "us-central1-docker.pkg.dev": {
      "auth": "${AUTH_STRING}"
    }
  }
}
EOF
elif [ "$CLOUD" = "gcp" ]; then
  echo "Setting up GCP credential helper for Artifact Registry"

  sudo tee /usr/local/bin/freedb-registry-auth.sh > /dev/null << 'HELPER'
#!/bin/bash
# Credential helper for GCP Artifact Registry
# Outputs a valid auth.json — falls back to empty auths if token fetch fails
TOKEN=$(curl -sf -H "Metadata-Flavor: Google" \
  http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])" 2>/dev/null || echo "")

if [ -n "$TOKEN" ]; then
  AUTH=$(echo -n "oauth2accesstoken:${TOKEN}" | base64 -w0)
  cat << AUTHEOF
{
  "auths": {
    "us-central1-docker.pkg.dev": {
      "auth": "${AUTH}"
    }
  }
}
AUTHEOF
else
  echo '{"auths": {}}'
fi
HELPER
  sudo chmod +x /usr/local/bin/freedb-registry-auth.sh

  /usr/local/bin/freedb-registry-auth.sh | sudo -u incus tee /home/incus/.config/containers/auth.json > /dev/null

  # Refresh the token every 45 minutes (expires every hour)
  CRON_LINE="*/45 * * * * /usr/local/bin/freedb-registry-auth.sh > /home/incus/.config/containers/auth.json 2>/dev/null"
  EXISTING=$(sudo -u incus crontab -l 2>/dev/null | grep -v freedb-registry-auth || true)
  echo "${EXISTING:+$EXISTING
}${CRON_LINE}" | sudo -u incus crontab -
  echo "Credential helper installed with 45-minute token refresh cron"
else
  echo "No private registry auth needed — using empty auth.json for public registries"
fi

# ============================================================================
# Configure incus environment and initialize
# ============================================================================

echo "Setting incus environment variables for OCI container support"
if ! grep -q "XDG_RUNTIME_DIR" /etc/default/incus 2>/dev/null; then
  sudo tee -a /etc/default/incus > /dev/null << 'INCUS_ENV'
# Setup for incus w/ OCI container support
XDG_RUNTIME_DIR=/home/incus/.config
TMPDIR=/home/incus/tmp
INCUS_ENV
else
  echo "Incus environment already configured in /etc/default/incus, skipping"
fi

sudo -u incus mkdir -p /home/incus/tmp

# Initialize incus if not already done (check if storage pool exists)
if sudo incus storage list 2>/dev/null | grep -q pd-standard; then
  echo "Incus already initialized, skipping"
else
  echo "Detecting attached persistent disk..."
  ATTACHED_DISK=$(detect_attached_disk)

  if [ -z "$ATTACHED_DISK" ]; then
    echo "Error: No attached persistent disk found."
    echo "Expected a non-boot block device for ZFS storage."
    exit 1
  fi
  echo "Found attached disk: $ATTACHED_DISK"

  echo "Initializing incus with preseed config..."
  sed "s|/dev/disk/by-id/google-freedb-data-1|${ATTACHED_DISK}|g" \
    "${SCRIPT_DIR}/../config/incus.yaml" | sudo incus admin init --preseed
fi

# ============================================================================
# Post-init: registry remote, DNS, deploy helper
# ============================================================================

# Add artifact registry remote (GCP-specific, skip on other clouds)
if [ "$CLOUD" = "gcp" ]; then
  if sudo incus remote list | grep -q gcr; then
    echo "Remote 'gcr' already exists, skipping"
  else
    sudo incus remote add gcr https://us-central1-docker.pkg.dev --protocol=oci
  fi
fi

# Add Docker Hub remote for OCI images
if sudo incus remote list | grep -q docker; then
  echo "Remote 'docker' already exists, skipping"
else
  sudo incus remote add docker https://docker.io --protocol=oci
fi

# Setup DNS for incus containers
sudo cp "${SCRIPT_DIR}/../config/incus-dns.service" /etc/systemd/system/incus-dns-incusbr0.service
sudo systemctl daemon-reload
sudo systemctl enable incus-dns-incusbr0.service
sudo systemctl start incus-dns-incusbr0.service

# Setup deploy helper
sudo mkdir -p /home/incus/deploy
sudo cp "${SCRIPT_DIR}/../../apps/deploy-container.sh" /home/incus/deploy/
sudo chown -R incus:incus /home/incus/deploy
sudo chmod +x /home/incus/deploy/deploy-container.sh

echo ""
echo "================================================================"
echo "Incus setup complete!"
echo ""
echo "Next steps:"
echo "  ./platform/scripts/traefik-instance.sh"
echo "  ./platform/scripts/db-instance.sh"
echo "================================================================"
