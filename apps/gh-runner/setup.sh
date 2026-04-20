#!/bin/bash
set -euo pipefail

# Setup a self-hosted GitHub Actions runner inside a FreeDB system container.
# The runner registers with GitHub and pulls jobs over outbound HTTPS.
# For deploys, it SSHs to the host at 10.0.0.1 and runs freedb deploy.
#
# Prerequisites:
#   - FreeDB platform installed
#   - GitHub personal access token or fine-grained token with admin:org or repo scope
#
# Usage:
#   GITHUB_TOKEN=ghp_xxx GITHUB_REPO=owner/repo ./setup.sh
#
# Optional:
#   RUNNER_NAME=gh-runner       Container/runner name (default: gh-runner)
#   RUNNER_LABELS=freedb,self-hosted  Extra labels (default: freedb)
#   RUNNER_USER=admin           Host user for SSH deploys (default: admin)

RUNNER_NAME="${RUNNER_NAME:-gh-runner}"
RUNNER_LABELS="${RUNNER_LABELS:-freedb}"
RUNNER_USER="${RUNNER_USER:-admin}"
GITHUB_TOKEN="${GITHUB_TOKEN:?Set GITHUB_TOKEN to a GitHub PAT with repo scope}"
GITHUB_REPO="${GITHUB_REPO:?Set GITHUB_REPO to owner/repo}"

echo "=== Setting up GitHub Actions runner: $RUNNER_NAME ==="
echo "Repository: $GITHUB_REPO"
echo ""

# 1. Create the system container (no Traefik)
echo "1. Creating system container..."
if sudo incus info "$RUNNER_NAME" &>/dev/null; then
  echo "   Container $RUNNER_NAME already exists, skipping creation"
else
  sudo incus launch images:ubuntu/24.04/cloud "$RUNNER_NAME" --profile default
  echo "   Waiting for container to start..."
  sleep 5
fi

# 2. Install dependencies inside the container
echo "2. Installing dependencies..."
sudo incus exec "$RUNNER_NAME" -- bash -c "
  apt-get update -qq
  apt-get install -y -qq curl jq openssh-client docker.io > /dev/null 2>&1
  # Create runner user
  useradd -m -s /bin/bash runner 2>/dev/null || true
  usermod -aG docker runner 2>/dev/null || true
"

# 3. Get a registration token from GitHub
echo "3. Getting registration token from GitHub..."
REG_TOKEN=$(curl -sf -X POST \
  -H "Authorization: token $GITHUB_TOKEN" \
  -H "Accept: application/vnd.github+json" \
  "https://api.github.com/repos/$GITHUB_REPO/actions/runners/registration-token" | jq -r '.token')

if [ -z "$REG_TOKEN" ] || [ "$REG_TOKEN" = "null" ]; then
  echo "Error: Could not get registration token. Check GITHUB_TOKEN permissions."
  exit 1
fi

# 4. Install and configure the runner
echo "4. Installing GitHub Actions runner..."
sudo incus exec "$RUNNER_NAME" -- sudo -u runner bash -c "
  cd /home/runner
  ARCH=\$(uname -m)
  case \$ARCH in
    x86_64)  RUNNER_ARCH=x64 ;;
    aarch64) RUNNER_ARCH=arm64 ;;
    *)       echo 'Unsupported architecture'; exit 1 ;;
  esac

  # Download latest runner
  RUNNER_VERSION=\$(curl -sf https://api.github.com/repos/actions/runner/releases/latest | jq -r '.tag_name' | sed 's/v//')
  curl -sL -o actions-runner.tar.gz \
    \"https://github.com/actions/runner/releases/download/v\${RUNNER_VERSION}/actions-runner-linux-\${RUNNER_ARCH}-\${RUNNER_VERSION}.tar.gz\"
  tar xzf actions-runner.tar.gz
  rm actions-runner.tar.gz

  # Configure (non-interactive)
  ./config.sh --unattended \
    --url \"https://github.com/$GITHUB_REPO\" \
    --token \"$REG_TOKEN\" \
    --name \"$RUNNER_NAME\" \
    --labels \"$RUNNER_LABELS\" \
    --replace
"

# 5. Install as a systemd service
echo "5. Installing runner service..."
sudo incus exec "$RUNNER_NAME" -- bash -c "
  cd /home/runner
  ./svc.sh install runner
  ./svc.sh start
"

# 6. Generate SSH key for host access
echo "6. Setting up SSH access to host..."
sudo incus exec "$RUNNER_NAME" -- sudo -u runner bash -c "
  if [ ! -f /home/runner/.ssh/id_ed25519 ]; then
    ssh-keygen -t ed25519 -f /home/runner/.ssh/id_ed25519 -N '' -q
  fi
  # Trust the host key on first connect
  ssh-keyscan -H 10.0.0.1 >> /home/runner/.ssh/known_hosts 2>/dev/null
"

# Get the public key and add it to the host
PUBKEY=$(sudo incus exec "$RUNNER_NAME" -- cat /home/runner/.ssh/id_ed25519.pub)
echo "   Adding runner's SSH key to ${RUNNER_USER}@host..."

# Add to authorized_keys if not already present
if ! grep -qF "$PUBKEY" "/home/${RUNNER_USER}/.ssh/authorized_keys" 2>/dev/null; then
  echo "$PUBKEY" >> "/home/${RUNNER_USER}/.ssh/authorized_keys"
  echo "   Key added to /home/${RUNNER_USER}/.ssh/authorized_keys"
else
  echo "   Key already present"
fi

# 7. Configure sudoers for passwordless freedb deploy
echo "7. Configuring sudoers for freedb deploy..."
SUDOERS_LINE="${RUNNER_USER} ALL=(root) NOPASSWD: /usr/local/bin/freedb deploy *"
SUDOERS_FILE="/etc/sudoers.d/freedb-deploy"
if [ ! -f "$SUDOERS_FILE" ]; then
  echo "$SUDOERS_LINE" > "$SUDOERS_FILE"
  chmod 440 "$SUDOERS_FILE"
  echo "   Created $SUDOERS_FILE"
else
  echo "   $SUDOERS_FILE already exists"
fi

# 8. Test the connection
echo "8. Testing SSH deploy access..."
if sudo incus exec "$RUNNER_NAME" -- sudo -u runner ssh -o ConnectTimeout=5 "${RUNNER_USER}@10.0.0.1" "sudo freedb --version" 2>/dev/null; then
  echo "   SSH deploy access working"
else
  echo "   Warning: SSH test failed. You may need to allow 10.0.0.0/24 in sshd_config"
fi

echo ""
echo "=== Setup complete ==="
echo ""
echo "Runner '$RUNNER_NAME' is registered and running."
echo "It will pick up jobs labeled 'self-hosted' and '$RUNNER_LABELS' from $GITHUB_REPO."
echo ""
echo "In your GitHub Actions workflow, use:"
echo ""
echo "  runs-on: [self-hosted, $RUNNER_LABELS]"
echo "  steps:"
echo "    - name: Deploy"
echo "      run: ssh ${RUNNER_USER}@10.0.0.1 \"sudo freedb deploy myapp --tag \${{ github.ref_name }}\""
echo ""
