#!/bin/bash
set -euo pipefail

# Close stdin to prevent commands from accidentally reading the curl pipe
# when run via: curl ... | bash
exec < /dev/null

# FreeDB bootstrap installer
# Usage: curl -fsSL https://raw.githubusercontent.com/danbiagini/FreeDB/main/install.sh | bash
#
# Optional environment variables:
#   FREEDB_BRANCH  - git branch/tag to checkout (default: main)
#   FREEDB_DIR     - install directory (default: ~/FreeDB)
#
# The installer saves progress to a marker file so it can resume after
# a reboot (required for ZFS kernel module). Just re-run the same command.

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

# Phase: incus
# incus.sh will reboot if ZFS needs a new kernel — the script won't return in that case.
# We write the marker before running so we know to skip incus on resume.
if [ "$PHASE" = "incus" ]; then
  echo "incus" > "$MARKER_FILE"
  echo ""
  echo "Running incus setup..."
  ./platform/scripts/incus.sh
  echo "traefik" > "$MARKER_FILE"
  PHASE="traefik"
fi

# Phase: traefik
if [ "$PHASE" = "traefik" ]; then
  echo ""
  echo "Running traefik setup..."
  ./platform/scripts/traefik-instance.sh
  echo "db" > "$MARKER_FILE"
  PHASE="db"
fi

# Phase: db
if [ "$PHASE" = "db" ]; then
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
