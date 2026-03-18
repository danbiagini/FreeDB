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
MARKER_FILE="$INSTALL_DIR/.freedb-install-phase"

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

# Check if we're resuming after a reboot
PHASE=$(cat "$MARKER_FILE" 2>/dev/null || echo "incus")

if [ "$PHASE" = "incus" ]; then
  echo ""
  echo "Running incus setup..."
  ./platform/scripts/incus.sh

  # ZFS requires a reboot if the running kernel doesn't match the installed modules
  if ! modprobe -n zfs 2>/dev/null; then
    echo "post-reboot" > "$MARKER_FILE"
    echo ""
    echo "================================================================"
    echo "ZFS kernel module requires a reboot."
    echo "After reboot, re-run this installer to complete setup:"
    echo ""
    echo "  cd $INSTALL_DIR && FREEDB_BRANCH=$BRANCH ./install.sh"
    echo "================================================================"
    sudo reboot
  fi
fi

if [ "$PHASE" = "incus" ] || [ "$PHASE" = "post-reboot" ]; then
  # If resuming after reboot, re-run incus.sh (idempotent parts skip, picks up from init)
  if [ "$PHASE" = "post-reboot" ]; then
    echo ""
    echo "Resuming after reboot..."
    ./platform/scripts/incus.sh
  fi

  echo "traefik" > "$MARKER_FILE"
fi

if [ "$PHASE" = "traefik" ] || [ "$(cat "$MARKER_FILE" 2>/dev/null)" = "traefik" ]; then
  echo ""
  echo "Running traefik setup..."
  ./platform/scripts/traefik-instance.sh
  echo "db" > "$MARKER_FILE"
fi

if [ "$PHASE" = "db" ] || [ "$(cat "$MARKER_FILE" 2>/dev/null)" = "db" ]; then
  echo ""
  echo "Running database setup..."
  ./platform/scripts/db-instance.sh
fi

# Clean up marker
rm -f "$MARKER_FILE"

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
