#!/bin/bash

# Get the directory of the currently running script
SCRIPT_DIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# Construct the full path to the traefik.toml file
CONFIG_DIR="${SCRIPT_DIR}/../config"
TRAEFIK_CONFIG_PATH="${CONFIG_DIR}/traefik.toml"

sudo -u incus incus launch images:debian/12/cloud proxy1
sudo -u incus incus exec proxy1 -- apt update
sudo -u incus incus exec proxy1 -- apt install -yq git curl

sudo -u incus incus exec proxy1 -- sudo adduser --system --group --shell /bin/bash --home /home/traefik traefik
sudo -u incus incus exec proxy1 -- sudo usermod -aG sudo traefik

# setup the traefik user with bash and PATH after the gcloud install
sudo -u incus incus exec proxy1 -- sudo -u traefik cp /etc/skel/.* /home/traefik/


# check version of traefik
echo "Installing traefik hard coded version 3.1.7, should check for updated versions at 'https://github.com/traefik/traefik/releases'"
sudo -u incus incus exec proxy1 -- sudo -u traefik -i sh -c "curl -L 'https://github.com/traefik/traefik/releases/download/v3.1.7/traefik_v3.1.7_linux_amd64.tar.gz' > traefik_v3.1.7.tar.gz"
sudo -u incus incus exec proxy1 -- sudo -u traefik -i tar -xzvf traefik_v3.1.7.tar.gz 

sudo -u incus incus exec proxy1 -- sudo cp /home/traefik/traefik /usr/local/bin/
sudo -u incus incus exec proxy1 -- sudo chown root:root /usr/local/bin/traefik
sudo -u incus incus exec proxy1 -- sudo chmod 755 /usr/local/bin/traefik
sudo -u incus incus exec proxy1 -- sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/traefik

sudo -u incus incus exec proxy1 -- sudo mkdir /etc/traefik
sudo -u incus incus exec proxy1 -- sudo mkdir /etc/traefik/acme
sudo -u incus incus exec proxy1 -- sudo chown -R root:root /etc/traefik
sudo -u incus incus exec proxy1 -- sudo chown -R traefik:traefik /etc/traefik/acme

sudo -u incus incus file push  "$TRAEFIK_CONFIG_PATH" proxy1/etc/traefik/

sudo -u incus incus exec proxy1 -- sudo chown root:root /etc/traefik/traefik.toml
sudo -u incus incus exec proxy1 -- sudo chmod 644 /etc/traefik/traefik.toml

sudo -u incus incus file push "${CONFIG_DIR}/traefik.service" /etc/systemd/system/

sudo -u incus incus exec proxy1 -- sudo chown root:root /etc/systemd/system/traefik.service
sudo -u incus incus exec proxy1 -- sudo chmod 644 /etc/systemd/system/traefik.service
sudo -u incus incus exec proxy1 -- sudo systemctl daemon-reload
sudo -u incus incus exec proxy1 -- sudo systemctl start traefik.service

#sudo -u incus incus network forward port add incusbr0 10.0.1.14 tcp 80,443,8080 <proxy1-ip>

#sudo -u incus incus exec proxy1 -- sudo -u traefik -i traefik --configFile=FreeDB/config/traefik.toml
