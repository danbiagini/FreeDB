#!/bin/bash
set -euo pipefail

# Deploy a container into incus (aka an OCI image)
# Take the following arguments:
#  instance -  the name of the instance to deploy
#  remote -  the incus remote image server to pull the image from
#  image -  the image to pull (optionally include the :tag to deploy)
# If the instance does not exist, create it.
# If the instance exists, update it.

# Example:
# ./deploy-container.sh my-instance my-incus-remote my-image:my-tag

# Check the arguments

if [ -z "${1:-}" ] || [ -z "${2:-}" ] || [ -z "${3:-}" ]; then
    echo "Usage: $0 <instance> <remote> <image:tag>"
    exit 1
fi

INSTANCE="$1"
REMOTE="$2"
IMAGE="$3"

# First check if the instance exists from "incus list", and delete if it does.

if incus list | grep -q "$INSTANCE"; then

    # Let's ask the user if they want to delete the instance
    read -p "Instance ${INSTANCE} exists. Do you want to delete it? (y/n) " answer
    if [ "$answer" == "y" ]; then
        incus delete "$INSTANCE" --force
    else
        echo "Aborting."
        exit 0
    fi
fi

# check for .env file in the current directory with the instance name, for example sportsoil-stage.env
env_file=""
if [ -f "${INSTANCE}.env" ]; then
    env_file="--environment-file ${INSTANCE}.env"
fi

echo "Creating instance 'incus launch ${REMOTE}:${IMAGE} ${INSTANCE} --profile default ${env_file}'"

# Now create the instance.
# shellcheck disable=SC2086
incus launch "${REMOTE}:${IMAGE}" "$INSTANCE" --profile default $env_file
