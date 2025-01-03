#!/bin/bash

# Deploy a container into incus (aka an OCI image)
# Take the following arguments:
#  instance -  the name of the instance to deploy
#  remote -  the incus remote image server to pull the imge from
#  image -  the incus remote image server to pull the imge from (optionally include the :tag to deploy)
# If the instance does not exist, create it.  
# If the instance exists, update it.

# Example:
# ./deploy-container.sh my-instance my-incus-remote my-image:my-tag

# Check the arguments

if [ -z "$1" ] || [ -z "$2" ] || [ -z "$3" ]; then
    echo "Usage: $0 <instance> <remote> <image:tag>"
    exit 1
fi

# First check if the instance exists from "incus list", and delete if it does.

if incus list | grep -q $1; then

    # Let's ask the user if they want to delete the instance
    read -p "Instance $1 exists. Do you want to delete it? (y/n) " answer
    if [ "$answer" == "y" ]; then
        incus delete $1 --force
    fi
fi

# check for .env file in the current directory with the instance name, for example sportsoil-stage.env

if [ -f $1.env ]; then
    env_file="--environment-file $1.env"
fi

echo "Creating instance 'incus launch $2:$3 $1 --profile default $env_file'"

# Now create the instance.
incus launch $2:$3 $1 --profile default $env_file 