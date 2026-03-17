#!/bin/bash
set -euo pipefail

# Optional: provide a service account key file for artifact registry auth
# If not provided, uses the instance's access token from the metadata server
KEY_FILE="${1:-}"

# ============================================================================
# Phase 1: Install packages and configure user
# ============================================================================

# needed for zabbly package install (a more recent version for debian 12).
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

sudo apt-get update; sudo apt-get install -yq incus
sudo apt-get install -yq postgresql-client-15

sudo sed -r -i'.BAK' 's/^Components(.*)$/Components\1 contrib/g' /etc/apt/sources.list.d/debian.sources

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

# Setup artifact registry auth for incus/skopeo
sudo -u incus mkdir -p /home/incus/.config/containers

if [ -n "$KEY_FILE" ] && [ -f "$KEY_FILE" ]; then
  # Use static service account key
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
else
  # Use a credential helper that fetches a fresh token from the metadata server
  echo "No key file provided, setting up credential helper using instance metadata token"

  sudo tee /usr/local/bin/gcp-registry-auth.sh > /dev/null << 'HELPER'
#!/bin/bash
# Credential helper for GCP Artifact Registry
# Returns a fresh access token from the instance metadata server
TOKEN=$(curl -s -H "Metadata-Flavor: Google" \
  http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
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
HELPER
  sudo chmod +x /usr/local/bin/gcp-registry-auth.sh

  # Generate initial auth.json and set up a cron to refresh it (token expires every hour)
  sudo -u incus /usr/local/bin/gcp-registry-auth.sh > /tmp/auth.json
  sudo -u incus cp /tmp/auth.json /home/incus/.config/containers/auth.json
  rm -f /tmp/auth.json

  # Refresh the token every 45 minutes
  CRON_LINE="*/45 * * * * /usr/local/bin/gcp-registry-auth.sh > /home/incus/.config/containers/auth.json"
  EXISTING=$(sudo -u incus crontab -l 2>/dev/null | grep -v gcp-registry-auth || true)
  echo "${EXISTING:+$EXISTING
}${CRON_LINE}" | sudo -u incus crontab -
  echo "Credential helper installed with 45-minute token refresh cron"
fi

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

# ============================================================================
# Phase 2: Manual ZFS and incus init
# ============================================================================
# The ZFS install requires interactive confirmation (license prompt).
# Run these commands manually, then proceed to Phase 3.
#
#   sudo apt install linux-headers-cloud-amd64 zfsutils-linux zfs-dkms zfs-zed
#   sudo incus admin init --preseed < platform/config/incus.yaml
#
# References:
#   https://openzfs.github.io/openzfs-docs/Getting%20Started/Debian/
#   https://blog.simos.info/how-to-install-and-set-up-incus-on-a-cloud-server/
#   https://www.cyberciti.biz/faq/installing-zfs-on-debian-12-bookworm-linux-apt-get/
#
# After ZFS/init is done, run:
#   ./platform/scripts/incus-post-init.sh
# ============================================================================

echo ""
echo "================================================================"
echo "Phase 1 complete."
echo ""
echo "Next, install ZFS and initialize incus manually:"
echo "  sudo apt install linux-headers-cloud-amd64 zfsutils-linux zfs-dkms zfs-zed"
echo "  sudo incus admin init --preseed < platform/config/incus.yaml"
echo ""
echo "Then run Phase 2:"
echo "  ./platform/scripts/incus-post-init.sh"
echo "================================================================"
