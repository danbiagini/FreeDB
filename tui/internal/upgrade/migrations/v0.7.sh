#!/bin/bash
set -euo pipefail

# Migration: v0.6 → v0.7
# - Replace hardcoded registry auth script with config-driven version
# - Extract existing config values into /opt/freedb/registry-auth.env

echo "=== Migration v0.7: Updatable registry auth script ==="

OLD_SCRIPT="/usr/local/bin/freedb-registry-auth.sh"
ENV_FILE="/opt/freedb/registry-auth.env"

if [ ! -f "$OLD_SCRIPT" ]; then
  echo "No registry auth script found, skipping"
  echo ""
  echo "=== Migration v0.7 complete ==="
  exit 0
fi

# Detect cloud and extract config from the existing script
CLOUD=""
REGISTRY_HOST=""
AWS_REGION=""

if grep -q "aws ecr" "$OLD_SCRIPT" 2>/dev/null; then
  CLOUD="aws"
  AWS_REGION=$(grep -oP 'region \K[a-z0-9-]+' "$OLD_SCRIPT" 2>/dev/null || echo "")
  REGISTRY_HOST=$(grep -oP '[0-9]+\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com' "$OLD_SCRIPT" 2>/dev/null || echo "")
elif grep -q "metadata.google.internal" "$OLD_SCRIPT" 2>/dev/null; then
  CLOUD="gcp"
  REGISTRY_HOST=$(grep -oP '[a-z0-9-]+-docker\.pkg\.dev' "$OLD_SCRIPT" 2>/dev/null || echo "")
fi

if [ -z "$CLOUD" ] || [ -z "$REGISTRY_HOST" ]; then
  echo "Could not detect cloud/registry from existing script, skipping"
  echo ""
  echo "=== Migration v0.7 complete ==="
  exit 0
fi

echo "1. Detected: cloud=$CLOUD, host=$REGISTRY_HOST"

# Write env file
echo "2. Writing $ENV_FILE..."
if [ ! -f "$ENV_FILE" ]; then
  mkdir -p /opt/freedb
  cat > "$ENV_FILE" << EOF
export FREEDB_CLOUD=${CLOUD}
export FREEDB_REGISTRY_HOST=${REGISTRY_HOST}
export FREEDB_AWS_REGION=${AWS_REGION}
EOF
  echo "   Created $ENV_FILE"
else
  echo "   $ENV_FILE already exists, skipping"
fi

# Install the new script
echo "3. Installing updated auth script..."
freedb install-auth-script 2>/dev/null || {
  echo "   Warning: Could not install auth script via freedb."
  echo "   Manually copy ops/registry-auth.sh to /usr/local/bin/freedb-registry-auth.sh"
}

# Run it once to verify
echo "4. Testing new auth script..."
OUTPUT=$(/usr/local/bin/freedb-registry-auth.sh 2>/dev/null || echo "")
if echo "$OUTPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('_updated',''))" 2>/dev/null | grep -q "20"; then
  echo "   Auth script working (has _updated timestamp)"
  # Write the fresh auth
  echo "$OUTPUT" | sudo -u incus tee /home/incus/.config/containers/auth.json > /dev/null
else
  echo "   Warning: Auth script output missing _updated field"
fi

echo ""
echo "=== Migration v0.7 complete ==="
