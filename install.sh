#!/bin/bash
set -euo pipefail

# FreeDB bootstrap installer
# Usage: curl -fsSL https://raw.githubusercontent.com/danbiagini/FreeDB/main/install.sh | bash
#
# Optional environment variables:
#   FREEDB_BRANCH  - git branch/tag to checkout (default: main)
#   FREEDB_DIR     - install directory (default: ~/FreeDB)

BRANCH="${FREEDB_BRANCH:-main}"
INSTALL_DIR="${FREEDB_DIR:-$HOME/FreeDB}"
REPO_URL="https://github.com/danbiagini/FreeDB.git"

echo "=============================="
echo " FreeDB Installer"
echo "=============================="
echo ""
echo "Branch:    $BRANCH"
echo "Directory: $INSTALL_DIR"
echo ""

# Install git if not present
if ! command -v git &>/dev/null; then
  echo "Installing git..."
  sudo apt-get update -qq
  sudo apt-get install -yq git
fi

# Clone or update repo
if [ -d "$INSTALL_DIR/.git" ]; then
  echo "Updating existing FreeDB installation..."
  cd "$INSTALL_DIR"
  git fetch origin
  git checkout "$BRANCH"
  git pull origin "$BRANCH" || true
else
  echo "Cloning FreeDB..."
  git clone "$REPO_URL" "$INSTALL_DIR"
  cd "$INSTALL_DIR"
  git checkout "$BRANCH"
fi

echo ""
echo "Running incus setup..."
./platform/scripts/incus.sh

echo ""
echo "Running traefik setup..."
./platform/scripts/traefik-instance.sh

echo ""
echo "Running database setup..."
./platform/scripts/db-instance.sh

echo ""
echo "=============================="
echo " FreeDB installation complete!"
echo "=============================="
echo ""
echo "Installed to: $INSTALL_DIR"
echo ""
echo "To deploy an app, use the deploy helper:"
echo "  sudo -u incus /home/incus/deploy/deploy-container.sh <name> <remote> <image:tag>"
echo ""
