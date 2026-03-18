#!/bin/bash
# Cloud environment detection and abstraction
# Source this file from other scripts: source "$(dirname "$0")/cloud-env.sh"

detect_cloud() {
  if curl -sf -m 1 -H "Metadata-Flavor: Google" \
    http://metadata.google.internal/computeMetadata/v1/ >/dev/null 2>&1; then
    echo "gcp"
  elif curl -sf -m 1 -o /dev/null http://169.254.169.254/latest/meta-data/ 2>/dev/null; then
    echo "aws"
  else
    echo "unknown"
  fi
}

CLOUD=$(detect_cloud)
echo "Detected cloud: $CLOUD"

# Get the host's internal/private IP
get_internal_ip() {
  case "$CLOUD" in
    gcp)
      curl -s -H "Metadata-Flavor: Google" \
        http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip
      ;;
    aws)
      local token
      token=$(curl -sf -X PUT -H "X-aws-ec2-metadata-token-ttl-seconds: 60" \
        http://169.254.169.254/latest/api/token)
      curl -sf -H "X-aws-ec2-metadata-token: $token" \
        http://169.254.169.254/latest/meta-data/local-ipv4
      ;;
    *)
      # Fallback: get IP of the default route interface
      ip -4 route get 1.0.0.0 | grep -oP 'src \K\S+'
      ;;
  esac
}

# Get a registry auth token (for OCI container image pulls)
get_registry_token() {
  case "$CLOUD" in
    gcp)
      curl -s -H "Metadata-Flavor: Google" \
        http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token \
        | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])"
      ;;
    aws)
      aws ecr get-login-password --region "${AWS_REGION:-us-east-1}" 2>/dev/null || echo ""
      ;;
    *)
      echo ""
      ;;
  esac
}

# Build the auth.json content for a registry
# Arguments: $1 = registry hostname
build_registry_auth() {
  local registry="$1"
  local token
  token=$(get_registry_token)

  if [ -z "$token" ]; then
    echo ""
    return
  fi

  case "$CLOUD" in
    gcp)
      local auth
      auth=$(echo -n "oauth2accesstoken:${token}" | base64 -w0)
      cat <<AUTHEOF
{
  "auths": {
    "${registry}": {
      "auth": "${auth}"
    }
  }
}
AUTHEOF
      ;;
    aws)
      local auth
      auth=$(echo -n "AWS:${token}" | base64 -w0)
      cat <<AUTHEOF
{
  "auths": {
    "${registry}": {
      "auth": "${auth}"
    }
  }
}
AUTHEOF
      ;;
    *)
      echo ""
      ;;
  esac
}

# Detect the attached non-boot persistent disk
detect_attached_disk() {
  case "$CLOUD" in
    gcp)
      ls /dev/disk/by-id/google-* 2>/dev/null | grep -v 'part' | grep -v 'persistent-disk-0' | head -1 || true
      ;;
    aws)
      # On AWS, find block devices that aren't the root disk
      local root_disk
      root_disk=$(findmnt -n -o SOURCE / | sed 's/[0-9p]*$//')
      lsblk -dpno NAME | while read -r dev; do
        if [ "$dev" != "$root_disk" ] && [ "$(lsblk -no TYPE "$dev" 2>/dev/null)" = "disk" ]; then
          echo "$dev"
          return
        fi
      done
      ;;
    *)
      # Generic: find non-root block devices
      local root_disk
      root_disk=$(findmnt -n -o SOURCE / | sed 's/[0-9p]*$//')
      lsblk -dpno NAME | while read -r dev; do
        if [ "$dev" != "$root_disk" ] && [ "$(lsblk -no TYPE "$dev" 2>/dev/null)" = "disk" ]; then
          echo "$dev"
          return
        fi
      done
      ;;
  esac
}

# Upload a file to cloud storage
# Arguments: $1 = local path, $2 = bucket, $3 = remote path
upload_to_storage() {
  case "$CLOUD" in
    gcp)
      gcloud storage cp "$1" "gs://$2/$3"
      ;;
    aws)
      aws s3 cp "$1" "s3://$2/$3"
      ;;
    *)
      echo "Warning: Unknown cloud, skipping upload of $1"
      ;;
  esac
}

# Install cloud CLI tools
install_cloud_cli() {
  case "$CLOUD" in
    gcp)
      curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/cloud.google.gpg
      echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" | sudo tee -a /etc/apt/sources.list.d/google-cloud-sdk.list
      sudo apt-get update && sudo apt-get install -yq google-cloud-cli
      ;;
    aws)
      if ! command -v aws &>/dev/null; then
        curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "/tmp/awscliv2.zip"
        cd /tmp && unzip -qo awscliv2.zip && sudo ./aws/install && rm -rf aws awscliv2.zip
        cd -
      fi
      ;;
    *)
      echo "Warning: Unknown cloud, skipping CLI install"
      ;;
  esac
}
