#!/bin/bash
# FreeDB registry credential helper
# Generates auth.json for private OCI registries (AWS ECR or GCP Artifact Registry)
# Reads configuration from /opt/freedb/registry-auth.env
#
# ENVIRONMENT (via registry-auth.env)
#   FREEDB_CLOUD        - "aws" or "gcp"
#   FREEDB_REGISTRY_HOST - registry hostname (e.g., 123456.dkr.ecr.us-east-2.amazonaws.com)
#   FREEDB_AWS_REGION   - AWS region (only for aws)

CONFIG_FILE="/opt/freedb/registry-auth.env"

if [ ! -f "$CONFIG_FILE" ]; then
  echo '{"_updated": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'", "auths": {}}'
  exit 0
fi

. "$CONFIG_FILE"

CLOUD="${FREEDB_CLOUD:-}"
REGISTRY_HOST="${FREEDB_REGISTRY_HOST:-}"
AWS_REGION="${FREEDB_AWS_REGION:-}"

if [ -z "$CLOUD" ] || [ -z "$REGISTRY_HOST" ]; then
  echo '{"_updated": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'", "auths": {}}'
  exit 0
fi

TOKEN=""
if [ "$CLOUD" = "aws" ]; then
  TOKEN=$(aws ecr get-login-password --region "$AWS_REGION" 2>/dev/null || echo "")
  if [ -n "$TOKEN" ]; then
    TOKEN=$(echo -n "AWS:${TOKEN}" | base64 -w0)
  fi
elif [ "$CLOUD" = "gcp" ]; then
  ACCESS_TOKEN=$(curl -sf -H "Metadata-Flavor: Google" \
    http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])" 2>/dev/null || echo "")
  if [ -n "$ACCESS_TOKEN" ]; then
    TOKEN=$(echo -n "oauth2accesstoken:${ACCESS_TOKEN}" | base64 -w0)
  fi
fi

if [ -n "$TOKEN" ]; then
  cat << EOF
{
  "_updated": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "auths": {
    "${REGISTRY_HOST}": {
      "auth": "${TOKEN}"
    }
  }
}
EOF
else
  echo '{"_updated": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'", "auths": {}}'
fi
