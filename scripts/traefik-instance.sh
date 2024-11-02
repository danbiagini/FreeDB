sudo -u incus incus launch images:debian/12/cloud proxy1
sudo -u incus incus exec proxy1 -- apt update
sudo -u incus incus exec proxy1 -- apt install -yq git curl

sudo -u incus incus exec proxy1 -- sudo adduser --system --shell /bin/bash --home /home/traefik traefik
sudo -u incus incus exec proxy1 -- sudo usermod -aG sudo traefik

# setup the traefik user with bash and PATH after the gcloud install
sudo -u incus incus exec proxy1 -- sudo -u traefik cp /etc/skel/.* ~/

#sudo -u incus incus exec proxy1 -- sudo -u -i git clone https://github.com/danbiagini/FreeDB.git

# check version of traefik
#incus exec proxy1 -- sudo -u traefik -i curl -L 'https://github.com/traefik/traefik/releases/download/v3.1.6/traefik_v3.1.6_linux_amd64.tar.gz' > traefik_v3.1.6.tar.gz
#incus exec proxy1 -- sudo -u traefik -i tar -xzvf traefik_v3.1.6.tar.gz 

#incus exec proxy1 -- sudo -u traefik -i traefik --configFile=FreeDB/config/traefik.toml

sudo -u incusincus network forward port add incusbr0 10.0.1.14 tcp 80,443,8080 <proxy1-ip>

# https://cloud.google.com/iap/docs/using-tcp-forwarding#create-firewall-rule
# to connect from G CloudShell 

# to connect to dash
# gcloud compute start-iap-tunnel freedb 8080 --local-host-port=localhost:8080
# 'preview on port 8080' in cloud shell 